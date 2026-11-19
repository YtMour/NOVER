package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"
)

type operatorSessionItem struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	TenantName string    `json:"tenant_name"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Current    bool      `json:"current"`
}

func (a *App) listOperatorSessions(w http.ResponseWriter, r *http.Request) {
	identity, _ := operatorIdentityFromContext(r.Context())
	rows, err := a.db.Query(r.Context(), `SELECT s.id, s.tenant_id, t.name, s.created_at, s.last_seen_at, s.expires_at
		FROM operator_sessions s JOIN tenants t ON t.id=s.tenant_id
		WHERE s.user_id=$1 AND s.revoked_at IS NULL AND s.expires_at > NOW()
		ORDER BY s.created_at DESC`, identity.UserID)
	if err != nil {
		a.logError(r, "list operator sessions failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Sessions could not be loaded.")
		return
	}
	defer rows.Close()
	items := make([]operatorSessionItem, 0)
	for rows.Next() {
		var item operatorSessionItem
		if err := rows.Scan(&item.ID, &item.TenantID, &item.TenantName, &item.CreatedAt, &item.LastSeenAt, &item.ExpiresAt); err != nil {
			a.logError(r, "scan operator session failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Sessions could not be loaded.")
			return
		}
		item.Current = item.ID == identity.SessionID
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		a.logError(r, "iterate operator sessions failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Sessions could not be loaded.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"current_session_id": identity.SessionID, "items": items})
}

func (a *App) revokeOperatorSessionByID(w http.ResponseWriter, r *http.Request) {
	identity, _ := operatorIdentityFromContext(r.Context())
	tag, err := a.db.Exec(r.Context(), `UPDATE operator_sessions SET revoked_at=NOW() WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL`, chi.URLParam(r, "sessionID"), identity.UserID)
	if err != nil {
		a.logError(r, "revoke operator session by id failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The session could not be revoked.")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "session_not_found", "This session was not found.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) revokeOtherOperatorSessions(w http.ResponseWriter, r *http.Request) {
	identity, _ := operatorIdentityFromContext(r.Context())
	tag, err := a.db.Exec(r.Context(), `UPDATE operator_sessions SET revoked_at=NOW() WHERE user_id=$1 AND id<>$2 AND revoked_at IS NULL`, identity.UserID, identity.SessionID)
	if err != nil {
		a.logError(r, "revoke other operator sessions failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Other sessions could not be revoked.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revoked": tag.RowsAffected()})
}

func (a *App) changeOperatorPassword(w http.ResponseWriter, r *http.Request) {
	if allowed, retry := a.allowRequest(r, "password-change", 10, time.Hour); !allowed {
		setRetryAfter(w, retry)
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many password changes. Please wait and try again.")
		return
	}
	var input struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Enter your current and new password.")
		return
	}
	input.CurrentPassword = strings.TrimSpace(input.CurrentPassword)
	input.NewPassword = strings.TrimSpace(input.NewPassword)
	if len(input.NewPassword) < 8 || len(input.NewPassword) > 200 || input.NewPassword == input.CurrentPassword {
		writeError(w, http.StatusBadRequest, "invalid_password", "Choose a new password of at least 8 characters, different from the current one.")
		return
	}
	identity, _ := operatorIdentityFromContext(r.Context())
	var passwordHash string
	if err := a.db.QueryRow(r.Context(), `SELECT password_hash FROM operator_users WHERE id=$1 AND status='ACTIVE'`, identity.UserID).Scan(&passwordHash); err != nil {
		a.logError(r, "load operator password hash failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The password could not be changed.")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(input.CurrentPassword)) != nil {
		writeError(w, http.StatusUnauthorized, "invalid_current_password", "The current password is incorrect.")
		return
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		a.logError(r, "hash new operator password failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The password could not be changed.")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.logError(r, "begin password change failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The password could not be changed.")
		return
	}
	defer tx.Rollback(r.Context())
	var tokenVersion int
	if err := tx.QueryRow(r.Context(), `UPDATE operator_users SET password_hash=$2, token_version=token_version+1, updated_at=NOW() WHERE id=$1 RETURNING token_version`, identity.UserID, string(newHash)).Scan(&tokenVersion); err != nil {
		a.logError(r, "update operator password failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The password could not be changed.")
		return
	}
	// Keep the current session valid while invalidating every other device.
	if _, err := tx.Exec(r.Context(), `UPDATE operator_sessions SET token_version=$2 WHERE id=$1`, identity.SessionID, tokenVersion); err != nil {
		a.logError(r, "refresh current session version failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The password could not be changed.")
		return
	}
	if _, err := tx.Exec(r.Context(), `UPDATE operator_sessions SET revoked_at=NOW() WHERE user_id=$1 AND id<>$2 AND revoked_at IS NULL`, identity.UserID, identity.SessionID); err != nil {
		a.logError(r, "revoke other sessions after password change failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The password could not be changed.")
		return
	}
	eventID, err := randomID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "The password could not be changed.")
		return
	}
	if _, err := tx.Exec(r.Context(), `INSERT INTO admin_events (id, tenant_id, actor, action, resource_type, resource_id, metadata_json) VALUES ($1,$2,$3,'PASSWORD_CHANGED','operator_user',$4,'{}'::jsonb)`, eventID, identity.TenantID, operatorActor(r), identity.UserID); err != nil {
		a.logError(r, "record password change failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The password could not be changed.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.logError(r, "commit password change failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The password could not be changed.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// touchOperatorSession updates last_seen_at at most once per five minutes to
// keep session listings useful without writing on every request.
func (a *App) touchOperatorSession(r *http.Request, sessionID string) {
	if sessionID == "" {
		return
	}
	_, _ = a.db.Exec(r.Context(), `UPDATE operator_sessions SET last_seen_at=NOW() WHERE id=$1 AND last_seen_at < NOW() - INTERVAL '5 minutes'`, sessionID)
}
