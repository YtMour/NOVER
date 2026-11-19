package api

import (
	"net/http"

	"github.com/YtMour/NOVER/server/internal/buildinfo"
)

// meta is a public, cache-free build identity endpoint. It is intentionally
// unauthenticated and excludes secrets or tenant data so operators, clients,
// and agents can confirm which build is serving requests.
func (a *App) meta(w http.ResponseWriter, r *http.Request) {
	payload := buildinfo.Info()
	payload["name"] = "nover"
	payload["environment"] = a.cfg.Environment
	if a.db != nil {
		var schemaVersion string
		if err := a.db.QueryRow(r.Context(), `SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1`).Scan(&schemaVersion); err == nil && schemaVersion != "" {
			payload["schema_version"] = schemaVersion
		}
	}
	writeJSON(w, http.StatusOK, payload)
}
