package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type product struct {
	ID                      string         `json:"id"`
	TenantID                string         `json:"tenant_id,omitempty"`
	TenantName              string         `json:"tenant_name,omitempty"`
	OwnerUserID             string         `json:"owner_user_id,omitempty"`
	OwnerName               string         `json:"owner_name,omitempty"`
	Name                    string         `json:"name"`
	Description             string         `json:"description"`
	Detail                  string         `json:"detail"`
	Highlights              []string       `json:"highlights"`
	LicenseTerms            string         `json:"license_terms"`
	Requirements            string         `json:"requirements"`
	SupportPeriod           string         `json:"support_period"`
	UpdatePolicy            string         `json:"update_policy"`
	UpdateUntil             *time.Time     `json:"update_until"`
	Status                  string         `json:"status"`
	CurrentReleaseVersion   string         `json:"current_release_version,omitempty"`
	CurrentReleaseChangelog string         `json:"current_release_changelog,omitempty"`
	AssetCount              int64          `json:"asset_count"`
	Images                  []productImage `json:"images,omitempty"`
}

type productImage struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	AltText   string `json:"alt_text"`
	MIMEType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
}

const productImageJSON = `COALESCE((SELECT json_agg(json_build_object(
	'id', pi.id,
	'url', '/api/v1/products/' || p.id || '/images/' || pi.id,
	'alt_text', pi.alt_text,
	'mime_type', pi.mime_type,
	'size_bytes', pi.size_bytes
) ORDER BY pi.created_at) FROM product_images pi WHERE pi.product_id=p.id AND pi.tenant_id=p.tenant_id), '[]'::json)`

const productAssetCountSQL = `(SELECT COUNT(*) FROM assets a WHERE a.product_id=p.id AND a.tenant_id=p.tenant_id)`
const productCurrentReleaseVersionSQL = `COALESCE((SELECT r.version FROM product_releases r WHERE r.product_id=p.id AND r.tenant_id=p.tenant_id AND r.status='PUBLISHED' ORDER BY r.published_at DESC NULLS LAST, r.created_at DESC LIMIT 1), '')`
const productCurrentReleaseChangelogSQL = `COALESCE((SELECT r.changelog FROM product_releases r WHERE r.product_id=p.id AND r.tenant_id=p.tenant_id AND r.status='PUBLISHED' ORDER BY r.published_at DESC NULLS LAST, r.created_at DESC LIMIT 1), '')`

// productProfileFieldsSQL selects the operator-facing profile columns shared by
// the admin list, public catalog, public detail, and delivery responses.
const productProfileFieldsSQL = `p.name, p.description, p.detail, p.highlights, p.license_terms, p.requirements, p.support_period, p.update_policy, p.update_until, p.status`

const productListFieldsSQL = productProfileFieldsSQL + `, ` + productCurrentReleaseVersionSQL + `, ` + productCurrentReleaseChangelogSQL + `, ` + productAssetCountSQL + `, ` + productImageJSON

func scanProduct(row interface{ Scan(...any) error }, item *product) error {
	var images []byte
	if err := row.Scan(&item.ID, &item.TenantID, &item.TenantName, &item.OwnerUserID, &item.OwnerName,
		&item.Name, &item.Description, &item.Detail, &item.Highlights, &item.LicenseTerms,
		&item.Requirements, &item.SupportPeriod, &item.UpdatePolicy, &item.UpdateUntil, &item.Status,
		&item.CurrentReleaseVersion, &item.CurrentReleaseChangelog, &item.AssetCount, &images); err != nil {
		return err
	}
	if item.Highlights == nil {
		item.Highlights = []string{}
	}
	if len(images) > 0 {
		if err := json.Unmarshal(images, &item.Images); err != nil {
			return err
		}
	}
	return nil
}

func scanProducts(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}, items *[]product) error {
	for rows.Next() {
		var item product
		if err := scanProduct(rows, &item); err != nil {
			return err
		}
		*items = append(*items, item)
	}
	return rows.Err()
}

func (a *App) listProducts(w http.ResponseWriter, r *http.Request) {
	identity, _ := operatorIdentityFromContext(r.Context())
	rows, err := a.db.Query(r.Context(), `SELECT p.id, p.tenant_id, t.name, COALESCE(p.owner_user_id, ''), COALESCE(u.display_name, ''), `+productListFieldsSQL+` FROM products p JOIN tenants t ON t.id=p.tenant_id LEFT JOIN operator_users u ON u.id=p.owner_user_id WHERE p.tenant_id=$1 ORDER BY p.created_at DESC`, identity.TenantID)
	if err != nil {
		a.logError(r, "list products failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Products could not be loaded.")
		return
	}
	defer rows.Close()
	products := make([]product, 0)
	if err := scanProducts(rows, &products); err != nil {
		a.logError(r, "iterate products failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Products could not be loaded.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": products})
}

func (a *App) createProduct(w http.ResponseWriter, r *http.Request) {
	var input struct {
		productInput
		Status string `json:"status"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Enter a valid product request.")
		return
	}
	input.productInput.normalize()
	input.Status = strings.ToUpper(strings.TrimSpace(input.Status))
	if input.Status == "" {
		input.Status = "DRAFT"
	}
	if !input.productInput.valid() || (input.Status != "DRAFT" && input.Status != "ACTIVE") {
		writeError(w, http.StatusBadRequest, "invalid_product", "Check the product name, description, highlights, and update policy.")
		return
	}
	if input.Status == "ACTIVE" {
		writeError(w, http.StatusConflict, "delivery_asset_required", "Create the product as a draft and upload at least one delivery file before publishing it.")
		return
	}
	id, err := randomID()
	if err != nil {
		a.logError(r, "create product id failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The product could not be created.")
		return
	}
	identity, _ := operatorIdentityFromContext(r.Context())
	item := product{
		ID: id, TenantID: requestTenantID(r), TenantName: identity.TenantName,
		OwnerUserID: identity.UserID, OwnerName: identity.DisplayName,
		Name: input.Name, Description: input.Description, Detail: input.Detail,
		Highlights: input.Highlights, LicenseTerms: input.LicenseTerms,
		Requirements: input.Requirements, SupportPeriod: input.SupportPeriod,
		UpdatePolicy: input.UpdatePolicy, UpdateUntil: input.UpdateUntil, Status: input.Status,
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.logError(r, "begin create product failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The product could not be created.")
		return
	}
	defer tx.Rollback(r.Context())
	_, err = tx.Exec(r.Context(), `INSERT INTO products (id, tenant_id, owner_user_id, name, description, detail, highlights, license_terms, requirements, support_period, update_policy, update_until, status) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		item.ID, item.TenantID, item.OwnerUserID, item.Name, item.Description, item.Detail,
		item.Highlights, item.LicenseTerms, item.Requirements, item.SupportPeriod,
		item.UpdatePolicy, input.updateUntilValue(), item.Status)
	if err == nil {
		var eventID string
		eventID, err = randomID()
		if err == nil {
			action := "PRODUCT_DRAFT_CREATED"
			_, err = tx.Exec(r.Context(), `INSERT INTO admin_events (id, tenant_id, actor, action, resource_type, resource_id, metadata_json) VALUES ($1,$2,$3,$4,'product',$5,jsonb_build_object('status',$6::text,'update_policy',$7::text))`, eventID, requestTenantID(r), operatorActor(r), action, item.ID, item.Status, item.UpdatePolicy)
		}
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		a.logError(r, "create product failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The product could not be created.")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

type productInput struct {
	Name          string     `json:"name"`
	Description   string     `json:"description"`
	Detail        string     `json:"detail"`
	Highlights    []string   `json:"highlights"`
	LicenseTerms  string     `json:"license_terms"`
	Requirements  string     `json:"requirements"`
	SupportPeriod string     `json:"support_period"`
	UpdatePolicy  string     `json:"update_policy"`
	UpdateUntil   *time.Time `json:"update_until"`
}

func (input *productInput) normalize() {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Detail = strings.TrimSpace(input.Detail)
	input.LicenseTerms = strings.TrimSpace(input.LicenseTerms)
	input.Requirements = strings.TrimSpace(input.Requirements)
	input.SupportPeriod = strings.TrimSpace(input.SupportPeriod)
	input.UpdatePolicy = strings.ToUpper(strings.TrimSpace(input.UpdatePolicy))
	if input.UpdatePolicy == "" {
		input.UpdatePolicy = "LIFETIME"
	}

	highlights := make([]string, 0, len(input.Highlights))
	for _, highlight := range input.Highlights {
		highlight = strings.TrimSpace(highlight)
		if highlight != "" {
			highlights = append(highlights, highlight)
		}
	}
	input.Highlights = highlights

	if input.UpdatePolicy != "TILL_DATE" {
		input.UpdateUntil = nil
	} else if input.UpdateUntil != nil {
		normalized := input.UpdateUntil.UTC()
		input.UpdateUntil = &normalized
	}
}

func (input productInput) valid() bool {
	if input.Name == "" || utf8.RuneCountInString(input.Name) > 120 ||
		utf8.RuneCountInString(input.Description) > 2000 ||
		utf8.RuneCountInString(input.Detail) > 6000 ||
		utf8.RuneCountInString(input.LicenseTerms) > 2000 ||
		utf8.RuneCountInString(input.Requirements) > 2000 ||
		utf8.RuneCountInString(input.SupportPeriod) > 120 {
		return false
	}
	if len(input.Highlights) > 8 {
		return false
	}
	for _, highlight := range input.Highlights {
		if utf8.RuneCountInString(highlight) > 120 {
			return false
		}
	}
	switch input.UpdatePolicy {
	case "LIFETIME", "CURRENT_ONLY":
		return input.UpdateUntil == nil
	case "TILL_DATE":
		return input.UpdateUntil != nil && !input.UpdateUntil.IsZero()
	default:
		return false
	}
}

func (input productInput) updateUntilValue() any {
	if input.UpdateUntil == nil {
		return nil
	}
	return input.UpdateUntil.UTC()
}

func (a *App) updateProduct(w http.ResponseWriter, r *http.Request) {
	var input productInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Enter a valid product request.")
		return
	}
	input.normalize()
	if !input.valid() {
		writeError(w, http.StatusBadRequest, "invalid_product", "Check the product name, description, highlights, and update policy.")
		return
	}

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.logError(r, "begin update product failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The product could not be updated.")
		return
	}
	defer tx.Rollback(r.Context())

	item := product{
		ID: productID(r), Name: input.Name, Description: input.Description, Detail: input.Detail,
		Highlights: input.Highlights, LicenseTerms: input.LicenseTerms, Requirements: input.Requirements,
		SupportPeriod: input.SupportPeriod, UpdatePolicy: input.UpdatePolicy, UpdateUntil: input.UpdateUntil,
	}
	err = tx.QueryRow(r.Context(), `UPDATE products SET name=$3, description=$4, detail=$5, highlights=$6, license_terms=$7, requirements=$8, support_period=$9, update_policy=$10, update_until=$11 WHERE id=$1 AND tenant_id=$2 RETURNING status`,
		item.ID, requestTenantID(r), item.Name, item.Description, item.Detail, item.Highlights,
		item.LicenseTerms, item.Requirements, item.SupportPeriod, item.UpdatePolicy, input.updateUntilValue()).Scan(&item.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "product_not_found", "Product was not found.")
		return
	}
	if err == nil {
		var eventID string
		eventID, err = randomID()
		if err == nil {
			_, err = tx.Exec(r.Context(), `INSERT INTO admin_events (id, tenant_id, actor, action, resource_type, resource_id, metadata_json) VALUES ($1,$2,$3,'PRODUCT_CONTENT_UPDATED','product',$4,jsonb_build_object('status',$5::text,'update_policy',$6::text))`, eventID, requestTenantID(r), operatorActor(r), item.ID, item.Status, item.UpdatePolicy)
		}
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		a.logError(r, "update product failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The product could not be updated.")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (a *App) updateProductStatus(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Status string `json:"status"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Enter a valid publication status.")
		return
	}
	input.Status = strings.ToUpper(strings.TrimSpace(input.Status))
	if input.Status != "DRAFT" && input.Status != "ACTIVE" && input.Status != "ARCHIVED" {
		writeError(w, http.StatusBadRequest, "invalid_product_status", "Choose DRAFT, ACTIVE, or ARCHIVED.")
		return
	}

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.logError(r, "begin product status update failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The publication status could not be updated.")
		return
	}
	defer tx.Rollback(r.Context())

	item := product{ID: productID(r)}
	if input.Status == "ACTIVE" {
		err = tx.QueryRow(r.Context(), `SELECT COUNT(a.id) FROM products p LEFT JOIN assets a ON a.product_id=p.id AND a.tenant_id=p.tenant_id WHERE p.id=$1 AND p.tenant_id=$2 GROUP BY p.id`, item.ID, requestTenantID(r)).Scan(&item.AssetCount)
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "product_not_found", "Product was not found.")
			return
		}
		if err != nil {
			a.logError(r, "check product delivery assets failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "The publication status could not be updated.")
			return
		}
		if item.AssetCount == 0 {
			writeError(w, http.StatusConflict, "delivery_asset_required", "Upload at least one delivery file before publishing the product.")
			return
		}
	}
	err = tx.QueryRow(r.Context(), `UPDATE products SET status=$3 WHERE id=$1 AND tenant_id=$2 RETURNING name, description, detail, highlights, license_terms, requirements, support_period, update_policy, update_until, status`,
		item.ID, requestTenantID(r), input.Status).Scan(&item.Name, &item.Description, &item.Detail, &item.Highlights, &item.LicenseTerms, &item.Requirements, &item.SupportPeriod, &item.UpdatePolicy, &item.UpdateUntil, &item.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "product_not_found", "Product was not found.")
		return
	}
	action := map[string]string{"DRAFT": "PRODUCT_UNPUBLISHED", "ACTIVE": "PRODUCT_PUBLISHED", "ARCHIVED": "PRODUCT_ARCHIVED"}[input.Status]
	if err == nil {
		var eventID string
		eventID, err = randomID()
		if err == nil {
			_, err = tx.Exec(r.Context(), `INSERT INTO admin_events (id, tenant_id, actor, action, resource_type, resource_id, metadata_json) VALUES ($1,$2,$3,$4,'product',$5,jsonb_build_object('status',$6::text))`, eventID, requestTenantID(r), operatorActor(r), action, item.ID, item.Status)
		}
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		a.logError(r, "update product status failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The publication status could not be updated.")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func productID(r *http.Request) string {
	return chi.URLParam(r, "productID")
}

func rewriteProductForSpace(item *product, slug string) {
	item.Detail = strings.ReplaceAll(item.Detail, "/api/v1/products/"+item.ID+"/images/", "/api/v1/spaces/"+slug+"/products/"+item.ID+"/images/")
	for index := range item.Images {
		item.Images[index].URL = "/api/v1/spaces/" + slug + "/products/" + item.ID + "/images/" + item.Images[index].ID
	}
}
