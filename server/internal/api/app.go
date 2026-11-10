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
	operatorPath := "/" + a.cfg.OperatorAPIPath
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(a.cors)
	r.Get("/healthz", a.health)
	r.Get("/readyz", a.ready)
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/products", a.listPublicProducts)
		r.Get("/products/{productID}/images/{imageID}", a.serveProductImage)
		r.Post("/join-applications", a.createJoinApplication)
		r.Post(operatorPath+"/login", a.operatorLogin)
		r.Post("/redeem", a.redeem)
		r.Get("/delivery", a.delivery)
		r.Post("/assets/{assetID}/ticket", a.createDownloadTicket)
		r.Get("/downloads/{token}", a.download)
		r.Route(operatorPath, func(r chi.Router) {
			r.Use(a.requireOperator)
			r.Get("/session", a.operatorSession)
			r.Post("/session/revoke", a.revokeOperatorSession)
			r.With(a.requirePermission(permissionApplicationRead)).Get("/applications", a.listJoinApplications)
			r.With(a.requirePermission(permissionApplicationReview)).Post("/applications/{applicationID}/review", a.reviewJoinApplication)
			r.With(a.requirePermission(permissionSettingsRead)).Get("/settings", a.getPlatformSettings)
			r.With(a.requirePermission(permissionSettingsManage)).Put("/settings", a.updatePlatformSettings)
			r.With(a.requirePermission(permissionMemberRead)).Get("/members", a.listTenantMembers)
			r.With(a.requirePermission(permissionProductRead)).Get("/products", a.listProducts)
			r.With(a.requirePermission(permissionActivityRead)).Get("/activity", a.adminActivity)
			r.With(a.requirePermission(permissionProductWrite)).Post("/products", a.createProduct)
			r.With(a.requirePermission(permissionProductWrite)).Put("/products/{productID}", a.updateProduct)
			r.With(a.requirePermission(permissionProductPublish)).Post("/products/{productID}/status", a.updateProductStatus)
			r.With(a.requirePermission(permissionFulfillmentWrite)).Post("/products/{productID}/assets", a.uploadAsset)
			r.With(a.requirePermission(permissionProductRead)).Get("/products/{productID}/images", a.listProductImages)
			r.With(a.requirePermission(permissionProductWrite)).Post("/products/{productID}/images", a.uploadProductImage)
			r.With(a.requirePermission(permissionProductRead)).Get("/products/{productID}/images/{imageID}", a.serveOperatorProductImage)
			r.With(a.requirePermission(permissionProductWrite)).Delete("/products/{productID}/images/{imageID}", a.deleteProductImage)
			r.With(a.requirePermission(permissionProductRead)).Get("/products/{productID}/releases", a.listProductReleases)
			r.With(a.requirePermission(permissionProductWrite)).Post("/products/{productID}/releases", a.createProductRelease)
			r.With(a.requirePermission(permissionProductPublish)).Post("/products/{productID}/releases/{releaseID}/status", a.updateProductReleaseStatus)
			r.With(a.requirePermission(permissionCodeIssue)).Post("/products/{productID}/codes", a.createCodes)
			r.With(a.requirePermission(permissionCodeManage)).Post("/codes/{codeID}/action", a.updateCodeAction)
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
		if origin != "" && allowedOrigin(a.cfg.AllowedOrigins, origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func allowedOrigin(allowed []string, origin string) bool {
	for _, candidate := range allowed {
		if origin == candidate {
			return true
		}
	}
	return false
}
