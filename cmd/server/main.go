package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/effiware/cloak-apps/internal/config"
	"github.com/effiware/cloak-apps/internal/keycloak"
	"github.com/effiware/cloak-apps/internal/server"
	"github.com/effiware/cloak-apps/internal/server/auth"
	"github.com/effiware/cloak-apps/internal/server/session"
	"github.com/effiware/cloak-apps/internal/services"
)

func main() {
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
	slog.Info("Initialized slog with", "level", level)

	ctx := context.Background()
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

	// Enable token introspection for cookie store (required for Tier 2)
	if cfg.Session.Store == "cookie" {
		keycloakClient.EnableIntrospection(time.Duration(cfg.Session.IntrospectionCacheTTL) * time.Second)
		slog.Info("Token introspection enabled (cookie store requires it)", "cache_ttl_seconds", cfg.Session.IntrospectionCacheTTL)
	}

	sessionStore, err := session.NewStore(session.StoreConfig{
		Secret:    cfg.Session.Secret,
		MaxAge:    cfg.Session.MaxAge,
		Secure:    cfg.Session.Secure,
		StoreType: cfg.Session.Store,
		RedisURL:  cfg.Session.RedisURL,
	})
	if err != nil {
		slog.Error("Failed to create session store,", "error", err)
		os.Exit(1)
	}
	slog.Info("Session store initialized", "type", cfg.Session.Store, "max_age", cfg.Session.MaxAge, "secure", cfg.Session.Secure)

	authHandlers := auth.NewHandlers(keycloakClient, sessionStore)
	slog.Info("Auth handlers initialized")

	orgService, err := services.NewOrganizationService(services.OsOptions{
		Name:        cfg.Organization.Name,
		HomeUrl:     cfg.Organization.HomeUrl,
		Description: cfg.Organization.CustomDescription,
	})
	if err != nil {
		slog.Error("Failed to create organization service,", "error", err)
		os.Exit(1)
	}

	appService, err := services.NewApplicationService(
		keycloakClient.AdminClient, cfg.Keycloak.ClientId, cfg.Server.RefreshIntervalMin,
	)
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
