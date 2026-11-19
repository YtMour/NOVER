package api

import (
	"net/http"

	"github.com/YtMour/NOVER/server/internal/buildinfo"
)

// meta is a public, cache-free build identity endpoint. It is intentionally
// unauthenticated and excludes secrets or tenant data so operators, clients,
// and agents can confirm which build is serving requests.
func (a *App) meta(w http.ResponseWriter, _ *http.Request) {
	payload := buildinfo.Info()
	payload["name"] = "nover"
	payload["environment"] = a.cfg.Environment
	writeJSON(w, http.StatusOK, payload)
}
