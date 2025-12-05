package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/effiware/cloak-apps/internal/keycloak"
	"github.com/effiware/cloak-apps/internal/server/middleware"
	"github.com/effiware/cloak-apps/internal/server/models"
)

type ApplicationService struct {
	adminClient  *keycloak.AdminClient
	clientScopes map[string]string // Maps scope ID to scope name
}

func NewApplicationService(adminClient *keycloak.AdminClient) (*ApplicationService, error) {
	service := &ApplicationService{
		adminClient:  adminClient,
		clientScopes: make(map[string]string),
	}

	// Fetch and cache client scopes on initialization
	if err := service.loadClientScopes(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to load client scopes: %w", err)
	}

	return service, nil
}

// loadClientScopes fetches all client scopes and builds ID→name mapping
func (as *ApplicationService) loadClientScopes(ctx context.Context) error {
	scopes, err := as.adminClient.GetClientScopes(ctx)
	if err != nil {
		return err
	}

	for _, scope := range scopes {
		as.clientScopes[scope.ID] = scope.Name
	}

	slog.Debug("Loaded client scopes,", "total_number", len(as.clientScopes))
	slog.Debug("Client scope", "mappings", as.clientScopes)
	return nil
}

// GetApplicationsForUser fetches all clients and filters based on user's roles
func (as *ApplicationService) GetApplicationsForUser(ctx context.Context, userInfo *middleware.UserInfo) ([]models.Application, error) {
	clients, err := as.adminClient.GetClients(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get clients: %w", err)
	}

	// Debug logging for application discovery
	slog.Debug("Fetching applications for", "user", userInfo.PreferredUsername, "roles", userInfo.ClientRoles)
	slog.Debug("Total clients length from Keycloak: ", "length", len(clients))

	var applications []models.Application

	for _, client := range clients {
		// Skip internal Keycloak clients (realm-management, account, etc.)
		if as.isInternalClient(client.ClientID) {
			continue
		}

		// Transform client to application
		app, err := as.transformClientToApplication(&client)
		if err != nil {
			slog.Debug("Failed to transform,", "client", client.ClientID, "error", err)
			continue
		}

		// Check if user has access to this client
		if roles, hasRoles := userInfo.ClientRoles[client.ClientID]; hasRoles && len(roles) > 0 {
			app.HasAccess = true
			applications = append(applications, *app)
			slog.Debug("Access GRANTED for", "client", client.ClientID, "roles", roles)
		} else {
			slog.Debug("Access DENIED for", "client", client.ClientID, "roles", roles)
		}
	}

	slog.Debug("Found accessible applications for", "user", userInfo.PreferredUsername, "applications", len(applications))
	return applications, nil
}

// transformClientToApplication converts Keycloak client to Application model
func (as *ApplicationService) transformClientToApplication(client *keycloak.ClientRepresentation) (*models.Application, error) {
	metadata, err := keycloak.ParseDescriptionJSON(client.Description)
	if err != nil {
		slog.Warn("Failed to parse description for", "client", client.ClientID, "error", err)
		// Use fallback - default to SSO disabled for safety
		metadata = &keycloak.DescriptionMetadata{
			Text:       client.Description,
			SSOEnabled: false,
		}
	}

	// Extract space from optional scopes
	space := as.extractScopeCategory(client.OptionalClientScopes, "space-")
	environment := as.extractScopeCategory(client.OptionalClientScopes, "env-")

	// Debug logging for metadata extraction
	slog.Debug("", "Client", client.ClientID, "OptionalClientScopes", client.OptionalClientScopes)
	slog.Debug("", "Client", client.ClientID, "Space", space, "Environment", environment)
	slog.Debug("", "Client", client.ClientID, "Attributes", client.Attributes)

	// Extract thumbnail URL from attributes
	thumbnailURL := ""
	if client.Attributes != nil {
		// Keycloak uses 'logoUri' not 'logoUrl'
		thumbnailURL = client.Attributes["logoUri"]
	}
	slog.Debug("", "Client", client.ClientID, "ThumbnailURL", thumbnailURL)
	slog.Debug("", "Client", client.ClientID, "SSOEnabled", metadata.SSOEnabled, "URL", client.BaseURL)

	return &models.Application{
		ID:           client.ID,
		ClientID:     client.ClientID,
		Name:         client.Name,
		Description:  metadata.Text,
		Tooltip:      metadata.Tooltip,
		IconEmoji:    metadata.IconEmoji,
		ThumbnailURL: thumbnailURL,
		URL:          client.BaseURL,
		Space:        space,
		Environment:  environment,
		Tags:         metadata.Tags,
		Order:        metadata.Order,
		SSOEnabled:   metadata.SSOEnabled,
		HasAccess:    false, // Will be set by caller
	}, nil
}

// extractScopeCategory extracts value from scope name with given prefix
// Example: extractScopeCategory(["space-operations", "env-shared"], "space-")
// returns "operations"
func (as *ApplicationService) extractScopeCategory(scopeNames []string, prefix string) string {
	// OptionalClientScopes contains scope names directly, not IDs
	for _, scopeName := range scopeNames {
		if strings.HasPrefix(scopeName, prefix) {
			// Remove prefix to get category value
			// "space-operations" → "operations"
			return strings.TrimPrefix(scopeName, prefix)
		}
	}
	return ""
}

// isInternalClient checks if a client is a Keycloak internal client
func (as *ApplicationService) isInternalClient(clientID string) bool {
	internalClients := []string{
		"account",
		"account-console",
		"admin-cli",
		"broker",
		"realm-management",
		"security-admin-console",
		"cloak-apps-portal", // Our portal client should not appear in the list
	}

	for _, internal := range internalClients {
		if clientID == internal {
			return true
		}
	}

	return false
}

// RefreshClientScopes reloads client scopes mapping
// Useful if scopes are added/modified at runtime
func (as *ApplicationService) RefreshClientScopes(ctx context.Context) error {
	return as.loadClientScopes(ctx)
}
