package hda

import (
	"log/slog"
	"net/http"
	"sort"

	"github.com/effiware/cloak-apps/internal/server/middlewares"
	"github.com/effiware/cloak-apps/internal/server/models"
	"github.com/effiware/cloak-apps/internal/services"
	"github.com/effiware/cloak-apps/internal/views"
	"github.com/effiware/cloak-apps/internal/views/components"
)

// UnauthorizedError represents an authentication error
type UnauthorizedError struct {
	Message string
}

func (e *UnauthorizedError) Error() string {
	return e.Message
}

// RenderIndex renders the main page shell (navbar, header, HTMX container)
// Applications are loaded separately via HTMX
func RenderIndex(orgService *services.OrganizationService) ViewHandlerT {
	return func(w http.ResponseWriter, r *http.Request) error {
		userInfo, ok := middlewares.GetUserFromContext(r.Context())
		if !ok {
			slog.ErrorContext(r.Context(), "Failed to get user from context")
			return &UnauthorizedError{Message: "User not authenticated"}
		}

		organization, err := orgService.GetOrganization()
		if err != nil {
			slog.ErrorContext(r.Context(), "Failed to load organization", "error", err)
			return err
		}

		slog.DebugContext(r.Context(), "Rendering index", "user_sub", userInfo.Sub)

		template := views.Index(userInfo, organization)
		return template.Render(r.Context(), w)
	}
}

// RenderApplications renders the applications grid fragment (for HTMX requests)
// Query params: env (environment filter), view (card|list)
func RenderApplications(appService *services.ApplicationService) ViewHandlerT {
	return func(w http.ResponseWriter, r *http.Request) error {
		userInfo, ok := middlewares.GetUserFromContext(r.Context())
		if !ok {
			slog.ErrorContext(r.Context(), "Failed to get user from context")
			return &UnauthorizedError{Message: "User not authenticated"}
		}

		// Parse query parameters
		view := r.URL.Query().Get("view")
		if view == "" {
			view = "card"
		}

		// Fetch applications from Keycloak
		applications, err := appService.GetApplicationsForUser(r.Context(), userInfo)
		if err != nil {
			slog.ErrorContext(r.Context(), "Failed to get applications", "error", err)
			// Return error panel instead of error - this is HDA, we return HTML
			retryURL := "/hda/applications?view=" + view
			template := components.ErrorPanel("The authorization server may be temporarily unavailable.", retryURL)
			return template.Render(r.Context(), w)
		}

		// Extract available environments from apps (for dynamic tabs)
		environments := extractEnvironments(applications)

		// Get selected environment, default to first available
		env := r.URL.Query().Get("env")
		if env == "" && len(environments) > 0 {
			env = environments[0]
		}

		// Filter applications by environment (strict match)
		filteredApps := filterByEnvironment(applications, env)

		slog.DebugContext(r.Context(), "Rendering applications", "user_sub", userInfo.Sub,
			"env", env, "view", view, "count", len(filteredApps), "available_envs", environments)

		template := components.ApplicationsGrid(filteredApps, environments, env, view)
		return template.Render(r.Context(), w)
	}
}

// extractEnvironments returns unique environment values from apps, sorted
func extractEnvironments(apps []models.Application) []string {
	envSet := make(map[string]bool)
	for _, app := range apps {
		if app.Environment != "" {
			envSet[app.Environment] = true
		}
	}

	environments := make([]string, 0, len(envSet))
	for env := range envSet {
		environments = append(environments, env)
	}

	// Sort for consistent ordering
	sort.Strings(environments)
	return environments
}

// filterByEnvironment filters applications by exact environment match
func filterByEnvironment(apps []models.Application, env string) []models.Application {
	if env == "" {
		return apps
	}

	var filtered []models.Application
	for _, app := range apps {
		if app.Environment == env {
			filtered = append(filtered, app)
		}
	}
	return filtered
}
