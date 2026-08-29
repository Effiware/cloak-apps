package middlewares

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// RequestLogger emits one JSON line per request. Must run inside otelchi so the context
// carries the span and the line gets a trace_id.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()

		defer func() {
			// Route template, not the resolved path - keeps cardinality low
			route := "unmatched"
			if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
				route = rc.RoutePattern()
			}
			slog.LogAttrs(r.Context(), slog.LevelInfo, "http request",
				slog.String("method", r.Method),
				slog.String("route", route),
				slog.Int("status", ww.Status()),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
				slog.Int("bytes", ww.BytesWritten()),
			)
		}()

		next.ServeHTTP(ww, r)
	})
}

// Recoverer keeps a panic on one line: an unrecovered one bypasses slog and prints a raw
// multi-line stack that Loki stores as dozens of unparseable, unlinked lines.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler {
				panic(rec) // deliberate abort, not a bug
			}
			slog.LogAttrs(r.Context(), slog.LevelError, "panic recovered",
				slog.String("panic", fmt.Sprint(rec)),
				slog.String("stack", string(debug.Stack())),
			)
			w.WriteHeader(http.StatusInternalServerError)
		}()

		next.ServeHTTP(w, r)
	})
}
