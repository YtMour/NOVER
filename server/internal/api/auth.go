package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

var dummyPasswordHash = func() []byte {
	hash, _ := bcrypt.GenerateFromPassword([]byte("nover-dummy-password"), bcrypt.DefaultCost)
	return hash
}()

type operatorIdentity struct {
	SessionID         string   `json:"-"`
	UserID            string   `json:"user_id"`
	Username          string   `json:"username"`
	DisplayName       string   `json:"display_name"`
	PlatformRole      string   `json:"platform_role"`
	MembershipID      string   `json:"membership_id"`
	TenantID          string   `json:"tenant_id"`
	TenantSlug        string   `json:"tenant_slug"`
	TenantName        string   `json:"tenant_name"`
	Role              string   `json:"role"`
	MembershipStatus  string   `json:"membership_status"`
	ApplicationID     string   `json:"application_id,omitempty"`
	ApplicationStatus string   `json:"application_status,omitempty"`
	Permissions       []string `json:"permissions"`
}

type operatorContextKey struct{}

const operatorIdentitySelect = `
	SELECT u.id, u.username, u.display_name, COALESCE(u.platform_role, ''), u.token_version,
	       m.id, m.tenant_id, t.name, t.slug, m.role, m.status,
	       COALESCE(ja.id, ''), COALESCE(ja.status, '')
	FROM operator_users u
	JOIN tenant_memberships m ON m.user_id=u.id AND m.status IN ('PENDING', 'ACTIVE')
	JOIN tenants t ON t.id=m.tenant_id AND t.status='ACTIVE'
	LEFT JOIN join_applications ja ON ja.operator_user_id=u.id`

func scanOperatorIdentity(row interface{ Scan(...any) error }, identity *operatorIdentity, tokenVersion *int) error {
	return row.Scan(
		&identity.UserID, &identity.Username, &identity.DisplayName, &identity.PlatformRole, tokenVersion,
		&identity.MembershipID, &identity.TenantID, &identity.TenantName, &identity.TenantSlug, &identity.Role, &identity.MembershipStatus,
		&identity.ApplicationID, &identity.ApplicationStatus,
	)
}

func (a *App) operatorLogin(w http.ResponseWriter, r *http.Request) {
	if allowed, retry := a.allowRequest(r, "operator-login", 30, time.Minute); !allowed {
		setRetryAfter(w, retry)
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
	input.Username = strings.TrimSpace(input.Username)

	var identity operatorIdentity
	var passwordHash string
	var tokenVersion int
	var mfaEnabled bool
	err := a.db.QueryRow(r.Context(), `
		SELECT u.id, u.username, u.display_name, u.password_hash, COALESCE(u.platform_role, ''), u.token_version,
		       m.id, m.tenant_id, t.name, t.slug, m.role, m.status,
		       COALESCE(ja.id, ''), COALESCE(ja.status, ''),
		       (u.mfa_enabled_at IS NOT NULL AND u.mfa_secret IS NOT NULL)
		FROM operator_users u
		JOIN tenant_memberships m ON m.user_id=u.id AND m.status IN ('PENDING', 'ACTIVE')
		JOIN tenants t ON t.id=m.tenant_id AND t.status='ACTIVE'
		LEFT JOIN join_applications ja ON ja.operator_user_id=u.id
		WHERE LOWER(u.username)=LOWER($1) AND u.status='ACTIVE'
		ORDER BY CASE WHEN t.id='tenant-default' THEN 0 ELSE 1 END, m.created_at
		LIMIT 1`, input.Username).Scan(
		&identity.UserID, &identity.Username, &identity.DisplayName, &passwordHash, &identity.PlatformRole, &tokenVersion,
		&identity.MembershipID, &identity.TenantID, &identity.TenantName, &identity.TenantSlug, &identity.Role, &identity.MembershipStatus,
		&identity.ApplicationID, &identity.ApplicationStatus, &mfaEnabled,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(input.Password))
		writeError(w, http.StatusUnauthorized, "operator_login_failed", "The username or password is incorrect.")
		return
	}
	if err != nil {
		a.logError(r, "load operator identity failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The operator session could not be created.")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(input.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "operator_login_failed", "The username or password is incorrect.")
		return
	}
	if mfaEnabled {
		challenge, expiresAt, err := newMFAChallenge(a.cfg.OperatorTokenKey, identity.UserID)
		if err != nil {
			a.logError(r, "create mfa challenge failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "The sign-in challenge could not be created.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"mfa_required": true, "challenge_token": challenge, "expires_at": expiresAt})
		return
	}
	a.completeOperatorLogin(w, r, identity, tokenVersion)
}

func (a *App) completeOperatorLogin(w http.ResponseWriter, r *http.Request, identity operatorIdentity, tokenVersion int) {
	sessionID, err := randomID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "The operator session could not be created.")
		return
	}
	token, expiresAt, err := newOperatorToken(a.cfg.OperatorTokenKey, sessionID, identity.UserID, 8*time.Hour)
	if err == nil {
		_, err = a.db.Exec(r.Context(), `INSERT INTO operator_sessions (id, user_id, membership_id, tenant_id, token_version, token_hash, expires_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`, sessionID, identity.UserID, identity.MembershipID, identity.TenantID, tokenVersion, hashToken(token), expiresAt)
	}
	if err != nil {
		a.logError(r, "create operator session failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The operator session could not be created.")
		return
	}
	identity.SessionID = sessionID
	identity.Permissions = permissionNames(identity)
	writeJSON(w, http.StatusOK, map[string]any{"access_token": token, "expires_at": expiresAt, "session": identity})
}

func (a *App) operatorSession(w http.ResponseWriter, r *http.Request) {
	identity, ok := operatorIdentityFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "operator_auth_required", "Operator sign-in is required.")
		return
	}
	writeJSON(w, http.StatusOK, identity)
}

func (a *App) revokeOperatorSession(w http.ResponseWriter, r *http.Request) {
	identity, ok := operatorIdentityFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "operator_auth_required", "Operator sign-in is required.")
		return
	}
	if _, err := a.db.Exec(r.Context(), `UPDATE operator_sessions SET revoked_at=NOW() WHERE id=$1 AND revoked_at IS NULL`, identity.SessionID); err != nil {
		a.logError(r, "revoke operator session failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The operator session could not be revoked.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) listPublicProducts(w http.ResponseWriter, r *http.Request) {
	a.writePublicProducts(w, r, defaultTenantID, "", "")
}

func (a *App) listSpaceProducts(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "spaceSlug")
	tenantID, tenantName, ok, err := a.resolveActiveTenantBySlug(r.Context(), slug)
	if err != nil {
		a.logError(r, "load public space failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Products could not be loaded.")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "space_not_found", "This space is not available.")
		return
	}
	a.writePublicProducts(w, r, tenantID, slug, tenantName)
}

func (a *App) resolveActiveTenantBySlug(ctx context.Context, slug string) (tenantID, tenantName string, ok bool, err error) {
	err = a.db.QueryRow(ctx, `SELECT id, name FROM tenants WHERE slug=$1 AND status='ACTIVE'`, slug).Scan(&tenantID, &tenantName)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return tenantID, tenantName, true, nil
}

func (a *App) writePublicProducts(w http.ResponseWriter, r *http.Request, tenantID, slug, tenantName string) {
	rows, err := a.db.Query(r.Context(), `SELECT p.id, ''::text, ''::text, ''::text, ''::text, `+productListFieldsSQL+` FROM products p WHERE p.tenant_id=$1 AND p.status='ACTIVE' ORDER BY p.created_at DESC`, tenantID)
	if err != nil {
		a.logError(r, "list public products failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Products could not be loaded.")
		return
	}
	defer rows.Close()
	items := make([]product, 0)
	if err := scanProducts(rows, &items); err != nil {
		a.logError(r, "iterate public products failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Products could not be loaded.")
		return
	}
	if slug != "" {
		for index := range items {
			items[index].TenantName = tenantName
			rewriteProductForSpace(&items[index], slug)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "space_name": tenantName})
}

func (a *App) publicProduct(w http.ResponseWriter, r *http.Request) {
	a.writePublicProduct(w, r, defaultTenantID, "")
}

func (a *App) publicSpaceProduct(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "spaceSlug")
	tenantID, tenantName, ok, err := a.resolveActiveTenantBySlug(r.Context(), slug)
	if err != nil {
		a.logError(r, "load public space failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "This product could not be loaded.")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "space_not_found", "This space is not available.")
		return
	}
	_ = tenantName
	a.writePublicProduct(w, r, tenantID, slug)
}

func (a *App) writePublicProduct(w http.ResponseWriter, r *http.Request, tenantID, slug string) {
	item := product{}
	err := scanProduct(a.db.QueryRow(r.Context(), `SELECT p.id, ''::text, ''::text, ''::text, ''::text, `+productListFieldsSQL+` FROM products p WHERE p.tenant_id=$1 AND p.id=$2 AND p.status='ACTIVE'`, tenantID, productID(r)), &item)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "product_not_found", "This product is not available.")
		return
	}
	if err != nil {
		a.logError(r, "load public product failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "This product could not be loaded.")
		return
	}
	if slug != "" {
		rewriteProductForSpace(&item, slug)
	}
	writeJSON(w, http.StatusOK, item)
}

func (a *App) requireOperator(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		claims, valid := verifyOperatorToken(a.cfg.OperatorTokenKey, token)
		if token == "" || !valid {
			writeError(w, http.StatusUnauthorized, "operator_auth_required", "Operator sign-in is required.")
			return
		}

		identity := operatorIdentity{SessionID: claims.SessionID}
		err := a.db.QueryRow(r.Context(), `
			SELECT u.id, u.username, u.display_name, COALESCE(u.platform_role, ''),
			       m.id, m.tenant_id, t.name, t.slug, m.role, m.status,
			       COALESCE(ja.id, ''), COALESCE(ja.status, '')
			FROM operator_sessions s
			JOIN operator_users u ON u.id=s.user_id AND u.status='ACTIVE' AND u.token_version=s.token_version
			JOIN tenant_memberships m ON m.id=s.membership_id AND m.user_id=u.id AND m.tenant_id=s.tenant_id AND m.status IN ('PENDING', 'ACTIVE')
			JOIN tenants t ON t.id=s.tenant_id AND t.status='ACTIVE'
			LEFT JOIN join_applications ja ON ja.operator_user_id=u.id
			WHERE s.id=$1 AND s.user_id=$2 AND s.token_hash=$3 AND s.revoked_at IS NULL AND s.expires_at>NOW()`,
			claims.SessionID, claims.UserID, hashToken(token),
		).Scan(&identity.UserID, &identity.Username, &identity.DisplayName, &identity.PlatformRole, &identity.MembershipID, &identity.TenantID, &identity.TenantName, &identity.TenantSlug, &identity.Role, &identity.MembershipStatus, &identity.ApplicationID, &identity.ApplicationStatus)
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				a.logError(r, "validate operator session failed", err)
			}
			writeError(w, http.StatusUnauthorized, "operator_auth_required", "Operator sign-in is required.")
			return
		}
		identity.Permissions = permissionNames(identity)
		a.touchOperatorSession(r, identity.SessionID)
		ctx := context.WithValue(r.Context(), operatorContextKey{}, identity)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func operatorIdentityFromContext(ctx context.Context) (operatorIdentity, bool) {
	identity, ok := ctx.Value(operatorContextKey{}).(operatorIdentity)
	return identity, ok
}

func operatorActor(r *http.Request) string {
	if identity, ok := operatorIdentityFromContext(r.Context()); ok {
		return identity.Username
	}
	return "unknown-operator"
}
