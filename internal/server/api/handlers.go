package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	_ "github.com/effiware/cloak-apps/internal/docs"
	"github.com/effiware/cloak-apps/internal/server/middlewares"
	"github.com/effiware/cloak-apps/internal/server/models"
)

type EndpointHandlerT func(w http.ResponseWriter, request *http.Request) (int, any, error)

func JsonHandler(endpointHandler EndpointHandlerT) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		code, payload, err := endpointHandler(w, r)
		if err != nil {
			slog.Error("EndpointHandler,", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		jsonPay, err := json.Marshal(payload)
		if err != nil {
			slog.Error("When marshaling JSON,", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(code)
		w.Write(jsonPay)
	}
}

// GetOrganization returns a handler that loads organization info
//
//	@Summary	Get organization
//	@Detail		Get organization details
//	@Tags		organization
//	@Accept		json
//	@Produce	json
//	@Router		/organization [get]
func GetOrganization(orgService interface {
	GetOrganization() (models.Organization, error)
}) EndpointHandlerT {
	return func(w http.ResponseWriter, r *http.Request) (int, any, error) {
		organization, err := orgService.GetOrganization()
		if err != nil {
			slog.Error("Failed to load organization,", "error", err)
			return http.StatusInternalServerError, map[string]string{"error": "failed to load organization info"}, nil
		}
		return http.StatusOK, organization, nil
	}
}

// GetApplications returns a handler that fetches applications for the authenticated user
//
//	@Summary		List assigned applications
//	@Description	List assigned applications for the authenticated user
//	@Tags			applications
//	@Accept			json
//	@Produce		json
//	@Router			/applications [get]
func GetApplications(appService interface {
	GetApplicationsForUser(ctx context.Context, userInfo *middlewares.UserInfo) ([]models.Application, error)
}) EndpointHandlerT {
	return func(w http.ResponseWriter, r *http.Request) (int, any, error) {
		// Get user info from context
		userInfo, ok := middlewares.GetUserFromContext(r.Context())
		if !ok {
			return http.StatusUnauthorized, map[string]string{"error": "unauthorized"}, nil
		}

		applications, err := appService.GetApplicationsForUser(r.Context(), userInfo)
		if err != nil {
			slog.Error("Failed to get applications,", "error", err)
			return http.StatusInternalServerError, map[string]string{"error": "failed to fetch applications"}, nil
		}

		return http.StatusOK, applications, nil
	}
}
