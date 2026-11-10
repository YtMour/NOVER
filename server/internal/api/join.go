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
)

var desiredUsernamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{3,31}$`)

type joinApplication struct {
	ID               string     `json:"id"`
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
	UseCase          string `json:"use_case"`
}

func (input *joinApplicationInput) normalize() {
	input.OrganizationName = strings.TrimSpace(input.OrganizationName)
	input.ContactName = strings.TrimSpace(input.ContactName)
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.DesiredUsername = strings.ToLower(strings.TrimSpace(input.DesiredUsername))
	input.UseCase = strings.TrimSpace(input.UseCase)
}

func (input joinApplicationInput) valid() bool {
	address, err := mail.ParseAddress(input.Email)
	return err == nil && address.Address == input.Email &&
		utf8.RuneCountInString(input.OrganizationName) >= 2 && utf8.RuneCountInString(input.OrganizationName) <= 120 &&
		utf8.RuneCountInString(input.ContactName) >= 2 && utf8.RuneCountInString(input.ContactName) <= 80 &&
		desiredUsernamePattern.MatchString(input.DesiredUsername) &&
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
	item := joinApplication{ID: id, OrganizationName: input.OrganizationName, ContactName: input.ContactName, Email: input.Email, DesiredUsername: input.DesiredUsername, UseCase: input.UseCase, Status: "PENDING"}
	err = a.db.QueryRow(r.Context(), `INSERT INTO join_applications (id, organization_name, contact_name, email, desired_username, use_case) VALUES ($1,$2,$3,$4,$5,$6) RETURNING created_at, updated_at`, item.ID, item.OrganizationName, item.ContactName, item.Email, item.DesiredUsername, item.UseCase).Scan(&item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeError(w, http.StatusConflict, "application_exists", "An open application already exists for this email address.")
			return
		}
		a.logError(r, "create join application failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The application could not be submitted.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": item.ID, "status": item.Status, "created_at": item.CreatedAt})
}

func (a *App) listJoinApplications(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `SELECT id, organization_name, contact_name, email, desired_username, use_case, status, reviewer, reviewed_at, created_at, updated_at FROM join_applications ORDER BY CASE status WHEN 'PENDING' THEN 0 WHEN 'REVIEWING' THEN 1 ELSE 2 END, created_at DESC`)
	if err != nil {
		a.logError(r, "list join applications failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Applications could not be loaded.")
		return
	}
	defer rows.Close()
	items := make([]joinApplication, 0)
	for rows.Next() {
		var item joinApplication
		if err := rows.Scan(&item.ID, &item.OrganizationName, &item.ContactName, &item.Email, &item.DesiredUsername, &item.UseCase, &item.Status, &item.Reviewer, &item.ReviewedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
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
	err = tx.QueryRow(r.Context(), `UPDATE join_applications SET status=$2, reviewer=$3, reviewed_at=NOW(), updated_at=NOW() WHERE id=$1 RETURNING id, organization_name, contact_name, email, desired_username, use_case, status, reviewer, reviewed_at, created_at, updated_at`, chi.URLParam(r, "applicationID"), nextStatus, operatorActor(r)).Scan(&item.ID, &item.OrganizationName, &item.ContactName, &item.Email, &item.DesiredUsername, &item.UseCase, &item.Status, &item.Reviewer, &item.ReviewedAt, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "application_not_found", "The application was not found.")
		return
	}
	if err == nil {
		var eventID string
		eventID, err = randomID()
		if err == nil {
			_, err = tx.Exec(r.Context(), `INSERT INTO admin_events (id, actor, action, resource_type, resource_id, metadata_json) VALUES ($1,$2,'JOIN_APPLICATION_REVIEWED','join_application',$3,jsonb_build_object('status',$4::text))`, eventID, operatorActor(r), item.ID, item.Status)
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
