package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	_ "github.com/effiware/cloak-apps/internal/docs"
	"github.com/effiware/cloak-apps/internal/server/middleware"
	"github.com/effiware/cloak-apps/internal/server/models"
)

type EndpointHandlerT func(w http.ResponseWriter, request *http.Request) (int, any, error)

func JsonHandler(endpointHandler EndpointHandlerT) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		code, payload, err := endpointHandler(w, r)
		if err != nil {
			log.Printf("Error: %s", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		jsonPay, err := json.Marshal(payload)
		if err != nil {
			log.Printf("Error when marshaling JSON: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(code)
		w.Write(jsonPay)
	}
}

// @Summary		Get the number of clicks
// @Description	Retrieve the total number of clicks recorded in the system
// @Tags		clicks
// @Accept		json
// @Produce		json
// @Router		/clicks [get]
func GetClicks(w http.ResponseWriter, r *http.Request) (int, any, error) {
	ctx := r.Context()
	_ = ctx // currently unused, but may be useful for logging or tracing in the future
	clicks := models.ClicksStore
	return http.StatusOK, models.Clicks{Count: clicks.GetCount()}, nil
}

// @Summary		Increment the number of clicks
// @Description	Increment the total number of clicks recorded in the system by one
// @Tags		clicks
// @Accept		json
// @Produce		json
// @Router		/clicks/increment [post]
func IncrementClicks(w http.ResponseWriter, r *http.Request) (int, any, error) {
	ctx := r.Context()
	_ = ctx // currently unused, but may be useful for logging or tracing in the future
	clicks := models.ClicksStore
	clicks.Increment()
	return http.StatusNoContent, map[string]string{}, nil
}

// GetApplications returns a handler that fetches applications for the authenticated user
func GetApplications(appService interface {
	GetApplicationsForUser(ctx context.Context, userInfo *middleware.UserInfo) ([]models.Application, error)
}) EndpointHandlerT {
	return func(w http.ResponseWriter, r *http.Request) (int, any, error) {
		// Get user info from context
		userInfo, ok := middleware.GetUserFromContext(r.Context())
		if !ok {
			return http.StatusUnauthorized, map[string]string{"error": "unauthorized"}, nil
		}

		// Fetch applications
		applications, err := appService.GetApplicationsForUser(r.Context(), userInfo)
		if err != nil {
			log.Printf("Failed to get applications: %v", err)
			return http.StatusInternalServerError, map[string]string{"error": "failed to fetch applications"}, nil
		}

		return http.StatusOK, applications, nil
	}
}
