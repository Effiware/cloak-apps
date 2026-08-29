package server

import (
	"net/http"

	"github.com/effiware/cloak-apps/internal"
	_ "github.com/effiware/cloak-apps/internal/docs"
	"github.com/effiware/cloak-apps/internal/server/api"
	"github.com/effiware/cloak-apps/internal/server/hda"
	mw "github.com/effiware/cloak-apps/internal/server/middlewares"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/riandyrn/otelchi"
	httpSwagger "github.com/swaggo/http-swagger"
)

func (hdaAndApi *HdaAndApi) RegisterRoutes() *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Heartbeat("/ping"))
	// otelchi first: the two below need the span in the request context
	r.Use(otelchi.Middleware("cloak-apps", otelchi.WithChiRoutes(r)))
	r.Use(mw.RequestLogger)
	r.Use(mw.Recoverer)

	// Public routes
	r.Handle("/static/*", http.FileServer(http.FS(internal.StaticFiles)))

	// Auth routes (public)
	r.Get("/auth/login", hdaAndApi.authHandlers.HandleLogin)
	r.Get("/auth/callback", hdaAndApi.authHandlers.HandleCallback)
	r.Get("/auth/logout", hdaAndApi.authHandlers.HandleLogout)

	// Metrics endpoint (public, secured via K8s NetworkPolicy)
	if hdaAndApi.prometheusRegistry != nil {
		r.Handle("/metrics", promhttp.HandlerFor(hdaAndApi.prometheusRegistry, promhttp.HandlerOpts{
			EnableOpenMetrics: true,
		}))
	}

	// Protected routes (require authentication)
	r.Group(func(r chi.Router) {
		r.Use(mw.AuthRequired(hdaAndApi.keycloakClient, hdaAndApi.sessionStore))

		// Auth routes (protected)
		r.Get("/auth/sso-redirect", hdaAndApi.authHandlers.HandleSSORedirect)

		// HDA routes
		r.HandleFunc("/", hda.WithJsonFallback(hda.RenderIndex(hdaAndApi.orgService)))
		r.HandleFunc("/hda/applications", hda.WithJsonFallback(hda.RenderApplications(hdaAndApi.appService)))

		// API routes
		r.Get("/api/v1/organization", api.JsonHandler(api.GetOrganization(hdaAndApi.orgService)))
		r.Get("/api/v1/applications", api.JsonHandler(api.GetApplications(hdaAndApi.appService)))
		if hdaAndApi.swaggerEnabled {
			r.Handle("/docs/*", http.FileServer(http.FS(internal.DocsFS)))
			r.Get("/swagger/*", httpSwagger.Handler(httpSwagger.URL("/docs/swagger.json")))
		}
	})

	return r
}
