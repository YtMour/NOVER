package api

import (
	"net/http"
	"time"
)

type tenantMember struct {
	UserID       string     `json:"user_id"`
	Username     string     `json:"username"`
	DisplayName  string     `json:"display_name"`
	Role         string     `json:"role"`
	PlatformRole string     `json:"platform_role"`
	Status       string     `json:"status"`
	JoinedAt     time.Time  `json:"joined_at"`
	LastActiveAt *time.Time `json:"last_active_at"`
}

func (a *App) listTenantMembers(w http.ResponseWriter, r *http.Request) {
	identity, _ := operatorIdentityFromContext(r.Context())
	rows, err := a.db.Query(r.Context(), `
		SELECT u.id, u.username, u.display_name, m.role, COALESCE(u.platform_role, ''), m.status, m.created_at,
		       (SELECT MAX(s.last_seen_at) FROM operator_sessions s WHERE s.user_id=u.id AND s.tenant_id=m.tenant_id)
		FROM tenant_memberships m
		JOIN operator_users u ON u.id=m.user_id
		WHERE m.tenant_id=$1
		ORDER BY CASE m.role WHEN 'OWNER' THEN 0 WHEN 'ADMIN' THEN 1 ELSE 2 END, u.username`, identity.TenantID)
	if err != nil {
		a.logError(r, "list tenant members failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Tenant members could not be loaded.")
		return
	}
	defer rows.Close()
	items := make([]tenantMember, 0)
	for rows.Next() {
		var item tenantMember
		if err := rows.Scan(&item.UserID, &item.Username, &item.DisplayName, &item.Role, &item.PlatformRole, &item.Status, &item.JoinedAt, &item.LastActiveAt); err != nil {
			a.logError(r, "scan tenant member failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Tenant members could not be loaded.")
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		a.logError(r, "iterate tenant members failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Tenant members could not be loaded.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tenant_id": identity.TenantID, "tenant_name": identity.TenantName, "items": items})
}
