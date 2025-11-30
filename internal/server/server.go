package server

import (
	"fmt"
	"log"
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
	appService     *services.ApplicationService
}

func NewHdaAndApi(
	keycloakClient *keycloak.Client,
	authHandlers *auth.Handlers,
	sessionStore *session.Store,
	appService *services.ApplicationService,
) *HdaAndApi {
	return &HdaAndApi{
		keycloakClient: keycloakClient,
		authHandlers:   authHandlers,
		sessionStore:   sessionStore,
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
	appService *services.ApplicationService,
) *http.Server {
	hdaAndApi := NewHdaAndApi(keycloakClient, authHandlers, sessionStore, appService)
	readTimeout, writeTimout, idleTimeout := time.Duration(timeout), time.Duration(3*timeout), time.Duration(6*timeout)

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", host, port),
		Handler:      hdaAndApi.RegisterRoutes(),
		IdleTimeout:  idleTimeout * time.Second,
		ReadTimeout:  readTimeout * time.Second,
		WriteTimeout: writeTimout * time.Second,
	}

	//server.RegisterOnShutdown(onServerShutdown)
	return server
}

func onServerShutdown() {
	// Add graceful shutdown logic if needed
	log.Println("Server shutdown complete")
}
