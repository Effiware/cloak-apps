package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/effiware/cloak-apps/internal/config"
	"github.com/effiware/cloak-apps/internal/keycloak"
	"github.com/effiware/cloak-apps/internal/server"
	"github.com/effiware/cloak-apps/internal/server/auth"
	"github.com/effiware/cloak-apps/internal/server/session"
	"github.com/effiware/cloak-apps/internal/services"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.9.0"
)

const (
	serviceName    = "cloak-apps"
	serviceVersion = "0.0.1"
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

	// Initialize TracerProvider (stored for graceful shutdown)
	var tracerProvider *sdktrace.TracerProvider
	if cfg.Otlp.Url != "" {
		otlpHttpHeaders := map[string]string{
			"content-type": "application/json",
		}
		otlpClientOpts := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(cfg.Otlp.Url),
			otlptracehttp.WithHeaders(otlpHttpHeaders),
		}
		if !cfg.Otlp.Secure {
			otlpClientOpts = append(otlpClientOpts, otlptracehttp.WithInsecure())
		}

		otlpHttpExporter, err := otlptrace.New(context.Background(), otlptracehttp.NewClient(otlpClientOpts...))
		if err != nil {
			slog.Error("Failed to create OTLP trace exporter,", "error", err)
			os.Exit(1)
		}

		tracerProvider = sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(
				otlpHttpExporter,
				sdktrace.WithMaxExportBatchSize(sdktrace.DefaultMaxExportBatchSize),
				sdktrace.WithBatchTimeout(sdktrace.DefaultScheduleDelay*time.Millisecond),
				sdktrace.WithMaxExportBatchSize(sdktrace.DefaultMaxExportBatchSize),
			),
			sdktrace.WithResource(
				resource.NewWithAttributes(
					semconv.SchemaURL,
					semconv.ServiceNameKey.String(serviceName),
					semconv.ServiceVersionKey.String(serviceVersion),
				),
			),
		)

		// Set it as the global trace provider
		otel.SetTracerProvider(tracerProvider)
		slog.Info("OTLP Trace Provider initialized,", "url", cfg.Otlp.Url, "secure", cfg.Otlp.Secure)
	}

	keycloakClient, err := keycloak.NewClient(
		context.Background(),
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
