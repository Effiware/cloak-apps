package services

import (
	"context"
	"fmt"
	"log"
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
func (s *ApplicationService) loadClientScopes(ctx context.Context) error {
	scopes, err := s.adminClient.GetClientScopes(ctx)
	if err != nil {
		return err
	}

	for _, scope := range scopes {
		s.clientScopes[scope.ID] = scope.Name
	}

	log.Printf("Loaded %d client scopes", len(s.clientScopes))
	log.Printf("[DEBUG] Client scope mappings: %+v", s.clientScopes)
	return nil
}

// GetApplicationsForUser fetches all clients and filters based on user's roles
func (s *ApplicationService) GetApplicationsForUser(ctx context.Context, userInfo *middleware.UserInfo) ([]models.Application, error) {
	// Fetch all clients from Keycloak
	clients, err := s.adminClient.GetClients(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get clients: %w", err)
	}

	// Debug logging for application discovery
	log.Printf("[DEBUG] Fetching applications for user %s with client roles: %+v",
		userInfo.PreferredUsername, userInfo.ClientRoles)
	log.Printf("[DEBUG] Total clients from Keycloak: %d", len(clients))

	var applications []models.Application

	for _, client := range clients {
		// Skip internal Keycloak clients (realm-management, account, etc.)
		if s.isInternalClient(client.ClientID) {
			continue
		}

		// Transform client to application
		app, err := s.transformClientToApplication(&client)
		if err != nil {
			log.Printf("Failed to transform client %s: %v", client.ClientID, err)
			continue
		}

		// Check if user has access to this client
		if roles, hasRoles := userInfo.ClientRoles[client.ClientID]; hasRoles && len(roles) > 0 {
			app.HasAccess = true
			applications = append(applications, *app)
			log.Printf("[DEBUG] Access GRANTED for client '%s' - User has roles: %v", client.ClientID, roles)
		} else {
			log.Printf("[DEBUG] Access DENIED for client '%s' - User has no roles", client.ClientID)
		}
	}

	log.Printf("Found %d accessible applications for user %s", len(applications), userInfo.PreferredUsername)
	return applications, nil
}

// transformClientToApplication converts Keycloak client to Application model
func (s *ApplicationService) transformClientToApplication(client *keycloak.ClientRepresentation) (*models.Application, error) {
	// Parse Description JSON
	metadata, err := keycloak.ParseDescriptionJSON(client.Description)
	if err != nil {
		log.Printf("Warning: Failed to parse description for client %s: %v", client.ClientID, err)
		// Use fallback - default to SSO disabled for safety
		metadata = &keycloak.DescriptionMetadata{
			Text:       client.Description,
			SSOEnabled: false,
		}
	}

	// Extract space from optional scopes
	space := s.extractScopeCategory(client.OptionalClientScopes, "space-")
	environment := s.extractScopeCategory(client.OptionalClientScopes, "env-")

	// Debug logging for metadata extraction
	log.Printf("[DEBUG] Client '%s' - OptionalClientScopes: %v", client.ClientID, client.OptionalClientScopes)
	log.Printf("[DEBUG] Client '%s' - Extracted space: '%s', environment: '%s'", client.ClientID, space, environment)
	log.Printf("[DEBUG] Client '%s' - Attributes: %+v", client.ClientID, client.Attributes)

	// Extract thumbnail URL from attributes
	thumbnailURL := ""
	if client.Attributes != nil {
		// Keycloak uses 'logoUri' not 'logoUrl'
		thumbnailURL = client.Attributes["logoUri"]
	}
	log.Printf("[DEBUG] Client '%s' - ThumbnailURL: '%s'", client.ClientID, thumbnailURL)
	log.Printf("[DEBUG] Client '%s' - SSOEnabled: %v, URL: '%s'", client.ClientID, metadata.SSOEnabled, client.BaseURL)

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
func (s *ApplicationService) extractScopeCategory(scopeNames []string, prefix string) string {
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
func (s *ApplicationService) isInternalClient(clientID string) bool {
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
func (s *ApplicationService) RefreshClientScopes(ctx context.Context) error {
	return s.loadClientScopes(ctx)
}
