package api

import (
	"net/http"
	"strings"
	"time"
)

func (a *App) operatorLogin(w http.ResponseWriter, r *http.Request) {
	if !a.allowRequest(r, "operator-login", 8, time.Minute) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many login attempts. Please wait and try again.")
		return
	}
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Enter a valid username and password.")
		return
	}
	if !constantTimeEqual(strings.TrimSpace(input.Username), a.cfg.AdminUsername) || !constantTimeEqual(input.Password, a.cfg.AdminPassword) {
		writeError(w, http.StatusUnauthorized, "operator_login_failed", "The username or password is incorrect.")
		return
	}
	token, expiresAt, err := newOperatorToken(a.cfg.OperatorTokenKey, a.cfg.AdminUsername, 8*time.Hour)
	if err != nil {
		a.logError(r, "create operator token failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The operator session could not be created.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"access_token": token, "expires_at": expiresAt})
}

func (a *App) operatorSession(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"username": a.cfg.AdminUsername})
}

func (a *App) listPublicProducts(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `SELECT id, name, description, status FROM products WHERE status='ACTIVE' ORDER BY created_at DESC`)
	if err != nil {
		a.logError(r, "list public products failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Products could not be loaded.")
		return
	}
	defer rows.Close()
	items := make([]product, 0)
	for rows.Next() {
		var item product
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.Status); err != nil {
			a.logError(r, "scan public product failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Products could not be loaded.")
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		a.logError(r, "iterate public products failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Products could not be loaded.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *App) requireOperator(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" || !verifyOperatorToken(a.cfg.OperatorTokenKey, token, a.cfg.AdminUsername) {
			writeError(w, http.StatusUnauthorized, "operator_auth_required", "Operator sign-in is required.")
			return
		}
		next.ServeHTTP(w, r)
	})
}
