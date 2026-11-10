package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (a *App) redeem(w http.ResponseWriter, r *http.Request) {
	if !a.allowRequest(r, "redeem", 12, time.Minute) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many attempts. Please wait and try again.")
		return
	}
	var input struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Enter a valid redemption code.")
		return
	}
	if !validRedeemCode(input.Code) {
		writeError(w, http.StatusBadRequest, "invalid_code_format", "Check the code and try again.")
		return
	}
	codeHash := hashCode(a.cfg.RedeemHMACKey, input.Code)
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.logError(r, "begin redemption transaction failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The code could not be checked.")
		return
	}
	defer tx.Rollback(r.Context())
	var codeID, productID, releaseID, status, tenantID string
	var expiresAt time.Time
	var accessDays, maxDownloads int
	err = tx.QueryRow(r.Context(), `SELECT id, tenant_id, product_id, COALESCE(release_id, ''), status, expires_at, access_days, max_downloads FROM redeem_codes WHERE code_hmac=$1 AND tenant_id=$2 FOR UPDATE`, codeHash, defaultTenantID).Scan(&codeID, &tenantID, &productID, &releaseID, &status, &expiresAt, &accessDays, &maxDownloads)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "code_not_found", "This code was not recognized. Check it and try again.")
		return
	}
	if err != nil {
		a.logError(r, "lookup redemption code failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The code could not be checked.")
		return
	}
	if status == "REDEEMED" {
		var entitlementID string
		err := tx.QueryRow(r.Context(), `SELECT id FROM entitlements WHERE redeem_code_id=$1 AND tenant_id=$2`, codeID, tenantID).Scan(&entitlementID)
		if err != nil {
			a.logError(r, "load existing entitlement failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "The delivery session could not be restored.")
			return
		}
		accessToken := newAccessToken(a.cfg.AccessTokenKey, entitlementID, codeHash)
		if err := tx.Commit(r.Context()); err != nil {
			a.logError(r, "commit repeated redemption failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "The delivery session could not be restored.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"access_token": accessToken, "product_id": productID})
		return
	}
	if status != "ASSIGNED" {
		writeError(w, http.StatusGone, "code_unavailable", "This code is no longer available.")
		return
	}
	if !expiresAt.After(time.Now()) {
		_, _ = tx.Exec(r.Context(), `UPDATE redeem_codes SET status='EXPIRED' WHERE id=$1 AND tenant_id=$2`, codeID, tenantID)
		if err := tx.Commit(r.Context()); err != nil {
			a.logError(r, "mark code expired failed", err)
		}
		writeError(w, http.StatusGone, "code_expired", "This code has expired.")
		return
	}
	entitlementID, err := randomID()
	if err != nil {
		a.logError(r, "create entitlement id failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The code could not be redeemed.")
		return
	}
	accessToken := newAccessToken(a.cfg.AccessTokenKey, entitlementID, codeHash)
	validUntil := time.Now().AddDate(0, 0, accessDays)
	_, err = tx.Exec(r.Context(), `INSERT INTO entitlements (id, tenant_id, redeem_code_id, product_id, release_id, access_token_hash, valid_until, max_downloads) VALUES ($1, $2, $3, $4, NULLIF($5,''), $6, $7, $8)`, entitlementID, tenantID, codeID, productID, releaseID, hashToken(accessToken), validUntil, maxDownloads)
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE redeem_codes SET status='REDEEMED', redeemed_at=NOW(), entitlement_id=$3 WHERE id=$1 AND tenant_id=$2`, codeID, tenantID, entitlementID)
	}
	if err != nil {
		a.logError(r, "create entitlement failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The code could not be redeemed.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.logError(r, "commit redemption failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The code could not be redeemed.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"access_token": accessToken, "product_id": productID})
}

func (a *App) delivery(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "delivery_auth_required", "Open your delivery link to continue.")
		return
	}
	var result struct {
		EntitlementID string    `json:"entitlement_id"`
		ValidUntil    time.Time `json:"valid_until"`
		BaseReleaseID string    `json:"base_release_id,omitempty"`
		BaseVersion   string    `json:"base_version,omitempty"`
		Product       product   `json:"product"`
		Assets        []asset   `json:"assets"`
	}
	result.Assets = make([]asset, 0)
	err := a.db.QueryRow(r.Context(), `SELECT e.id, e.valid_until, COALESCE(e.release_id, ''), COALESCE(base.version, ''), p.id, p.name, p.description, p.detail, p.status, `+productCurrentReleaseVersionSQL+`, `+productCurrentReleaseChangelogSQL+` FROM entitlements e JOIN products p ON p.id=e.product_id AND p.tenant_id=e.tenant_id LEFT JOIN product_releases base ON base.id=e.release_id AND base.tenant_id=e.tenant_id WHERE e.access_token_hash=$1 AND e.tenant_id=$2 AND e.status='ACTIVE' AND e.valid_until>NOW()`, hashToken(token), defaultTenantID).Scan(&result.EntitlementID, &result.ValidUntil, &result.BaseReleaseID, &result.BaseVersion, &result.Product.ID, &result.Product.Name, &result.Product.Description, &result.Product.Detail, &result.Product.Status, &result.Product.CurrentReleaseVersion, &result.Product.CurrentReleaseChangelog)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnauthorized, "delivery_auth_invalid", "The delivery session is invalid or expired.")
		return
	}
	if err != nil {
		a.logError(r, "load delivery failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The delivery could not be loaded.")
		return
	}
	rows, err := a.db.Query(r.Context(), `
		SELECT a.id, a.product_id, COALESCE(a.release_id, ''), COALESCE(ar.version, ''), a.file_name, a.mime_type, a.size_bytes, a.sha256
		FROM assets a
		LEFT JOIN product_releases ar ON ar.id=a.release_id AND ar.tenant_id=a.tenant_id
		LEFT JOIN product_releases base ON base.id=$3 AND base.tenant_id=a.tenant_id
		WHERE a.product_id=$1 AND a.tenant_id=$2
		  AND (a.release_id IS NULL OR (ar.status='PUBLISHED' AND ($3='' OR ar.published_at>=base.published_at)))
		ORDER BY ar.published_at DESC NULLS LAST, a.created_at DESC`, result.Product.ID, defaultTenantID, result.BaseReleaseID)
	if err != nil {
		a.logError(r, "load delivery assets failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The delivery could not be loaded.")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var item asset
		if err := rows.Scan(&item.ID, &item.ProductID, &item.ReleaseID, &item.ReleaseVersion, &item.FileName, &item.MIMEType, &item.SizeBytes, &item.SHA256); err != nil {
			a.logError(r, "scan delivery asset failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "The delivery could not be loaded.")
			return
		}
		result.Assets = append(result.Assets, item)
	}
	if err := rows.Err(); err != nil {
		a.logError(r, "iterate delivery assets failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The delivery could not be loaded.")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func cleanOrderRef(value string) string {
	return strings.TrimSpace(value)
}
