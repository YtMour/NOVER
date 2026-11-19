package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

const (
	inviteTTL        = 72 * time.Hour
	passwordResetTTL = 2 * time.Hour
)

type invitationItem struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	Status      string    `json:"status"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
}

func newInvitationToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func (a *App) createMemberInvitation(w http.ResponseWriter, r *http.Request) {
	if allowed, retry := a.allowRequest(r, "member-invite", 30, time.Hour); !allowed {
		setRetryAfter(w, retry)
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many invitations. Please wait and try again.")
		return
	}
	var input struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Role        string `json:"role"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Enter valid invitation details.")
		return
	}
	input.Username = strings.ToLower(strings.TrimSpace(input.Username))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Role = strings.ToUpper(strings.TrimSpace(input.Role))
	if !desiredUsernamePattern.MatchString(input.Username) ||
		utf8.RuneCountInString(input.DisplayName) < 2 || utf8.RuneCountInString(input.DisplayName) > 80 ||
		!isAssignableMemberRole(input.Role) {
		writeError(w, http.StatusBadRequest, "invalid_member", "Check the username, display name, and role.")
		return
	}
	identity, _ := operatorIdentityFromContext(r.Context())

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.logError(r, "begin invitation failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The invitation could not be created.")
		return
	}
	defer tx.Rollback(r.Context())

	invitationID := mustRandomID()
	expiresAt := time.Now().UTC().Add(inviteTTL)
	token, err := newInvitationToken()
	if err != nil {
		a.logError(r, "generate invitation token failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The invitation could not be created.")
		return
	}

	var userID, membershipID string
	var existingUserID, existingDisplayName, existingMembershipID, existingRole, existingStatus string
	err = tx.QueryRow(r.Context(), `
		SELECT u.id, u.display_name, COALESCE(m.id, ''), COALESCE(m.role, ''), COALESCE(m.status, '')
		FROM operator_users u
		LEFT JOIN tenant_memberships m ON m.user_id=u.id AND m.tenant_id=$2
		WHERE LOWER(u.username)=LOWER($1)
		LIMIT 1`, input.Username, identity.TenantID).Scan(&existingUserID, &existingDisplayName, &existingMembershipID, &existingRole, &existingStatus)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		userID = "user-" + mustRandomID()
		membershipID = "membership-" + mustRandomID()
		// The account cannot be used until the invitation is accepted; the
		// stored password hash is a random value that nobody knows.
		temporarySecret := mustRandomID() + mustRandomID()
		passwordHash, hashErr := bcrypt.GenerateFromPassword([]byte(temporarySecret), bcrypt.DefaultCost)
		if hashErr != nil {
			a.logError(r, "hash invitation placeholder failed", hashErr)
			writeError(w, http.StatusInternalServerError, "internal_error", "The invitation could not be created.")
			return
		}
		if _, err := tx.Exec(r.Context(), `INSERT INTO operator_users (id, username, display_name, password_hash, status) VALUES ($1,$2,$3,$4,'ACTIVE')`, userID, input.Username, input.DisplayName, string(passwordHash)); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				writeError(w, http.StatusConflict, "member_exists", "This username is already in use.")
				return
			}
			a.logError(r, "create invited user failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "The invitation could not be created.")
			return
		}
		if _, err := tx.Exec(r.Context(), `INSERT INTO tenant_memberships (id, tenant_id, user_id, role, status) VALUES ($1,$2,$3,$4,'PENDING')`, membershipID, identity.TenantID, userID, input.Role); err != nil {
			a.logError(r, "create invited membership failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "The invitation could not be created.")
			return
		}
	case err != nil:
		a.logError(r, "check invitation username failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The invitation could not be created.")
		return
	default:
		// Re-invite a placeholder account whose previous invitation expired or
		// was revoked, but never overwrite an active or suspended membership.
		if existingMembershipID == "" || existingStatus != "PENDING" {
			writeError(w, http.StatusConflict, "member_exists", "This username is already in use.")
			return
		}
		userID = existingUserID
		membershipID = existingMembershipID
		if _, err := tx.Exec(r.Context(), `UPDATE operator_users SET display_name=$2, status='ACTIVE', updated_at=NOW() WHERE id=$1`, userID, input.DisplayName); err != nil {
			a.logError(r, "update invited user failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "The invitation could not be created.")
			return
		}
		if _, err := tx.Exec(r.Context(), `UPDATE tenant_memberships SET role=$2, updated_at=NOW() WHERE id=$1`, membershipID, input.Role); err != nil {
			a.logError(r, "update invited membership failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "The invitation could not be created.")
			return
		}
		if _, err := tx.Exec(r.Context(), `UPDATE operator_invitations SET revoked_at=NOW() WHERE user_id=$1 AND used_at IS NULL AND revoked_at IS NULL`, userID); err != nil {
			a.logError(r, "revoke old invitations failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "The invitation could not be created.")
			return
		}
	}
	if _, err := tx.Exec(r.Context(), `INSERT INTO operator_invitations (id, tenant_id, user_id, membership_id, kind, role, token_hash, created_by, expires_at) VALUES ($1,$2,$3,$4,'INVITE',$5,$6,$7,$8)`,
		invitationID, identity.TenantID, userID, membershipID, input.Role, hashToken(token), operatorActor(r), expiresAt); err != nil {
		a.logError(r, "save invitation failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The invitation could not be created.")
		return
	}
	if err := recordInvitationAudit(r, tx, identity.TenantID, "MEMBER_INVITED", invitationID, map[string]string{"username": input.Username, "role": input.Role}); err != nil {
		a.logError(r, "record invitation audit failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The invitation could not be created.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.logError(r, "commit invitation failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The invitation could not be created.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":           invitationID,
		"kind":         "INVITE",
		"username":     input.Username,
		"display_name": input.DisplayName,
		"role":         input.Role,
		"token":        token,
		"expires_at":   expiresAt,
	})
}

func (a *App) listMemberInvitations(w http.ResponseWriter, r *http.Request) {
	identity, _ := operatorIdentityFromContext(r.Context())
	rows, err := a.db.Query(r.Context(), `
		SELECT i.id, i.kind, u.username, u.display_name, i.role,
		       CASE
		           WHEN i.revoked_at IS NOT NULL THEN 'REVOKED'
		           WHEN i.used_at IS NOT NULL THEN 'USED'
		           WHEN i.expires_at < NOW() THEN 'EXPIRED'
		           ELSE 'PENDING'
		       END,
		       i.expires_at, i.created_at
		FROM operator_invitations i
		JOIN operator_users u ON u.id=i.user_id
		WHERE i.tenant_id=$1
		ORDER BY i.created_at DESC
		LIMIT 100`, identity.TenantID)
	if err != nil {
		a.logError(r, "list invitations failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Invitations could not be loaded.")
		return
	}
	defer rows.Close()
	items := make([]invitationItem, 0)
	for rows.Next() {
		var item invitationItem
		if err := rows.Scan(&item.ID, &item.Kind, &item.Username, &item.DisplayName, &item.Role, &item.Status, &item.ExpiresAt, &item.CreatedAt); err != nil {
			a.logError(r, "scan invitation failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Invitations could not be loaded.")
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		a.logError(r, "iterate invitations failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Invitations could not be loaded.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *App) revokeMemberInvitation(w http.ResponseWriter, r *http.Request) {
	identity, _ := operatorIdentityFromContext(r.Context())
	var userID string
	err := a.db.QueryRow(r.Context(), `UPDATE operator_invitations SET revoked_at=NOW() WHERE id=$1 AND tenant_id=$2 AND used_at IS NULL AND revoked_at IS NULL RETURNING user_id`, chi.URLParam(r, "invitationID"), identity.TenantID).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "invitation_not_found", "This invitation was not found or is no longer pending.")
		return
	}
	if err != nil {
		a.logError(r, "revoke invitation failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The invitation could not be revoked.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) createPasswordReset(w http.ResponseWriter, r *http.Request) {
	if allowed, retry := a.allowRequest(r, "password-reset-link", 30, time.Hour); !allowed {
		setRetryAfter(w, retry)
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many reset links. Please wait and try again.")
		return
	}
	identity, _ := operatorIdentityFromContext(r.Context())
	membershipID := chi.URLParam(r, "membershipID")
	var userID, role, username string
	err := a.db.QueryRow(r.Context(), `SELECT u.id, m.role, u.username FROM tenant_memberships m JOIN operator_users u ON u.id=m.user_id WHERE m.id=$1 AND m.tenant_id=$2`, membershipID, identity.TenantID).Scan(&userID, &role, &username)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "member_not_found", "This member was not found.")
		return
	}
	if err != nil {
		a.logError(r, "load member for password reset failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The reset link could not be created.")
		return
	}
	if userID == identity.UserID {
		writeError(w, http.StatusConflict, "member_self_forbidden", "Change your own password from the account security page.")
		return
	}
	expiresAt := time.Now().UTC().Add(passwordResetTTL)
	token, err := newInvitationToken()
	if err != nil {
		a.logError(r, "generate reset token failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The reset link could not be created.")
		return
	}
	invitationID := mustRandomID()
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.logError(r, "begin password reset failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The reset link could not be created.")
		return
	}
	defer tx.Rollback(r.Context())
	if _, err := tx.Exec(r.Context(), `UPDATE operator_invitations SET revoked_at=NOW() WHERE user_id=$1 AND kind='PASSWORD_RESET' AND used_at IS NULL AND revoked_at IS NULL`, userID); err != nil {
		a.logError(r, "revoke old reset links failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The reset link could not be created.")
		return
	}
	if _, err := tx.Exec(r.Context(), `INSERT INTO operator_invitations (id, tenant_id, user_id, membership_id, kind, role, token_hash, created_by, expires_at) VALUES ($1,$2,$3,$4,'PASSWORD_RESET',$5,$6,$7,$8)`,
		invitationID, identity.TenantID, userID, membershipID, role, hashToken(token), operatorActor(r), expiresAt); err != nil {
		a.logError(r, "save password reset failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The reset link could not be created.")
		return
	}
	if err := recordInvitationAudit(r, tx, identity.TenantID, "PASSWORD_RESET_LINK_CREATED", invitationID, map[string]string{"username": username}); err != nil {
		a.logError(r, "record password reset audit failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The reset link could not be created.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.logError(r, "commit password reset failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The reset link could not be created.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":         invitationID,
		"kind":       "PASSWORD_RESET",
		"username":   username,
		"token":      token,
		"expires_at": expiresAt,
	})
}

func (a *App) acceptInvitation(w http.ResponseWriter, r *http.Request) {
	if allowed, retry := a.allowRequest(r, "invitation-accept", 10, time.Hour); !allowed {
		setRetryAfter(w, retry)
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many attempts. Please wait and try again.")
		return
	}
	var input struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Enter a password to accept the invitation.")
		return
	}
	input.Token = strings.TrimSpace(input.Token)
	input.Password = strings.TrimSpace(input.Password)
	if input.Token == "" || len(input.Password) < 8 || len(input.Password) > 200 {
		writeError(w, http.StatusBadRequest, "invalid_password", "Choose a password between 8 and 200 characters.")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.logError(r, "begin invitation accept failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The invitation could not be accepted.")
		return
	}
	defer tx.Rollback(r.Context())

	var invitationID, tenantID, userID, membershipID, kind, username string
	var expiresAt time.Time
	var usedAt, revokedAt *time.Time
	err = tx.QueryRow(r.Context(), `
		SELECT i.id, i.tenant_id, i.user_id, i.membership_id, i.kind, u.username, i.expires_at, i.used_at, i.revoked_at
		FROM operator_invitations i
		JOIN operator_users u ON u.id=i.user_id
		WHERE i.token_hash=$1
		FOR UPDATE OF i`, hashToken(input.Token)).Scan(&invitationID, &tenantID, &userID, &membershipID, &kind, &username, &expiresAt, &usedAt, &revokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusBadRequest, "invalid_invitation", "This invitation link is not valid.")
		return
	}
	if err != nil {
		a.logError(r, "load invitation failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The invitation could not be accepted.")
		return
	}
	if usedAt != nil {
		writeError(w, http.StatusGone, "invitation_used", "This invitation has already been used.")
		return
	}
	if revokedAt != nil || !expiresAt.After(time.Now()) {
		writeError(w, http.StatusGone, "invitation_expired", "This invitation has expired. Ask an administrator for a new one.")
		return
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		a.logError(r, "hash invitation password failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The invitation could not be accepted.")
		return
	}
	if kind == "INVITE" {
		if _, err := tx.Exec(r.Context(), `UPDATE operator_users SET password_hash=$2, status='ACTIVE', updated_at=NOW() WHERE id=$1`, userID, string(passwordHash)); err != nil {
			a.logError(r, "activate invited user failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "The invitation could not be accepted.")
			return
		}
		if _, err := tx.Exec(r.Context(), `UPDATE tenant_memberships SET status='ACTIVE', updated_at=NOW() WHERE id=$1 AND status='PENDING'`, membershipID); err != nil {
			a.logError(r, "activate invited membership failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "The invitation could not be accepted.")
			return
		}
	} else {
		if _, err := tx.Exec(r.Context(), `UPDATE operator_users SET password_hash=$2, token_version=token_version+1, updated_at=NOW() WHERE id=$1`, userID, string(passwordHash)); err != nil {
			a.logError(r, "reset invited password failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "The invitation could not be accepted.")
			return
		}
		if _, err := tx.Exec(r.Context(), `UPDATE operator_sessions SET revoked_at=NOW() WHERE user_id=$1 AND revoked_at IS NULL`, userID); err != nil {
			a.logError(r, "revoke sessions after password reset failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "The invitation could not be accepted.")
			return
		}
	}
	if _, err := tx.Exec(r.Context(), `UPDATE operator_invitations SET used_at=NOW() WHERE id=$1`, invitationID); err != nil {
		a.logError(r, "mark invitation used failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The invitation could not be accepted.")
		return
	}
	action := "INVITATION_ACCEPTED"
	if kind == "PASSWORD_RESET" {
		action = "PASSWORD_RESET_COMPLETED"
	}
	if err := recordInvitationAudit(r, tx, tenantID, action, invitationID, map[string]string{"username": username}); err != nil {
		a.logError(r, "record invitation acceptance failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The invitation could not be accepted.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.logError(r, "commit invitation acceptance failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The invitation could not be accepted.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"kind": kind, "username": username})
}

func isAssignableMemberRole(role string) bool {
	_, ok := assignableMemberRoles[role]
	return ok
}

func mustRandomID() string {
	value, err := randomID()
	if err != nil {
		panic("randomID failed: " + err.Error())
	}
	return value
}

func recordInvitationAudit(r *http.Request, tx pgx.Tx, tenantID, action, resourceID string, metadata map[string]string) error {
	eventID, err := randomID()
	if err != nil {
		return err
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	actor := operatorActor(r)
	if actor == "unknown-operator" {
		actor = "self-service"
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO admin_events (id, tenant_id, actor, action, resource_type, resource_id, metadata_json) VALUES ($1,$2,$3,$4,'invitation',$5,$6::jsonb)`, eventID, tenantID, actor, action, resourceID, string(payload))
	return err
}
