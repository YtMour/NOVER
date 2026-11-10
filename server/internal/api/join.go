package api

import (
	"errors"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

var desiredUsernamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{3,31}$`)

type joinApplication struct {
	ID               string     `json:"id"`
	TenantID         string     `json:"tenant_id,omitempty"`
	OperatorUserID   string     `json:"operator_user_id,omitempty"`
	OrganizationName string     `json:"organization_name"`
	ContactName      string     `json:"contact_name"`
	Email            string     `json:"email"`
	DesiredUsername  string     `json:"desired_username"`
	UseCase          string     `json:"use_case"`
	Status           string     `json:"status"`
	Reviewer         string     `json:"reviewer"`
	ReviewedAt       *time.Time `json:"reviewed_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type joinApplicationInput struct {
	OrganizationName string `json:"organization_name"`
	ContactName      string `json:"contact_name"`
	Email            string `json:"email"`
	DesiredUsername  string `json:"desired_username"`
	ExpectedPassword string `json:"expected_password"`
	UseCase          string `json:"use_case"`
}

func (input *joinApplicationInput) normalize() {
	input.OrganizationName = strings.TrimSpace(input.OrganizationName)
	input.ContactName = strings.TrimSpace(input.ContactName)
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.DesiredUsername = strings.ToLower(strings.TrimSpace(input.DesiredUsername))
	input.ExpectedPassword = strings.TrimSpace(input.ExpectedPassword)
	input.UseCase = strings.TrimSpace(input.UseCase)
}

func (input joinApplicationInput) valid() bool {
	address, err := mail.ParseAddress(input.Email)
	return err == nil && address.Address == input.Email &&
		utf8.RuneCountInString(input.OrganizationName) >= 2 && utf8.RuneCountInString(input.OrganizationName) <= 120 &&
		utf8.RuneCountInString(input.ContactName) >= 2 && utf8.RuneCountInString(input.ContactName) <= 80 &&
		desiredUsernamePattern.MatchString(input.DesiredUsername) &&
		len(input.ExpectedPassword) >= 8 && len(input.ExpectedPassword) <= 200 &&
		utf8.RuneCountInString(input.UseCase) >= 10 && utf8.RuneCountInString(input.UseCase) <= 1200
}

func (a *App) createJoinApplication(w http.ResponseWriter, r *http.Request) {
	if !a.allowRequest(r, "join-application", 5, time.Hour) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Please wait before submitting another application.")
		return
	}
	var input joinApplicationInput
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}
	input.normalize()
	if !input.valid() {
		writeError(w, http.StatusBadRequest, "invalid_application", "Complete the organization, contact, email, username, and use case fields.")
		return
	}
	id, err := randomID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "The application could not be submitted.")
		return
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.ExpectedPassword), bcrypt.DefaultCost)
	if err != nil {
		a.logError(r, "hash applicant password failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The application could not be submitted.")
		return
	}
	userID := "user-" + id
	tenantID := "tenant-" + id
	membershipID := "membership-" + id
	item := joinApplication{ID: id, TenantID: tenantID, OperatorUserID: userID, OrganizationName: input.OrganizationName, ContactName: input.ContactName, Email: input.Email, DesiredUsername: input.DesiredUsername, UseCase: input.UseCase, Status: "PENDING"}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.logError(r, "begin join application failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The application could not be submitted.")
		return
	}
	defer tx.Rollback(r.Context())
	slug := input.DesiredUsername + "-" + strings.ToLower(id[:8])
	_, err = tx.Exec(r.Context(), `INSERT INTO tenants (id, name, slug, status) VALUES ($1,$2,$3,'ACTIVE')`, tenantID, input.OrganizationName, slug)
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO operator_users (id, username, display_name, password_hash) VALUES ($1,$2,$3,$4)`, userID, input.DesiredUsername, input.ContactName, string(passwordHash))
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO tenant_memberships (id, tenant_id, user_id, role, status) VALUES ($1,$2,$3,'OWNER','PENDING')`, membershipID, tenantID, userID)
	}
	if err == nil {
		err = tx.QueryRow(r.Context(), `INSERT INTO join_applications (id, tenant_id, operator_user_id, organization_name, contact_name, email, desired_username, use_case) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING created_at, updated_at`, item.ID, item.TenantID, item.OperatorUserID, item.OrganizationName, item.ContactName, item.Email, item.DesiredUsername, item.UseCase).Scan(&item.CreatedAt, &item.UpdatedAt)
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeError(w, http.StatusConflict, "application_exists", "This email address or account name is already in use.")
			return
		}
		a.logError(r, "create join application failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The application could not be submitted.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.logError(r, "commit join application failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The application could not be submitted.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": item.ID, "status": item.Status, "created_at": item.CreatedAt})
}

func (a *App) listJoinApplications(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `SELECT id, tenant_id, COALESCE(operator_user_id, ''), organization_name, contact_name, email, desired_username, use_case, status, reviewer, reviewed_at, created_at, updated_at FROM join_applications ORDER BY CASE status WHEN 'PENDING' THEN 0 WHEN 'REVIEWING' THEN 1 ELSE 2 END, created_at DESC`)
	if err != nil {
		a.logError(r, "list join applications failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Applications could not be loaded.")
		return
	}
	defer rows.Close()
	items := make([]joinApplication, 0)
	for rows.Next() {
		var item joinApplication
		if err := rows.Scan(&item.ID, &item.TenantID, &item.OperatorUserID, &item.OrganizationName, &item.ContactName, &item.Email, &item.DesiredUsername, &item.UseCase, &item.Status, &item.Reviewer, &item.ReviewedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			a.logError(r, "scan join application failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Applications could not be loaded.")
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		a.logError(r, "iterate join applications failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Applications could not be loaded.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *App) reviewJoinApplication(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Action string `json:"action"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}
	input.Action = strings.ToUpper(strings.TrimSpace(input.Action))
	nextStatus, ok := map[string]string{"REVIEW": "REVIEWING", "APPROVE": "APPROVED", "REJECT": "REJECTED"}[input.Action]
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_review_action", "Choose REVIEW, APPROVE, or REJECT.")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "The application could not be reviewed.")
		return
	}
	defer tx.Rollback(r.Context())
	var item joinApplication
	var currentStatus string
	err = tx.QueryRow(r.Context(), `SELECT id, tenant_id, COALESCE(operator_user_id, ''), organization_name, contact_name, email, desired_username, use_case, status, reviewer, reviewed_at, created_at, updated_at FROM join_applications WHERE id=$1 FOR UPDATE`, chi.URLParam(r, "applicationID")).Scan(&item.ID, &item.TenantID, &item.OperatorUserID, &item.OrganizationName, &item.ContactName, &item.Email, &item.DesiredUsername, &item.UseCase, &currentStatus, &item.Reviewer, &item.ReviewedAt, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "application_not_found", "The application was not found.")
		return
	}
	if (input.Action == "REVIEW" && currentStatus != "PENDING" && currentStatus != "REVIEWING") || (input.Action == "APPROVE" && currentStatus == "REJECTED") || (input.Action == "REJECT" && currentStatus == "APPROVED") {
		writeError(w, http.StatusConflict, "application_status_conflict", "This application is no longer in a compatible review state.")
		return
	}
	item.Status = nextStatus
	item.Reviewer = operatorActor(r)
	if item.OperatorUserID != "" {
		switch input.Action {
		case "APPROVE":
			if _, err = tx.Exec(r.Context(), `UPDATE tenant_memberships SET status='ACTIVE', updated_at=NOW() WHERE tenant_id=$1 AND user_id=$2 AND status='PENDING'`, item.TenantID, item.OperatorUserID); err == nil {
				_, err = tx.Exec(r.Context(), `UPDATE tenants SET status='ACTIVE', updated_at=NOW() WHERE id=$1`, item.TenantID)
			}
		case "REJECT":
			if _, err = tx.Exec(r.Context(), `UPDATE tenant_memberships SET status='SUSPENDED', updated_at=NOW() WHERE tenant_id=$1 AND user_id=$2`, item.TenantID, item.OperatorUserID); err == nil {
				_, err = tx.Exec(r.Context(), `UPDATE operator_users SET status='SUSPENDED', updated_at=NOW() WHERE id=$1`, item.OperatorUserID)
			}
			if err == nil {
				_, err = tx.Exec(r.Context(), `UPDATE tenants SET status='SUSPENDED', updated_at=NOW() WHERE id=$1`, item.TenantID)
			}
		}
	}
	if err == nil {
		err = tx.QueryRow(r.Context(), `UPDATE join_applications SET status=$2, reviewer=$3, reviewed_at=NOW(), updated_at=NOW() WHERE id=$1 RETURNING reviewed_at, updated_at`, item.ID, nextStatus, item.Reviewer).Scan(&item.ReviewedAt, &item.UpdatedAt)
	}
	if err == nil {
		var eventID string
		eventID, err = randomID()
		if err == nil {
			_, err = tx.Exec(r.Context(), `INSERT INTO admin_events (id, tenant_id, actor, action, resource_type, resource_id, metadata_json) VALUES ($1,$2,$3,'JOIN_APPLICATION_REVIEWED','join_application',$4,jsonb_build_object('status',$5::text))`, eventID, item.TenantID, operatorActor(r), item.ID, item.Status)
		}
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		a.logError(r, "review join application failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The application could not be reviewed.")
		return
	}
	writeJSON(w, http.StatusOK, item)
}
