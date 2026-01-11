package hda

import (
	"log/slog"
	"net/http"

	"github.com/effiware/cloak-apps/internal/server/middleware"
	"github.com/effiware/cloak-apps/internal/services"
	views "github.com/effiware/cloak-apps/internal/views"
)

// UnauthorizedError represents an authentication error
type UnauthorizedError struct {
	Message string
}

func (e *UnauthorizedError) Error() string {
	return e.Message
}

func RenderRoot(orgService *services.OrganizationService, appService *services.ApplicationService) ViewHandlerT {
	return func(w http.ResponseWriter, r *http.Request) error {
		// Get user info from context
		userInfo, ok := middleware.GetUserFromContext(r.Context())
		if !ok {
			slog.Error("Failed to get user from context")
			return &UnauthorizedError{Message: "User not authenticated"}
		}

		organization, err := orgService.GetOrganization()
		if err != nil {
			slog.Error("Failed to load organization,", "error", err)
			return err
		}

		applications, err := appService.GetApplicationsForUser(r.Context(), userInfo)
		if err != nil {
			slog.Error("Failed to get applications,", "error", err)
			return err
		}

		slog.Debug("Rendering for", "user", userInfo.PreferredUsername, "applications", len(applications))

		// Render template with real data
		template := views.Index(userInfo, organization, applications)
		return template.Render(r.Context(), w)
	}
}
