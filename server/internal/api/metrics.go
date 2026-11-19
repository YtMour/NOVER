package api

import "net/http"

// metrics serves the process-local Prometheus exposition text. It is public
// for scraping on the private API network; public deployments should restrict
// /metrics at the reverse proxy.
func (a *App) metricsHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(a.metrics.Render()))
}
