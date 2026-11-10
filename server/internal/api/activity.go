package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type activityItem struct {
	CodeID           string     `json:"code_id"`
	ProductID        string     `json:"product_id"`
	ProductName      string     `json:"product_name"`
	CodeSuffix       string     `json:"code_suffix"`
	CodeStatus       string     `json:"code_status"`
	ExternalOrderRef string     `json:"external_order_ref"`
	CodeExpiresAt    time.Time  `json:"code_expires_at"`
	RedeemedAt       *time.Time `json:"redeemed_at"`
	EntitlementUntil *time.Time `json:"entitlement_until"`
	DownloadsIssued  int        `json:"downloads_issued"`
	DownloadEvents   int        `json:"download_events"`
}

type activityProductSummary struct {
	ProductID      string `json:"product_id"`
	Codes          int    `json:"codes"`
	Redeemed       int    `json:"redeemed"`
	DownloadEvents int    `json:"download_events"`
}

type activitySummary struct {
	TotalCodes     int                      `json:"total_codes"`
	RedeemedCodes  int                      `json:"redeemed_codes"`
	DownloadEvents int                      `json:"download_events"`
	Products       []activityProductSummary `json:"products"`
}

func (a *App) adminActivity(w http.ResponseWriter, r *http.Request) {
	identity, _ := operatorIdentityFromContext(r.Context())
	rows, err := a.db.Query(r.Context(), `
		SELECT rc.id, p.id, p.name, rc.display_suffix, rc.status, rc.external_order_ref,
		       rc.expires_at, rc.redeemed_at, e.valid_until,
		       COALESCE(e.downloads_issued, 0),
	       COALESCE((SELECT COUNT(*) FROM download_events de WHERE de.entitlement_id=e.id AND de.tenant_id=e.tenant_id), 0)
	FROM redeem_codes rc
	JOIN products p ON p.id=rc.product_id AND p.tenant_id=rc.tenant_id
	LEFT JOIN entitlements e ON e.id=rc.entitlement_id AND e.tenant_id=rc.tenant_id
	WHERE rc.tenant_id=$1
	ORDER BY rc.created_at DESC
	LIMIT 100`, identity.TenantID)
	if err != nil {
		a.logError(r, "list admin activity failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Activity could not be loaded.")
		return
	}
	defer rows.Close()
	items := make([]activityItem, 0)
	for rows.Next() {
		var item activityItem
		if err := rows.Scan(&item.CodeID, &item.ProductID, &item.ProductName, &item.CodeSuffix, &item.CodeStatus, &item.ExternalOrderRef, &item.CodeExpiresAt, &item.RedeemedAt, &item.EntitlementUntil, &item.DownloadsIssued, &item.DownloadEvents); err != nil {
			a.logError(r, "scan admin activity failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Activity could not be loaded.")
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		a.logError(r, "iterate admin activity failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Activity could not be loaded.")
		return
	}

	summaryRows, err := a.db.Query(r.Context(), `
		SELECT rc.product_id,
		       COUNT(*),
		       COUNT(*) FILTER (WHERE rc.status='REDEEMED'),
	       COALESCE(SUM((SELECT COUNT(*) FROM download_events de WHERE de.entitlement_id=e.id AND de.tenant_id=e.tenant_id)), 0)
		FROM redeem_codes rc
		LEFT JOIN entitlements e ON e.id=rc.entitlement_id AND e.tenant_id=rc.tenant_id
		WHERE rc.tenant_id=$1
		GROUP BY rc.product_id`, identity.TenantID)
	if err != nil {
		a.logError(r, "load activity summary failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Activity summary could not be loaded.")
		return
	}
	defer summaryRows.Close()
	summary := activitySummary{Products: make([]activityProductSummary, 0)}
	for summaryRows.Next() {
		var item activityProductSummary
		if err := summaryRows.Scan(&item.ProductID, &item.Codes, &item.Redeemed, &item.DownloadEvents); err != nil {
			a.logError(r, "scan activity summary failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Activity summary could not be loaded.")
			return
		}
		summary.TotalCodes += item.Codes
		summary.RedeemedCodes += item.Redeemed
		summary.DownloadEvents += item.DownloadEvents
		summary.Products = append(summary.Products, item)
	}
	if err := summaryRows.Err(); err != nil {
		a.logError(r, "iterate activity summary failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Activity summary could not be loaded.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "summary": summary})
}

func (a *App) updateCodeAction(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Action string `json:"action"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Choose a valid code action.")
		return
	}
	input.Action = strings.ToUpper(strings.TrimSpace(input.Action))
	if input.Action != "SUSPEND" && input.Action != "RESUME" && input.Action != "REVOKE" {
		writeError(w, http.StatusBadRequest, "invalid_action", "Choose suspend, resume, or revoke.")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.logError(r, "begin code action failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The code action could not be completed.")
		return
	}
	defer tx.Rollback(r.Context())
	var status, tenantID string
	var entitlementID *string
	err = tx.QueryRow(r.Context(), `SELECT status, tenant_id, entitlement_id FROM redeem_codes WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, chi.URLParam(r, "codeID"), requestTenantID(r)).Scan(&status, &tenantID, &entitlementID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "code_not_found", "The code was not found.")
		return
	}
	if err != nil {
		a.logError(r, "load code for action failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The code action could not be completed.")
		return
	}
	var nextCodeStatus string
	var nextEntitlementStatus string
	switch input.Action {
	case "SUSPEND":
		if status == "REVOKED" || status == "REFUNDED" || status == "EXPIRED" {
			writeError(w, http.StatusConflict, "code_not_changeable", "This code can no longer be suspended.")
			return
		}
		nextCodeStatus = "SUSPENDED"
		nextEntitlementStatus = "SUSPENDED"
	case "RESUME":
		if status != "SUSPENDED" {
			writeError(w, http.StatusConflict, "code_not_suspended", "Only a suspended code can be resumed.")
			return
		}
		if entitlementID == nil {
			nextCodeStatus = "ASSIGNED"
		} else {
			nextCodeStatus = "REDEEMED"
			nextEntitlementStatus = "ACTIVE"
		}
	case "REVOKE":
		if status == "REVOKED" || status == "REFUNDED" {
			writeError(w, http.StatusConflict, "code_not_changeable", "This code has already been revoked.")
			return
		}
		nextCodeStatus = "REVOKED"
		nextEntitlementStatus = "REVOKED"
	}
	if input.Action == "RESUME" && entitlementID != nil {
		tag, err := tx.Exec(r.Context(), `UPDATE entitlements SET status='ACTIVE' WHERE id=$1 AND tenant_id=$2 AND status='SUSPENDED' AND valid_until>NOW()`, *entitlementID, tenantID)
		if err != nil {
			a.logError(r, "resume entitlement failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "The code action could not be completed.")
			return
		}
		if tag.RowsAffected() != 1 {
			writeError(w, http.StatusConflict, "entitlement_not_resumable", "This delivery period has ended or the entitlement is not suspended.")
			return
		}
	} else if entitlementID != nil {
		if _, err := tx.Exec(r.Context(), `UPDATE entitlements SET status=$3 WHERE id=$1 AND tenant_id=$2 AND status IN ('ACTIVE', 'SUSPENDED')`, *entitlementID, tenantID, nextEntitlementStatus); err != nil {
			a.logError(r, "update entitlement status failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "The code action could not be completed.")
			return
		}
	}
	if _, err := tx.Exec(r.Context(), `UPDATE redeem_codes SET status=$3 WHERE id=$1 AND tenant_id=$2`, chi.URLParam(r, "codeID"), tenantID, nextCodeStatus); err != nil {
		a.logError(r, "update code status failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The code action could not be completed.")
		return
	}
	eventID, err := randomID()
	if err != nil {
		a.logError(r, "create action audit id failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The code action could not be completed.")
		return
	}
	if _, err := tx.Exec(r.Context(), `INSERT INTO admin_events (id, tenant_id, actor, action, resource_type, resource_id, metadata_json) VALUES ($1,$2,$3,$4,'redeem_code',$5,jsonb_build_object('status',$6::text))`, eventID, tenantID, operatorActor(r), input.Action, chi.URLParam(r, "codeID"), nextCodeStatus); err != nil {
		a.logError(r, "record code action failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The code action could not be completed.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.logError(r, "commit code action failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The code action could not be completed.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": nextCodeStatus})
}
