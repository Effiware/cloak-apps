package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/effiware/cloak-apps/internal/config"
	"github.com/effiware/cloak-apps/internal/keycloak"
	"github.com/effiware/cloak-apps/internal/server"
	"github.com/effiware/cloak-apps/internal/server/auth"
	"github.com/effiware/cloak-apps/internal/server/session"
	"github.com/effiware/cloak-apps/internal/services"
	"github.com/effiware/cloak-apps/internal/version"
	"github.com/effiware/cloak-apps/utils"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		slog.Error("Failed to load configuration file", "error", err)
		os.Exit(1)
	}

	bootLogger(cfg)

	otelResource := bootOtelResource(cfg)
	tracerProvider := bootOtel(cfg, otelResource)
	meterProvider, prometheusRegistry := bootMeter(cfg, otelResource)

	// Unconditional: with metrics off these bind to the global no-op meter. Skipping
	// them would leave the package-level instruments nil and panic on first use.
	keycloak.InitAdminMetrics()
	keycloak.InitIntrospectionMetrics()
	services.InitApplicationMetrics()
	utils.InitCacheMetrics()

	keycloakClient, err := keycloak.NewClient(
		context.Background(),
		cfg.Keycloak.Url,
		cfg.Keycloak.Realm,
		cfg.Keycloak.ClientId,
		cfg.Keycloak.ClientSecret,
		cfg.Keycloak.RedirectUri,
	)
	if err != nil {
		slog.Error("Failed to create Keycloak client", "error", err)
		os.Exit(1)
	}
	slog.Info("Keycloak client initialized", "realm", cfg.Keycloak.Realm)

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
		slog.Error("Failed to create session store", "error", err)
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
		slog.Error("Failed to create organization service", "error", err)
		os.Exit(1)
	}

	appService, err := services.NewApplicationService(
		keycloakClient.AdminClient, cfg.Keycloak.ClientId, cfg.Server.RefreshIntervalMin,
	)
	if err != nil {
		slog.Error("Failed to create application service", "error", err)
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
		prometheusRegistry,
		!slices.Contains([]string{"production", "prod"}, strings.ToLower(cfg.Server.Environment)),
	)

	slog.Info("Starting server", "address", httpServer.Addr, "version", version.Version, "build", version.BuildHash)
	slog.Info("Keycloak issuer configured", "url", cfg.Keycloak.Url+"/realms/"+cfg.Keycloak.Realm)
	slog.Info("OAuth redirect configured", "redirect_uri", cfg.Keycloak.RedirectUri)

	// Start server in a goroutine
	serverErr := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	// Wait for interrupt signal or server error
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		slog.Error("Server failed to start", "error", err)
		os.Exit(1)
	case sig := <-quit:
		slog.Info("Received shutdown signal", "signal", sig)
	}

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	slog.Info("Shutting down HTTP server...")
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	} else {
		slog.Info("HTTP server shutdown complete")
	}

	// Shutdown MeterProvider after server (flushes pending metrics)
	if meterProvider != nil {
		slog.Info("Shutting down MeterProvider...")
		if err := meterProvider.Shutdown(shutdownCtx); err != nil {
			slog.Error("MeterProvider shutdown error", "error", err)
		} else {
			slog.Info("MeterProvider shutdown complete")
		}
	}

	// Shutdown TracerProvider after server (flushes pending spans)
	if tracerProvider != nil {
		slog.Info("Shutting down TracerProvider...")
		if err := tracerProvider.Shutdown(shutdownCtx); err != nil {
			slog.Error("TracerProvider shutdown error", "error", err)
		} else {
			slog.Info("TracerProvider shutdown complete")
		}
	}

	slog.Info("Graceful shutdown complete")
}
