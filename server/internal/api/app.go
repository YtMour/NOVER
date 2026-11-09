package api

import (
	"log/slog"
	"net/http"

	"github.com/YtMour/NOVER/server/internal/config"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

type App struct {
	db     *pgxpool.Pool
	cfg    config.Config
	logger *slog.Logger
	limit  *rateLimiter
}

func New(pool *pgxpool.Pool, cfg config.Config, logger *slog.Logger) *App {
	return &App{db: pool, cfg: cfg, logger: logger, limit: newRateLimiter()}
}

func (a *App) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(a.cors)
	r.Get("/healthz", a.health)
	r.Get("/readyz", a.ready)
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/products", a.listPublicProducts)
		r.Post("/ops/login", a.operatorLogin)
		r.Post("/redeem", a.redeem)
		r.Get("/delivery", a.delivery)
		r.Post("/assets/{assetID}/ticket", a.createDownloadTicket)
		r.Get("/downloads/{token}", a.download)
		r.Route("/ops", func(r chi.Router) {
			r.Use(a.requireOperator)
			r.Get("/session", a.operatorSession)
			r.Get("/products", a.listProducts)
			r.Get("/activity", a.adminActivity)
			r.Post("/products", a.createProduct)
			r.Post("/products/{productID}/assets", a.uploadAsset)
			r.Post("/products/{productID}/codes", a.createCodes)
			r.Post("/codes/{codeID}/action", a.updateCodeAction)
		})
	})
	return r
}

func (a *App) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'self'; object-src 'none'; frame-ancestors 'none'; form-action 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'")
		origin := r.Header.Get("Origin")
		if origin != "" && origin == a.cfg.AllowedOrigin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
