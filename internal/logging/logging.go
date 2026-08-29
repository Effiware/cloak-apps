// Package logging installs the JSON-to-stdout logger required by the instrumentation contract.
package logging

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel/trace"
)

// traceHandler stamps every record with the span's IDs. The key spellings are hard-wired in
// Grafana's Loki datasource regex `"trace_id":"(\w+)"` - any other spelling silently fails to link.
type traceHandler struct{ slog.Handler }

func (h traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.Handler.Handle(ctx, r)
}

// WithAttrs and WithGroup rewrap, otherwise a derived logger drops back to the bare handler.
func (h traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceHandler{h.Handler.WithAttrs(attrs)}
}

func (h traceHandler) WithGroup(name string) slog.Handler {
	return traceHandler{h.Handler.WithGroup(name)}
}

// Init makes JSON-on-stdout the slog default. One object per line, nothing else may write there.
func Init(level slog.Level) {
	slog.SetDefault(slog.New(traceHandler{
		slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}),
	}))
}
