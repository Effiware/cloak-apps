package hda

import (
	"log"
	"net/http"

	"github.com/effiware/cloak-apps/internal/server/middleware"
	"github.com/effiware/cloak-apps/internal/server/models"
	"github.com/effiware/cloak-apps/internal/services"
	views "github.com/effiware/cloak-apps/internal/views"
	components "github.com/effiware/cloak-apps/internal/views/components"
)

var clicks *models.Clicks = models.ClicksStore

func RenderRoot(appService *services.ApplicationService) ViewHandlerT {
	return func(w http.ResponseWriter, r *http.Request) error {
		// Get user info from context
		userInfo, ok := middleware.GetUserFromContext(r.Context())
		if !ok {
			log.Printf("Failed to get user from context")
			return &UnauthorizedError{Message: "User not authenticated"}
		}

		// Fetch applications for user
		applications, err := appService.GetApplicationsForUser(r.Context(), userInfo)
		if err != nil {
			log.Printf("Failed to get applications: %v", err)
			return err
		}

		log.Printf("Rendering for user: %s with %d applications", userInfo.PreferredUsername, len(applications))

		// Render template with real data
		template := views.Index(userInfo, applications)
		return template.Render(r.Context(), w)
	}
}

func RenderClick(w http.ResponseWriter, r *http.Request) error {
	clicks.Increment()
	template := components.Click()
	return template.Render(r.Context(), w)
}

// UnauthorizedError represents an authentication error
type UnauthorizedError struct {
	Message string
}

func (e *UnauthorizedError) Error() string {
	return e.Message
}
