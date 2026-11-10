package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type product struct {
	ID          string         `json:"id"`
	TenantID    string         `json:"tenant_id,omitempty"`
	TenantName  string         `json:"tenant_name,omitempty"`
	OwnerUserID string         `json:"owner_user_id,omitempty"`
	OwnerName   string         `json:"owner_name,omitempty"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Detail      string         `json:"detail"`
	Status      string         `json:"status"`
	AssetCount  int64          `json:"asset_count"`
	Images      []productImage `json:"images,omitempty"`
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

func scanProducts(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}, items *[]product) error {
	for rows.Next() {
		var item product
		var images []byte
		if err := rows.Scan(&item.ID, &item.TenantID, &item.TenantName, &item.OwnerUserID, &item.OwnerName, &item.Name, &item.Description, &item.Detail, &item.Status, &item.AssetCount, &images); err != nil {
			return err
		}
		if err := json.Unmarshal(images, &item.Images); err != nil {
			return err
		}
		*items = append(*items, item)
	}
	return rows.Err()
}

func (a *App) listProducts(w http.ResponseWriter, r *http.Request) {
	identity, _ := operatorIdentityFromContext(r.Context())
	rows, err := a.db.Query(r.Context(), `SELECT p.id, p.tenant_id, t.name, COALESCE(p.owner_user_id, ''), COALESCE(u.display_name, ''), p.name, p.description, p.detail, p.status, `+productAssetCountSQL+`, `+productImageJSON+` FROM products p JOIN tenants t ON t.id=p.tenant_id LEFT JOIN operator_users u ON u.id=p.owner_user_id WHERE p.tenant_id=$1 ORDER BY p.created_at DESC`, identity.TenantID)
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
		writeError(w, http.StatusBadRequest, "invalid_product", "Product name is required and must be at most 120 characters.")
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
	item := product{ID: id, TenantID: requestTenantID(r), TenantName: identity.TenantName, OwnerUserID: identity.UserID, OwnerName: identity.DisplayName, Name: input.Name, Description: input.Description, Detail: input.Detail, Status: input.Status}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.logError(r, "begin create product failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The product could not be created.")
		return
	}
	defer tx.Rollback(r.Context())
	_, err = tx.Exec(r.Context(), `INSERT INTO products (id, tenant_id, owner_user_id, name, description, detail, status) VALUES ($1, $2, $3, $4, $5, $6, $7)`, item.ID, item.TenantID, item.OwnerUserID, item.Name, item.Description, item.Detail, item.Status)
	if err == nil {
		var eventID string
		eventID, err = randomID()
		if err == nil {
			action := "PRODUCT_DRAFT_CREATED"
			_, err = tx.Exec(r.Context(), `INSERT INTO admin_events (id, tenant_id, actor, action, resource_type, resource_id, metadata_json) VALUES ($1,$2,$3,$4,'product',$5,jsonb_build_object('status',$6::text))`, eventID, requestTenantID(r), operatorActor(r), action, item.ID, item.Status)
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
	Name        string `json:"name"`
	Description string `json:"description"`
	Detail      string `json:"detail"`
}

func (input *productInput) normalize() {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Detail = strings.TrimSpace(input.Detail)
}

func (input productInput) valid() bool {
	return input.Name != "" && utf8.RuneCountInString(input.Name) <= 120 && utf8.RuneCountInString(input.Description) <= 2000 && utf8.RuneCountInString(input.Detail) <= 6000
}

func (a *App) updateProduct(w http.ResponseWriter, r *http.Request) {
	var input productInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Enter a valid product request.")
		return
	}
	input.normalize()
	if !input.valid() {
		writeError(w, http.StatusBadRequest, "invalid_product", "Product name is required and must be at most 120 characters.")
		return
	}

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.logError(r, "begin update product failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The product could not be updated.")
		return
	}
	defer tx.Rollback(r.Context())

	item := product{ID: productID(r), Name: input.Name, Description: input.Description, Detail: input.Detail}
	err = tx.QueryRow(r.Context(), `UPDATE products SET name=$3, description=$4, detail=$5 WHERE id=$1 AND tenant_id=$2 RETURNING status`, item.ID, requestTenantID(r), item.Name, item.Description, item.Detail).Scan(&item.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "product_not_found", "Product was not found.")
		return
	}
	if err == nil {
		var eventID string
		eventID, err = randomID()
		if err == nil {
			_, err = tx.Exec(r.Context(), `INSERT INTO admin_events (id, tenant_id, actor, action, resource_type, resource_id, metadata_json) VALUES ($1,$2,$3,'PRODUCT_CONTENT_UPDATED','product',$4,jsonb_build_object('status',$5::text))`, eventID, requestTenantID(r), operatorActor(r), item.ID, item.Status)
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
	err = tx.QueryRow(r.Context(), `UPDATE products SET status=$3 WHERE id=$1 AND tenant_id=$2 RETURNING name, description, detail, status`, item.ID, requestTenantID(r), input.Status).Scan(&item.Name, &item.Description, &item.Detail, &item.Status)
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
