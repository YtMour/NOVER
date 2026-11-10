package api

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

var releaseVersionPattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)

type productRelease struct {
	ID          string     `json:"id"`
	TenantID    string     `json:"tenant_id"`
	ProductID   string     `json:"product_id"`
	Version     string     `json:"version"`
	Changelog   string     `json:"changelog"`
	Status      string     `json:"status"`
	CreatedBy   string     `json:"created_by"`
	CreatorName string     `json:"creator_name"`
	PublishedAt *time.Time `json:"published_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type productReleaseInput struct {
	Version   string `json:"version"`
	Changelog string `json:"changelog"`
}

func (input *productReleaseInput) normalize() {
	input.Version = strings.TrimSpace(input.Version)
	input.Changelog = strings.TrimSpace(input.Changelog)
}

func (input productReleaseInput) valid() bool {
	return releaseVersionPattern.MatchString(input.Version) && len([]rune(input.Version)) <= 64 && len([]rune(input.Changelog)) <= 6000
}

func (a *App) listProductReleases(w http.ResponseWriter, r *http.Request) {
	identity, _ := operatorIdentityFromContext(r.Context())
	productID := chi.URLParam(r, "productID")
	rows, err := a.db.Query(r.Context(), `
		SELECT r.id, r.tenant_id, r.product_id, r.version, r.changelog, r.status,
		       COALESCE(r.created_by, ''), COALESCE(u.display_name, ''), r.published_at, r.created_at, r.updated_at
		FROM product_releases r
		LEFT JOIN operator_users u ON u.id=r.created_by
		WHERE r.tenant_id=$1 AND r.product_id=$2
		ORDER BY r.created_at DESC`, identity.TenantID, productID)
	if err != nil {
		a.logError(r, "list product releases failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Product releases could not be loaded.")
		return
	}
	defer rows.Close()
	items := make([]productRelease, 0)
	for rows.Next() {
		var item productRelease
		if err := rows.Scan(&item.ID, &item.TenantID, &item.ProductID, &item.Version, &item.Changelog, &item.Status, &item.CreatedBy, &item.CreatorName, &item.PublishedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			a.logError(r, "scan product release failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Product releases could not be loaded.")
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		a.logError(r, "iterate product releases failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Product releases could not be loaded.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"product_id": productID, "items": items})
}

func (a *App) createProductRelease(w http.ResponseWriter, r *http.Request) {
	identity, _ := operatorIdentityFromContext(r.Context())
	var input productReleaseInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Enter a valid release request.")
		return
	}
	input.normalize()
	if !input.valid() {
		writeError(w, http.StatusBadRequest, "invalid_release", "Use a semantic version such as 1.0.0 and keep the changelog within 6000 characters.")
		return
	}
	productID := chi.URLParam(r, "productID")
	var productStatus string
	if err := a.db.QueryRow(r.Context(), `SELECT status FROM products WHERE id=$1 AND tenant_id=$2`, productID, identity.TenantID).Scan(&productStatus); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "product_not_found", "Product was not found.")
		return
	} else if err != nil {
		a.logError(r, "check product for release failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The product release could not be created.")
		return
	}
	if productStatus == "ARCHIVED" {
		writeError(w, http.StatusConflict, "product_archived", "Archived products cannot receive new releases.")
		return
	}
	id, err := randomID()
	if err != nil {
		a.logError(r, "create product release id failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The product release could not be created.")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.logError(r, "begin product release creation failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The product release could not be created.")
		return
	}
	defer tx.Rollback(r.Context())
	var item productRelease
	err = tx.QueryRow(r.Context(), `
		INSERT INTO product_releases (id, tenant_id, product_id, version, changelog, created_by)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING id, tenant_id, product_id, version, changelog, status, COALESCE(created_by, ''), $7::text, published_at, created_at, updated_at`,
		id, identity.TenantID, productID, input.Version, input.Changelog, identity.UserID, identity.DisplayName).
		Scan(&item.ID, &item.TenantID, &item.ProductID, &item.Version, &item.Changelog, &item.Status, &item.CreatedBy, &item.CreatorName, &item.PublishedAt, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "product_releases_tenant_id_product_id_version_key") {
			writeError(w, http.StatusConflict, "release_exists", "This version already exists for the product.")
			return
		}
		a.logError(r, "create product release failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The product release could not be created.")
		return
	}
	auditID, err := randomID()
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO admin_events (id, tenant_id, actor, action, resource_type, resource_id, metadata_json) VALUES ($1,$2,$3,'PRODUCT_RELEASE_CREATED','product_release',$4,jsonb_build_object('product_id',$5::text,'version',$6::text))`, auditID, identity.TenantID, operatorActor(r), item.ID, productID, item.Version)
	}
	if err != nil {
		a.logError(r, "audit product release creation failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The product release could not be created.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.logError(r, "commit product release creation failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The product release could not be created.")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (a *App) updateProductReleaseStatus(w http.ResponseWriter, r *http.Request) {
	identity, _ := operatorIdentityFromContext(r.Context())
	var input struct {
		Status string `json:"status"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Enter a valid release status.")
		return
	}
	input.Status = strings.ToUpper(strings.TrimSpace(input.Status))
	if input.Status != "DRAFT" && input.Status != "PUBLISHED" && input.Status != "ARCHIVED" {
		writeError(w, http.StatusBadRequest, "invalid_release_status", "Choose DRAFT, PUBLISHED, or ARCHIVED.")
		return
	}
	productID, releaseID := chi.URLParam(r, "productID"), chi.URLParam(r, "releaseID")
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.logError(r, "begin release status update failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The product release could not be updated.")
		return
	}
	defer tx.Rollback(r.Context())
	var item productRelease
	err = tx.QueryRow(r.Context(), `
		SELECT r.id, r.tenant_id, r.product_id, r.version, r.changelog, r.status,
		       COALESCE(r.created_by, ''), COALESCE(u.display_name, ''), r.published_at, r.created_at, r.updated_at
		FROM product_releases r LEFT JOIN operator_users u ON u.id=r.created_by
		WHERE r.id=$1 AND r.product_id=$2 AND r.tenant_id=$3 FOR UPDATE`, releaseID, productID, identity.TenantID).
		Scan(&item.ID, &item.TenantID, &item.ProductID, &item.Version, &item.Changelog, &item.Status, &item.CreatedBy, &item.CreatorName, &item.PublishedAt, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "release_not_found", "Product release was not found.")
		return
	}
	if err != nil {
		a.logError(r, "load product release failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The product release could not be updated.")
		return
	}
	if input.Status == "PUBLISHED" {
		var assetCount int
		var productStatus string
		err = tx.QueryRow(r.Context(), `SELECT p.status, COUNT(a.id) FROM products p LEFT JOIN assets a ON a.product_id=p.id AND a.tenant_id=p.tenant_id WHERE p.id=$1 AND p.tenant_id=$2 GROUP BY p.id, p.status`, productID, identity.TenantID).Scan(&productStatus, &assetCount)
		if err != nil {
			a.logError(r, "check release assets failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "The product release could not be published.")
			return
		}
		if productStatus != "ACTIVE" || assetCount == 0 {
			writeError(w, http.StatusConflict, "release_delivery_required", "Publish the product and upload at least one delivery file before publishing a release.")
			return
		}
	}
	var publishedAt *time.Time
	if input.Status == "PUBLISHED" {
		now := time.Now()
		publishedAt = &now
	}
	err = tx.QueryRow(r.Context(), `UPDATE product_releases SET status=$4, published_at=$5, updated_at=NOW() WHERE id=$1 AND product_id=$2 AND tenant_id=$3 RETURNING status, published_at, updated_at`, releaseID, productID, identity.TenantID, input.Status, publishedAt).Scan(&item.Status, &item.PublishedAt, &item.UpdatedAt)
	if err != nil {
		a.logError(r, "update product release status failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The product release could not be updated.")
		return
	}
	auditID, auditErr := randomID()
	if auditErr == nil {
		_, auditErr = tx.Exec(r.Context(), `INSERT INTO admin_events (id, tenant_id, actor, action, resource_type, resource_id, metadata_json) VALUES ($1,$2,$3,'PRODUCT_RELEASE_STATUS_UPDATED','product_release',$4,jsonb_build_object('product_id',$5::text,'status',$6::text))`, auditID, identity.TenantID, operatorActor(r), releaseID, productID, input.Status)
	}
	if auditErr != nil {
		a.logError(r, "audit product release status failed", auditErr)
		writeError(w, http.StatusInternalServerError, "internal_error", "The product release could not be updated.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.logError(r, "commit product release status failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The product release could not be updated.")
		return
	}
	writeJSON(w, http.StatusOK, item)
}
