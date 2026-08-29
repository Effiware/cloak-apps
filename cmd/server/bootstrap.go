package main

import (
	"cmp"
	"context"
	"log/slog"
	"os"

	"github.com/effiware/cloak-apps/internal/config"
	"github.com/effiware/cloak-apps/internal/version"
	"go.opentelemetry.io/otel/trace"
)

var containerID string //nolint:gochecknoglobals

// getContainerID returns HOSTNAME (the container ID under K8s/Docker); memoized so
// the log `instance` field and the trace resource agree.
func getContainerID() string {
	if containerID == "" {
		if containerID = os.Getenv("HOSTNAME"); containerID == "" {
			containerID = version.ServiceName + "-local"
		}
	}
	return containerID
}

// traceHandler stamps trace_id/span_id from a ctx-carried span onto every record.
type traceHandler struct{ slog.Handler }

func (h traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
	}
	return h.Handler.Handle(ctx, r)
}
func (h traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceHandler{h.Handler.WithAttrs(attrs)}
}
func (h traceHandler) WithGroup(name string) slog.Handler {
	return traceHandler{h.Handler.WithGroup(name)}
}

// bootLogger installs the JSON logger at the configured level, with constant identity fields on every line.
func bootLogger(cfg *config.Config) {
	var level = new(slog.Level)
	if err := level.UnmarshalText([]byte(cfg.Server.LogLevel)); err == nil {
		slog.SetLogLoggerLevel(*level)
	} else {
		slog.Error("Error while unmarshalling log level", "error", err)
	}
	handler := traceHandler{slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})}
	slog.SetDefault(slog.New(handler).With(
		"service", serviceName(), "env", cfg.Server.Environment, "instance", getContainerID()))
	slog.Info("Initialized slog with", "level", level)
}

// serviceName resolves service identity for traces AND logs: the standard
// OTEL_SERVICE_NAME wins over the build-time name — the SDK resource builder never reads it.
// It must equal the pod's app.kubernetes.io/name label or Grafana's trace→logs link breaks.
func serviceName() string {
	return cmp.Or(os.Getenv("OTEL_SERVICE_NAME"), version.ServiceName)
}
