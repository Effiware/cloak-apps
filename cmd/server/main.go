package main

import (
	"context"
	"log"
	"strconv"

	"github.com/effiware/cloak-apps/internal/config"
	"github.com/effiware/cloak-apps/internal/keycloak"
	"github.com/effiware/cloak-apps/internal/server"
	"github.com/effiware/cloak-apps/internal/server/auth"
	"github.com/effiware/cloak-apps/internal/server/session"
	"github.com/effiware/cloak-apps/internal/services"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize context
	ctx := context.Background()

	// Initialize Keycloak client
	keycloakClient, err := keycloak.NewClient(
		ctx,
		cfg.Keycloak.Url,
		cfg.Keycloak.Realm,
		cfg.Keycloak.ClientId,
		cfg.Keycloak.ClientSecret,
		cfg.Keycloak.RedirectUri,
	)
	if err != nil {
		log.Fatalf("Failed to create Keycloak client: %v", err)
	}
	log.Printf("Keycloak client initialized for realm: %s", cfg.Keycloak.Realm)

	// Convert MaxAge from string to int
	maxAge, err := strconv.Atoi(cfg.Session.MaxAge)
	if err != nil {
		log.Fatalf("Invalid session max_age: %v", err)
	}

	// Initialize session store
	sessionStore := session.NewStore(cfg.Session.Secret, maxAge)
	log.Printf("Session store initialized with max age: %d seconds", maxAge)

	// Initialize auth handlers
	authHandlers := auth.NewHandlers(
		keycloakClient,
		sessionStore,
		cfg.Keycloak.Url,
		cfg.Keycloak.Realm,
	)
	log.Println("Auth handlers initialized")

	// Initialize application service
	appService, err := services.NewApplicationService(keycloakClient.AdminClient)
	if err != nil {
		log.Fatalf("Failed to create application service: %v", err)
	}
	log.Println("Application service initialized")

	// Create HTTP server
	httpServer := server.HttpServer(
		cfg.Server.Host,
		cfg.Server.Port,
		cfg.Server.Timeout,
		keycloakClient,
		authHandlers,
		sessionStore,
		appService,
	)

	log.Printf("Starting server on %s", httpServer.Addr)
	log.Printf("Keycloak URL: %s/realms/%s", cfg.Keycloak.Url, cfg.Keycloak.Realm)
	log.Printf("Redirect URI: %s", cfg.Keycloak.RedirectUri)

	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
