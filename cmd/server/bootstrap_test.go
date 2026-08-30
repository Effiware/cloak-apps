package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/effiware/cloak-apps/internal/config"
	mw "github.com/effiware/cloak-apps/internal/server/middlewares"
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

func testConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Server.Name = "cloak-apps"
	cfg.Server.Environment = "test"
	cfg.Otlp.Protocol = "grpc"
	return cfg
}

// Guards the metric name, the RED labels and the go_*/process_* families a custom registry misses.
func TestBootMeterExposesRequestMetrics(t *testing.T) {
	cfg := testConfig()
	cfg.Metrics.Enabled = true
	meterProvider, registry := bootMeter(cfg, bootOtelResource(cfg))
	if meterProvider == nil || registry == nil {
		t.Fatal("bootMeter returned nil with metrics enabled")
	}
	t.Cleanup(func() { _ = meterProvider.Shutdown(t.Context()) })

	r := chi.NewRouter()
	r.Use(mw.RequestMetrics(func(req *http.Request) bool { return req.URL.Path == "/metrics" }))
	r.Get("/hda/applications", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/hda/applications", nil))

	rec := httptest.NewRecorder()
	promhttp.HandlerFor(registry, promhttp.HandlerOpts{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	for _, want := range []string{
		"cloak_apps_http_server_request_duration_seconds_bucket",
		`http_route="/hda/applications"`, `http_request_method="GET"`, `http_response_status_code="204"`,
		"go_goroutines", "process_open_fds",
	} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("/metrics is missing %q", want)
		}
	}
}

func TestBootMeterDisabled(t *testing.T) {
	if meterProvider, registry := bootMeter(testConfig(), nil); meterProvider != nil || registry != nil {
		t.Fatal("expected nils when metrics.enabled is false")
	}
}

func TestBootOtelDisabled(t *testing.T) {
	if tracerProvider := bootOtel(testConfig(), nil); tracerProvider != nil {
		t.Fatal("expected nil TracerProvider with no otlp.url and no OTEL_* endpoint env")
	}
}

// Regression guard: without SetTextMapPropagator OTel defaults to a no-op and no
// traceparent ever leaves the process — silently, with traces still looking healthy.
func TestBootOtelSetsPropagator(t *testing.T) {
	cfg := testConfig()
	// Not :4317 — a local collector would otherwise ingest spans from every test run.
	cfg.Otlp.Url = "127.0.0.1:14317"
	tracerProvider := bootOtel(cfg, bootOtelResource(cfg))
	if tracerProvider == nil {
		t.Fatal("expected a TracerProvider")
	}
	// Bounded: nothing listens on :4317 here and the batch processor retries on flush.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_ = tracerProvider.Shutdown(ctx)
	})

	ctx, span := tracerProvider.Tracer("test").Start(t.Context(), "test")
	defer span.End()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	if req.Header.Get("traceparent") == "" {
		t.Fatal("no traceparent injected — the propagator is not set")
	}
}
