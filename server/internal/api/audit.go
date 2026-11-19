package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type auditEventItem struct {
	ID           string          `json:"id"`
	Actor        string          `json:"actor"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id"`
	Metadata     json.RawMessage `json:"metadata"`
	CreatedAt    time.Time       `json:"created_at"`
}

func (a *App) listAuditEvents(w http.ResponseWriter, r *http.Request) {
	identity, _ := operatorIdentityFromContext(r.Context())
	limit, offset, err := parsePagination(r, 50, 200)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_pagination", "Use a limit between 1 and 200 and a non-negative offset.")
		return
	}
	action := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("action")))
	resourceType := strings.TrimSpace(r.URL.Query().Get("resource_type"))
	if len(action) > 64 || len(resourceType) > 64 {
		writeError(w, http.StatusBadRequest, "invalid_audit_filter", "Audit filters must be at most 64 characters.")
		return
	}

	var total int
	if err := a.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM admin_events WHERE tenant_id=$1 AND ($2='' OR action=$2) AND ($3='' OR resource_type=$3)`, identity.TenantID, action, resourceType).Scan(&total); err != nil {
		a.logError(r, "count audit events failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Audit events could not be loaded.")
		return
	}
	rows, err := a.db.Query(r.Context(), `SELECT id, actor, action, resource_type, resource_id, metadata_json, created_at
		FROM admin_events
		WHERE tenant_id=$1 AND ($2='' OR action=$2) AND ($3='' OR resource_type=$3)
		ORDER BY created_at DESC, id DESC
		LIMIT $4 OFFSET $5`, identity.TenantID, action, resourceType, limit, offset)
	if err != nil {
		a.logError(r, "list audit events failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Audit events could not be loaded.")
		return
	}
	defer rows.Close()
	items := make([]auditEventItem, 0)
	for rows.Next() {
		var item auditEventItem
		var metadata []byte
		if err := rows.Scan(&item.ID, &item.Actor, &item.Action, &item.ResourceType, &item.ResourceID, &metadata, &item.CreatedAt); err != nil {
			a.logError(r, "scan audit event failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Audit events could not be loaded.")
			return
		}
		if len(metadata) == 0 {
			metadata = []byte("{}")
		}
		item.Metadata = json.RawMessage(metadata)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		a.logError(r, "iterate audit events failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Audit events could not be loaded.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":    items,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
		"has_more": offset+len(items) < total,
	})
}
