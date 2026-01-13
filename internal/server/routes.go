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
	httpSwagger "github.com/swaggo/http-swagger"
)

func (hdaAndApi *HdaAndApi) RegisterRoutes() *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Heartbeat("/ping"))
	r.Use(middleware.Logger)

	// Public routes
	r.Handle("/docs/*", http.FileServer(http.FS(internal.DocsFS)))
	r.Get("/swagger/*", httpSwagger.Handler(httpSwagger.URL("/docs/swagger.json")))
	r.Handle("/static/*", http.FileServer(http.FS(internal.StaticFiles)))

	// Auth routes (public)
	r.Get("/auth/login", hdaAndApi.authHandlers.HandleLogin)
	r.Get("/auth/callback", hdaAndApi.authHandlers.HandleCallback)
	r.Get("/auth/logout", hdaAndApi.authHandlers.HandleLogout)

	// Protected routes (require authentication)
	r.Group(func(r chi.Router) {
		r.Use(mw.AuthRequired(hdaAndApi.keycloakClient, hdaAndApi.sessionStore))

		// Auth routes (protected)
		r.Get("/auth/sso-redirect", hdaAndApi.authHandlers.HandleSSORedirect)

		// HDA routes
		r.HandleFunc("/", hda.WithJsonFallback(hda.RenderRoot(hdaAndApi.orgService, hdaAndApi.appService)))

		// API routes
		r.Get("/api/v1/organization", api.JsonHandler(api.GetOrganization(hdaAndApi.orgService)))
		r.Get("/api/v1/applications", api.JsonHandler(api.GetApplications(hdaAndApi.appService)))
	})

	return r
}
