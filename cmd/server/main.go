package main

import (
	"context"
	"log/slog"
	"os"
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
		slog.Error("Failed to load configuration file,", "error", err)
		os.Exit(1)
	}

	// Initialize global logger with desired level
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.Server.LogLevel)); err == nil {
		slog.SetLogLoggerLevel(level)
	} else {
		slog.Error("Error while unmarshalling log level,", "error", err)
	}
	slog.Info("Initialized log", "level", level)

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
		slog.Error("Failed to create Keycloak client,", "error", err)
		os.Exit(1)
	}
	slog.Info("Keycloak client initialized for", "realm", cfg.Keycloak.Realm)

	// Convert MaxAge from string to int
	maxAge, err := strconv.Atoi(cfg.Session.MaxAge)
	if err != nil {
		slog.Error("Invalid session max_age,", "error", err)
		os.Exit(1)
	}

	// Initialize session store
	sessionStore := session.NewStore(cfg.Session.Secret, maxAge)
	slog.Info("Session store initialized with", "max_age", maxAge)

	// Initialize auth handlers
	authHandlers := auth.NewHandlers(
		keycloakClient,
		sessionStore,
		cfg.Keycloak.Url,
		cfg.Keycloak.Realm,
	)
	slog.Info("Auth handlers initialized")

	orgService, err := services.NewOrganizationService(services.OsOptions{
		Name:        cfg.Organization.Name,
		HomeUrl:     cfg.Organization.HomeUrl,
		Description: cfg.Organization.CustomDescription,
	})
	if err != nil {
		slog.Error("Failed to create organization service,", "error", err)
	}

	appService, err := services.NewApplicationService(keycloakClient.AdminClient, cfg.Keycloak.ClientId)
	if err != nil {
		slog.Error("Failed to create application service,", "error", err)
		os.Exit(1)
	}

	// Create HTTP server
	httpServer := server.HttpServer(
		cfg.Server.Host,
		cfg.Server.Port,
		cfg.Server.Timeout,
		keycloakClient,
		authHandlers,
		sessionStore,
		orgService,
		appService,
	)

	slog.Info("Starting server,", "address", httpServer.Addr)
	slog.Info("Keycloak", "URL", cfg.Keycloak.Url+"/realms/"+cfg.Keycloak.Realm)
	slog.Info("Redirect", "URI", cfg.Keycloak.RedirectUri)

	if err := httpServer.ListenAndServe(); err != nil {
		slog.Error("Starting server failed", "error", err)
		os.Exit(1)
	}
}
