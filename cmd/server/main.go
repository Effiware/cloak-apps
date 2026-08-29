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
	"github.com/effiware/cloak-apps/internal/logging"
	"github.com/effiware/cloak-apps/internal/server"
	"github.com/effiware/cloak-apps/internal/server/auth"
	"github.com/effiware/cloak-apps/internal/server/session"
	"github.com/effiware/cloak-apps/internal/services"
	"github.com/effiware/cloak-apps/internal/version"
	"github.com/effiware/cloak-apps/utils"
	"go.opentelemetry.io/otel/attribute"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otelprometheus "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const serviceName = "cloak-apps"

// getContainerID returns the HOSTNAME env var (container ID in K8s/Docker)
func getContainerID() string {
	if hostname := os.Getenv("HOSTNAME"); hostname != "" {
		return hostname
	}
	return serviceName + "-local"
}

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		slog.Error("Failed to load configuration file,", "error", err)
		os.Exit(1)
	}

	// JSON to stdout with trace_id/span_id, per the instrumentation contract
	var level slog.Level
	levelErr := level.UnmarshalText([]byte(cfg.Server.LogLevel))
	if levelErr != nil {
		level = slog.LevelInfo
	}
	logging.Init(level)
	if levelErr != nil {
		slog.Warn("Invalid log level, defaulted to INFO", "error", levelErr)
	}
	slog.Info("Initialized slog with", "level", level)

	// Create shared OTel resource for both tracing and metrics
	otelResource := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String(serviceName),
		semconv.ServiceVersionKey.String(version.Version),
		attribute.String("vcs.ref.head.revision", version.BuildHash), // not yet in semconv/v1.26.0
		semconv.DeploymentEnvironmentKey.String(cfg.Server.Environment),
		semconv.ContainerIDKey.String(getContainerID()),
		semconv.TelemetrySDKLanguageGo,
		semconv.TelemetrySDKNameKey.String("opentelemetry"),
		semconv.TelemetrySDKVersionKey.String("1.26.0"),
	)

	// Initialize TracerProvider (stored for graceful shutdown)
	var tracerProvider *sdktrace.TracerProvider
	if cfg.Otlp.Url != "" {
		otlpClientOpts := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(cfg.Otlp.Url),
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
			),
			sdktrace.WithResource(otelResource),
		)

		// Set it as the global trace provider
		otel.SetTracerProvider(tracerProvider)

		// W3C traceparent on in- and outbound calls; without it OTel defaults to a no-op propagator
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{}, propagation.Baggage{},
		))
		slog.Info("OTLP Trace Provider initialized,", "url", cfg.Otlp.Url, "secure", cfg.Otlp.Secure)
	}

	// Initialize MeterProvider with Prometheus exporter (stored for graceful shutdown)
	var meterProvider *sdkmetric.MeterProvider
	var prometheusRegistry *prometheus.Registry
	if cfg.Metrics.Enabled {
		prometheusRegistry = prometheus.NewRegistry()

		prometheusExporter, err := otelprometheus.New(
			otelprometheus.WithRegisterer(prometheusRegistry),
			otelprometheus.WithoutScopeInfo(),
			otelprometheus.WithNamespace("cloakapps"),
		)
		if err != nil {
			slog.Error("Failed to create Prometheus exporter,", "error", err)
			os.Exit(1)
		}

		meterProvider = sdkmetric.NewMeterProvider(
			sdkmetric.WithResource(otelResource),
			sdkmetric.WithReader(prometheusExporter),
		)

		// Set it as the global meter provider
		otel.SetMeterProvider(meterProvider)

		// Initialize metrics instruments now that MeterProvider is set
		keycloak.InitAdminMetrics()
		keycloak.InitIntrospectionMetrics()
		services.InitApplicationMetrics()
		utils.InitCacheMetrics()

		slog.Info("Prometheus MeterProvider initialized")
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
		prometheusRegistry,
		!slices.Contains([]string{"production", "prod"}, strings.ToLower(cfg.Server.Environment)),
	)

	slog.Info("Starting server,", "address", httpServer.Addr, "version", version.Version, "build", version.BuildHash)
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
