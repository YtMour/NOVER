package api

import (
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

var assignableMemberRoles = map[string]struct{}{
	"ADMIN": {}, "EDITOR": {}, "FULFILLMENT": {}, "ANALYST": {},
}

type tenantMember struct {
	MembershipID string     `json:"membership_id"`
	UserID       string     `json:"user_id"`
	Username     string     `json:"username"`
	DisplayName  string     `json:"display_name"`
	Role         string     `json:"role"`
	PlatformRole string     `json:"platform_role"`
	Status       string     `json:"status"`
	JoinedAt     time.Time  `json:"joined_at"`
	LastActiveAt *time.Time `json:"last_active_at"`
}

const tenantMemberSelect = `SELECT m.id, u.id, u.username, u.display_name, m.role, COALESCE(u.platform_role, ''), m.status, m.created_at,
	       (SELECT MAX(s.last_seen_at) FROM operator_sessions s WHERE s.user_id=u.id AND s.tenant_id=m.tenant_id)
	FROM tenant_memberships m
	JOIN operator_users u ON u.id=m.user_id`

func scanTenantMember(row interface{ Scan(...any) error }, item *tenantMember) error {
	return row.Scan(&item.MembershipID, &item.UserID, &item.Username, &item.DisplayName, &item.Role, &item.PlatformRole, &item.Status, &item.JoinedAt, &item.LastActiveAt)
}

func (a *App) listTenantMembers(w http.ResponseWriter, r *http.Request) {
	identity, _ := operatorIdentityFromContext(r.Context())
	rows, err := a.db.Query(r.Context(), tenantMemberSelect+` WHERE m.tenant_id=$1 ORDER BY CASE m.role WHEN 'OWNER' THEN 0 WHEN 'ADMIN' THEN 1 ELSE 2 END, u.username`, identity.TenantID)
	if err != nil {
		a.logError(r, "list tenant members failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Tenant members could not be loaded.")
		return
	}
	defer rows.Close()
	items := make([]tenantMember, 0)
	for rows.Next() {
		var item tenantMember
		if err := scanTenantMember(rows, &item); err != nil {
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

type memberCreateInput struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
	Role        string `json:"role"`
}

func (input *memberCreateInput) normalize() {
	input.Username = strings.ToLower(strings.TrimSpace(input.Username))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Password = strings.TrimSpace(input.Password)
	input.Role = strings.ToUpper(strings.TrimSpace(input.Role))
}

func (input memberCreateInput) valid() bool {
	if !desiredUsernamePattern.MatchString(input.Username) {
		return false
	}
	if runes := utf8.RuneCountInString(input.DisplayName); runes < 2 || runes > 80 {
		return false
	}
	if len(input.Password) < 8 || len(input.Password) > 200 {
		return false
	}
	_, ok := assignableMemberRoles[input.Role]
	return ok
}

func (a *App) createTenantMember(w http.ResponseWriter, r *http.Request) {
	if allowed, retry := a.allowRequest(r, "member-create", 20, time.Hour); !allowed {
		setRetryAfter(w, retry)
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many member invitations. Please wait and try again.")
		return
	}
	var input memberCreateInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Enter valid member details.")
		return
	}
	input.normalize()
	if !input.valid() {
		writeError(w, http.StatusBadRequest, "invalid_member", "Check the username, display name, password (at least 8 characters), and role.")
		return
	}
	identity, _ := operatorIdentityFromContext(r.Context())
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		a.logError(r, "hash member password failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The member could not be created.")
		return
	}
	userID, err := randomID()
	if err != nil {
		a.logError(r, "create member user id failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The member could not be created.")
		return
	}
	membershipID, err := randomID()
	if err != nil {
		a.logError(r, "create member membership id failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The member could not be created.")
		return
	}
	userID = "user-" + userID
	membershipID = "membership-" + membershipID
	item := tenantMember{
		MembershipID: membershipID,
		UserID:       userID,
		Username:     input.Username,
		DisplayName:  input.DisplayName,
		Role:         input.Role,
		Status:       "ACTIVE",
		JoinedAt:     time.Now().UTC(),
	}

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.logError(r, "begin member creation failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The member could not be created.")
		return
	}
	defer tx.Rollback(r.Context())
	_, err = tx.Exec(r.Context(), `INSERT INTO operator_users (id, username, display_name, password_hash, status) VALUES ($1,$2,$3,$4,'ACTIVE')`, userID, input.Username, input.DisplayName, string(passwordHash))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeError(w, http.StatusConflict, "member_exists", "This username is already in use.")
			return
		}
		a.logError(r, "create member user failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The member could not be created.")
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO tenant_memberships (id, tenant_id, user_id, role, status) VALUES ($1,$2,$3,$4,'ACTIVE')`, membershipID, identity.TenantID, userID, input.Role); err != nil {
		a.logError(r, "create member membership failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The member could not be created.")
		return
	}
	if err = recordMemberAudit(r, tx, identity.TenantID, "MEMBER_CREATED", membershipID, map[string]string{"username": input.Username, "role": input.Role, "status": "ACTIVE"}); err != nil {
		a.logError(r, "record member creation failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The member could not be created.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.logError(r, "commit member creation failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The member could not be created.")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

type memberUpdateInput struct {
	Role   string `json:"role"`
	Status string `json:"status"`
}

func (a *App) updateTenantMember(w http.ResponseWriter, r *http.Request) {
	var input memberUpdateInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Enter a valid member update.")
		return
	}
	input.Role = strings.ToUpper(strings.TrimSpace(input.Role))
	input.Status = strings.ToUpper(strings.TrimSpace(input.Status))
	if input.Role == "" && input.Status == "" {
		writeError(w, http.StatusBadRequest, "invalid_member", "Choose a role or status to update.")
		return
	}
	if input.Role != "" {
		if _, ok := assignableMemberRoles[input.Role]; !ok {
			writeError(w, http.StatusBadRequest, "invalid_member", "Choose a valid member role.")
			return
		}
	}
	if input.Status != "" && input.Status != "ACTIVE" && input.Status != "SUSPENDED" {
		writeError(w, http.StatusBadRequest, "invalid_member", "Choose ACTIVE or SUSPENDED status.")
		return
	}
	identity, _ := operatorIdentityFromContext(r.Context())
	membershipID := chi.URLParam(r, "membershipID")
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.logError(r, "begin member update failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The member could not be updated.")
		return
	}
	defer tx.Rollback(r.Context())
	var item tenantMember
	err = tx.QueryRow(r.Context(), tenantMemberSelect+` WHERE m.id=$1 AND m.tenant_id=$2 FOR UPDATE OF m`, membershipID, identity.TenantID).
		Scan(&item.MembershipID, &item.UserID, &item.Username, &item.DisplayName, &item.Role, &item.PlatformRole, &item.Status, &item.JoinedAt, &item.LastActiveAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "member_not_found", "This member was not found.")
		return
	}
	if err != nil {
		a.logError(r, "load member for update failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The member could not be updated.")
		return
	}
	if item.UserID == identity.UserID {
		writeError(w, http.StatusConflict, "member_self_forbidden", "You cannot change your own membership.")
		return
	}
	if item.Role == "OWNER" {
		writeError(w, http.StatusConflict, "member_owner_protected", "The space owner cannot be changed here.")
		return
	}
	if input.Role != "" {
		item.Role = input.Role
	}
	if input.Status != "" {
		item.Status = input.Status
	}
	if _, err := tx.Exec(r.Context(), `UPDATE tenant_memberships SET role=$3, status=$4, updated_at=NOW() WHERE id=$1 AND tenant_id=$2`, item.MembershipID, identity.TenantID, item.Role, item.Status); err != nil {
		a.logError(r, "update member failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The member could not be updated.")
		return
	}
	if item.Status == "SUSPENDED" {
		if _, err := tx.Exec(r.Context(), `UPDATE operator_sessions SET revoked_at=NOW() WHERE user_id=$1 AND tenant_id=$2 AND revoked_at IS NULL`, item.UserID, identity.TenantID); err != nil {
			a.logError(r, "revoke suspended member sessions failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "The member could not be updated.")
			return
		}
	}
	if err := recordMemberAudit(r, tx, identity.TenantID, "MEMBER_UPDATED", item.MembershipID, map[string]string{"role": item.Role, "status": item.Status}); err != nil {
		a.logError(r, "record member update failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The member could not be updated.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.logError(r, "commit member update failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The member could not be updated.")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func recordMemberAudit(r *http.Request, tx pgx.Tx, tenantID, action, resourceID string, metadata map[string]string) error {
	eventID, err := randomID()
	if err != nil {
		return err
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO admin_events (id, tenant_id, actor, action, resource_type, resource_id, metadata_json) VALUES ($1,$2,$3,$4,'member',$5,$6::jsonb)`, eventID, tenantID, operatorActor(r), action, resourceID, string(payload))
	return err
}
