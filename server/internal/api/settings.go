package api

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var webPathPattern = regexp.MustCompile(`^/[a-z0-9][a-z0-9_-]{3,63}$`)
var apiPathPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{3,63}$`)

type platformSettings struct {
	DeploymentMode  string    `json:"deployment_mode"`
	StorefrontURL   string    `json:"storefront_url"`
	ConsoleURL      string    `json:"console_url"`
	APIBaseURL      string    `json:"api_base_url"`
	OperatorWebPath string    `json:"operator_web_path"`
	OperatorAPIPath string    `json:"operator_api_path"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (settings *platformSettings) normalize() {
	settings.DeploymentMode = strings.ToUpper(strings.TrimSpace(settings.DeploymentMode))
	settings.StorefrontURL = strings.TrimRight(strings.TrimSpace(settings.StorefrontURL), "/")
	settings.ConsoleURL = strings.TrimRight(strings.TrimSpace(settings.ConsoleURL), "/")
	settings.APIBaseURL = strings.TrimRight(strings.TrimSpace(settings.APIBaseURL), "/")
	settings.OperatorWebPath = strings.ToLower(strings.TrimSpace(settings.OperatorWebPath))
	settings.OperatorAPIPath = strings.ToLower(strings.Trim(strings.TrimSpace(settings.OperatorAPIPath), "/"))
}

func validHTTPURL(value string) bool {
	parsed, err := url.ParseRequestURI(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func (settings platformSettings) valid() bool {
	validMode := settings.DeploymentMode == "COLOCATED" || settings.DeploymentMode == "SPLIT_ORIGIN" || settings.DeploymentMode == "SPLIT_APPS"
	return validMode && validHTTPURL(settings.StorefrontURL) && validHTTPURL(settings.ConsoleURL) && validHTTPURL(settings.APIBaseURL) && webPathPattern.MatchString(settings.OperatorWebPath) && apiPathPattern.MatchString(settings.OperatorAPIPath)
}

func (a *App) getPlatformSettings(w http.ResponseWriter, r *http.Request) {
	var item platformSettings
	err := a.db.QueryRow(r.Context(), `SELECT deployment_mode, storefront_url, console_url, api_base_url, operator_web_path, operator_api_path, updated_at FROM platform_settings WHERE id='default'`).Scan(&item.DeploymentMode, &item.StorefrontURL, &item.ConsoleURL, &item.APIBaseURL, &item.OperatorWebPath, &item.OperatorAPIPath, &item.UpdatedAt)
	if err != nil {
		a.logError(r, "load platform settings failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Platform settings could not be loaded.")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (a *App) updatePlatformSettings(w http.ResponseWriter, r *http.Request) {
	var input platformSettings
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}
	input.normalize()
	if !input.valid() {
		writeError(w, http.StatusBadRequest, "invalid_platform_settings", "Enter valid HTTP URLs and route paths between 4 and 64 characters.")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Platform settings could not be saved.")
		return
	}
	defer tx.Rollback(r.Context())
	err = tx.QueryRow(r.Context(), `UPDATE platform_settings SET deployment_mode=$1, storefront_url=$2, console_url=$3, api_base_url=$4, operator_web_path=$5, operator_api_path=$6, updated_at=NOW() WHERE id='default' RETURNING updated_at`, input.DeploymentMode, input.StorefrontURL, input.ConsoleURL, input.APIBaseURL, input.OperatorWebPath, input.OperatorAPIPath).Scan(&input.UpdatedAt)
	if err == nil {
		var eventID string
		eventID, err = randomID()
		if err == nil {
			_, err = tx.Exec(r.Context(), `INSERT INTO admin_events (id, actor, action, resource_type, resource_id, metadata_json) VALUES ($1,$2,'PLATFORM_SETTINGS_UPDATED','platform_settings','default',jsonb_build_object('deployment_mode',$3::text,'operator_web_path',$4::text,'operator_api_path',$5::text))`, eventID, a.cfg.AdminUsername, input.DeploymentMode, input.OperatorWebPath, input.OperatorAPIPath)
		}
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		a.logError(r, "save platform settings failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Platform settings could not be saved.")
		return
	}
	writeJSON(w, http.StatusOK, input)
}
