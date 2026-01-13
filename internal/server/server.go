package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	_ "github.com/effiware/cloak-apps/internal/docs"
	"github.com/effiware/cloak-apps/internal/keycloak"
	"github.com/effiware/cloak-apps/internal/server/auth"
	"github.com/effiware/cloak-apps/internal/server/session"
	"github.com/effiware/cloak-apps/internal/services"
)

type HdaAndApi struct {
	keycloakClient *keycloak.Client
	authHandlers   *auth.Handlers
	sessionStore   *session.Store
	orgService     *services.OrganizationService
	appService     *services.ApplicationService
}

func NewHdaAndApi(
	keycloakClient *keycloak.Client,
	authHandlers *auth.Handlers,
	sessionStore *session.Store,
	orgService *services.OrganizationService,
	appService *services.ApplicationService,
) *HdaAndApi {
	return &HdaAndApi{
		keycloakClient: keycloakClient,
		authHandlers:   authHandlers,
		sessionStore:   sessionStore,
		orgService:     orgService,
		appService:     appService,
	}
}

func HttpServer(
	host string,
	port int,
	timeout int,
	keycloakClient *keycloak.Client,
	authHandlers *auth.Handlers,
	sessionStore *session.Store,
	orgService *services.OrganizationService,
	appService *services.ApplicationService,
) *http.Server {
	hdaAndApi := NewHdaAndApi(keycloakClient, authHandlers, sessionStore, orgService, appService)
	readTimeout, writeTimout, idleTimeout := time.Duration(timeout), time.Duration(3*timeout), time.Duration(6*timeout)

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", host, port),
		Handler:      hdaAndApi.RegisterRoutes(),
		IdleTimeout:  idleTimeout * time.Second,
		ReadTimeout:  readTimeout * time.Second,
		WriteTimeout: writeTimout * time.Second,
	}

	server.RegisterOnShutdown(func() {
		appService.ShutDown()
		slog.Debug("Server shutdown complete")
	})

	return server
}
