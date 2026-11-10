package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

func (a *App) createCodes(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Count            int    `json:"count"`
		CodeTTLDays      int    `json:"code_ttl_days"`
		AccessDays       int    `json:"access_days"`
		MaxDownloads     int    `json:"max_downloads"`
		ExternalOrderRef string `json:"external_order_ref"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Enter valid code batch details.")
		return
	}
	if input.Count == 0 {
		input.Count = 1
	}
	if input.CodeTTLDays == 0 {
		input.CodeTTLDays = 90
	}
	if input.AccessDays == 0 {
		input.AccessDays = 365
	}
	if input.MaxDownloads == 0 {
		input.MaxDownloads = 25
	}
	if input.Count < 1 || input.Count > 100 || input.CodeTTLDays < 1 || input.CodeTTLDays > 3650 || input.AccessDays < 1 || input.AccessDays > 3650 || input.MaxDownloads < 1 || input.MaxDownloads > 10000 || len(input.ExternalOrderRef) > 200 {
		writeError(w, http.StatusBadRequest, "invalid_code_batch", "Check code count, validity, download limit, and order reference.")
		return
	}
	productID := chi.URLParam(r, "productID")
	var productExists bool
	if err := a.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM products WHERE id=$1 AND tenant_id=$2 AND status='ACTIVE')`, productID, requestTenantID(r)).Scan(&productExists); err != nil {
		a.logError(r, "check product for codes failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Codes could not be created.")
		return
	}
	if !productExists {
		writeError(w, http.StatusNotFound, "product_not_found", "Product was not found.")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.logError(r, "begin code transaction failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Codes could not be created.")
		return
	}
	defer tx.Rollback(r.Context())
	issued := make([]string, 0, input.Count)
	expires := time.Now().AddDate(0, 0, input.CodeTTLDays)
	for i := 0; i < input.Count; i++ {
		code, err := newRedeemCode()
		if err != nil {
			a.logError(r, "generate redeem code failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Codes could not be created.")
			return
		}
		id, err := randomID()
		if err != nil {
			a.logError(r, "create code id failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Codes could not be created.")
			return
		}
		normalized := normalizeCode(code)
		_, err = tx.Exec(r.Context(), `INSERT INTO redeem_codes (id, tenant_id, product_id, code_hmac, display_suffix, external_order_ref, expires_at, access_days, max_downloads) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`, id, requestTenantID(r), productID, hashCode(a.cfg.RedeemHMACKey, normalized), normalized[len(normalized)-4:], cleanOrderRef(input.ExternalOrderRef), expires, input.AccessDays, input.MaxDownloads)
		if err != nil {
			a.logError(r, "save redeem code failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Codes could not be created.")
			return
		}
		issued = append(issued, code)
	}
	eventID, err := randomID()
	if err != nil {
		a.logError(r, "create code audit id failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Codes could not be created.")
		return
	}
	if _, err := tx.Exec(r.Context(), `INSERT INTO admin_events (id, tenant_id, actor, action, resource_type, resource_id, metadata_json) VALUES ($1,$2,$3,'CODES_ISSUED','product',$4,jsonb_build_object('count',$5::integer))`, eventID, requestTenantID(r), operatorActor(r), productID, input.Count); err != nil {
		a.logError(r, "record code issuance failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Codes could not be created.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.logError(r, "commit code batch failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Codes could not be created.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"codes": issued, "code_ttl_until": expires, "access_days": input.AccessDays, "max_downloads": input.MaxDownloads})
}
