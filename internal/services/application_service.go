package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/effiware/cloak-apps/internal/keycloak"
	"github.com/effiware/cloak-apps/internal/server/middlewares"
	"github.com/effiware/cloak-apps/internal/server/models"
	"github.com/effiware/cloak-apps/internal/version"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
)

var (
	tracer = otel.Tracer(version.ServiceName) //nolint:gochecknoglobals

	// Metrics instruments (initialized via InitApplicationMetrics after MeterProvider is set)
	applicationsFetchDuration metric.Float64Histogram
	applicationsFetchCounter  metric.Int64Counter
)

// InitApplicationMetrics initializes metrics instruments. Must be called after MeterProvider is set.
func InitApplicationMetrics() {
	meter := otel.Meter(version.ServiceName)
	var err error

	applicationsFetchDuration, err = meter.Float64Histogram(
		"applications.fetch.duration",
		metric.WithDescription("Duration of fetching applications for a user"),
		metric.WithUnit("s"),
	)
	if err != nil {
		slog.Error("Failed to create applications fetch duration histogram", "error", err)
	}

	applicationsFetchCounter, err = meter.Int64Counter(
		"applications.fetch.total",
		metric.WithDescription("Total number of application fetch operations"),
		metric.WithUnit("{operation}"),
	)
	if err != nil {
		slog.Error("Failed to create applications fetch counter", "error", err)
	}
}

type ApplicationService struct {
	done              chan struct{}
	adminClient       *keycloak.AdminClient
	clientScopes      map[string]string // Maps scope ID to scope name
	cloakAppsClientId string
	refreshTicker     *time.Ticker
}

func NewApplicationService(adminClient *keycloak.AdminClient, cloakAppsClientId string, refreshIntervalMin int) (*ApplicationService, error) {
	service := &ApplicationService{
		done:              make(chan struct{}),
		adminClient:       adminClient,
		clientScopes:      make(map[string]string),
		cloakAppsClientId: cloakAppsClientId,
		refreshTicker:     time.NewTicker(time.Duration(refreshIntervalMin) * time.Minute),
	}

	if err := service.loadClientScopes(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to load client scopes: %w", err)
	}
	service.scheduleClientScopesRefresh()

	return service, nil
}

// loadClientScopes fetches all client scopes and builds ID→name mapping
func (as *ApplicationService) loadClientScopes(ctx context.Context) error {
	ctx, span := tracer.Start(ctx, "ApplicationService.LoadClientScopes")
	defer span.End()

	scopes, err := as.adminClient.GetClientScopes(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get client scopes")
		return err
	}

	for _, scope := range scopes {
		as.clientScopes[scope.ID] = scope.Name
	}

	span.SetAttributes(attribute.Int("scopes.count", len(as.clientScopes)))
	slog.Debug("Loaded client scopes,", "total_number", len(as.clientScopes))
	return nil
}

// scheduleClientScopesRefresh refreshes client scopes based on the set interval
func (as *ApplicationService) scheduleClientScopesRefresh() {
	go func() {
		for {
			select {
			case <-as.done:
				as.refreshTicker.Stop()
				return
			case <-as.refreshTicker.C:
				slog.Debug("Automatic client scopes refresh triggered")
				if err := as.loadClientScopes(context.Background()); err != nil {
					slog.Error("Error while refreshing client scopes,", "error", err)
				}
			}
		}
	}()
}

// GetApplicationsForUser fetches all clients and filters based on user's roles
func (as *ApplicationService) GetApplicationsForUser(ctx context.Context, userInfo *middlewares.UserInfo) ([]models.Application, error) {
	ctx, span := tracer.Start(ctx, "ApplicationService.GetApplicationsForUser")
	defer span.End()
	start := time.Now()

	span.SetAttributes(attribute.String("user.sub", userInfo.Sub))

	clients, err := as.adminClient.GetClients(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get clients")
		applicationsFetchCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
		applicationsFetchDuration.Record(ctx, time.Since(start).Seconds(),
			metric.WithAttributes(attribute.String("status", "error")))
		return nil, fmt.Errorf("failed to get clients: %w", err)
	}

	slog.Debug("Fetching applications for", "user", userInfo.PreferredUsername, "roles", userInfo.ClientRoles)

	var applications []models.Application
	for _, client := range clients {
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

	span.SetAttributes(attribute.Int("applications.count", len(applications)))
	applicationsFetchCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "success")))
	applicationsFetchDuration.Record(ctx, time.Since(start).Seconds(),
		metric.WithAttributes(attribute.String("status", "success")))
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
	slog.Debug("-->", "Client", client.ClientID, "OptionalClientScopes", client.OptionalClientScopes)
	slog.Debug("-->", "Client", client.ClientID, "Space", space, "Environment", environment)

	// Extract thumbnail URL from attributes
	thumbnailURL := ""
	if client.Attributes != nil {
		// Keycloak uses 'logoUri' not 'logoUrl'
		thumbnailURL = client.Attributes["logoUri"]
	}
	slog.Debug("-->", "Client", client.ClientID, "ThumbnailURL", thumbnailURL)
	slog.Debug("-->", "Client", client.ClientID, "SSOEnabled", metadata.SSOEnabled, "URL", client.BaseURL)

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
		"account-console",
		"admin-cli",
		"broker",
		"realm-management",
		"security-admin-console",
		as.cloakAppsClientId, // Our portal client should not appear in the list
	}

	for _, internal := range internalClients {
		if clientID == internal {
			return true
		}
	}

	return false
}

func (as *ApplicationService) ShutDown() {
	close(as.done)
}
