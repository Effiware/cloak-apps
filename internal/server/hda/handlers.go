package hda

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/effiware/cloak-apps/internal/server/http_errors"
)

type ViewHandlerT func(w http.ResponseWriter, request *http.Request) error

func WithJsonFallback(viewHandler ViewHandlerT) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := viewHandler(w, r)
		if err != nil {
			slog.ErrorContext(r.Context(), "View handler failed", "error", err)
			if clientErr, ok := err.(*http_errors.ClientErr); ok {
				JsonHandler(r.Context(), w, clientErr.HttpCode, clientErr)
			} else {
				JsonHandler(r.Context(), w, http.StatusInternalServerError,
					http_errors.InternalErr{
						HttpCode: http.StatusInternalServerError,
						Message:  "internal server error",
					},
				)
			}
		}
	}
}

func JsonHandler(ctx context.Context, w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	jsonPay, err := json.Marshal(payload)

	if err != nil {
		slog.ErrorContext(ctx, "Failed to marshal JSON response", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(code)
	w.Write(jsonPay)
}
