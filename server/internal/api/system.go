package api

import (
	"net/http"
	"os"
	"time"

	"github.com/YtMour/NOVER/server/internal/buildinfo"
	"github.com/YtMour/NOVER/server/internal/db"
)

// systemStatus is an operator-facing summary used by the console deployment
// page to verify upgrades, migrations, database latency and storage health.
func (a *App) systemStatus(w http.ResponseWriter, r *http.Request) {
	dbReady := true
	latencyStart := time.Now()
	if err := a.db.Ping(r.Context()); err != nil {
		dbReady = false
	}
	latency := time.Since(latencyStart)

	plan, planErr := db.Plan(r.Context(), a.db)
	if planErr != nil {
		a.logError(r, "load migration plan failed", planErr)
	}
	storageWritable := false
	storagePath := a.cfg.StorageDir
	if storagePath != "" {
		if file, err := os.CreateTemp(storagePath, ".status-*"); err == nil {
			name := file.Name()
			_ = file.Close()
			_ = os.Remove(name)
			storageWritable = true
		}
	}

	payload := map[string]any{
		"app": map[string]any{
			"name":           "nover",
			"version":        buildinfo.Version,
			"commit":         buildinfo.Commit,
			"built_at":       buildinfo.BuiltAt,
			"environment":    a.cfg.Environment,
			"uptime_seconds": int64(time.Since(a.startedAt).Seconds()),
		},
		"database": map[string]any{
			"ready":      dbReady,
			"latency_ms": float64(latency.Microseconds()) / 1000.0,
			"migrations": map[string]any{
				"applied": plan.Applied,
				"pending": plan.Pending,
				"latest":  plan.Latest,
			},
			"schema_ready": plan.SchemaReady,
		},
		"storage": map[string]any{
			"path":     storagePath,
			"writable": storageWritable,
		},
	}
	if planErr != nil {
		payload["migrations_error"] = "migration status could not be read"
	}
	writeJSON(w, http.StatusOK, payload)
}
