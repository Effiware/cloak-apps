package keycloak

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/effiware/cloak-apps/utils"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2"
)

type Client struct {
	IssuerURL             string
	Provider              *oidc.Provider
	OAuth2Config          *oauth2.Config
	AdminClient           *AdminClient
	Verifier              *oidc.IDTokenVerifier
	httpClient            *http.Client
	introspectionEnabled  bool
	introspectionDone     chan struct{}
	cachedIntrospectToken utils.CircuitWithKey
}

func NewClient(ctx context.Context, keycloakURL, realm, clientID, clientSecret, redirectURI string) (*Client, error) {
	ctx, span := otel.GetTracerProvider().Tracer("cloak-apps").Start(
		ctx,
		"NewClient",
		trace.WithAttributes(attribute.String("clientID", clientID)),
	)
	defer span.End()

	issuerURL := fmt.Sprintf("%s/realms/%s", keycloakURL, realm)

	// Initialize OIDC provider
	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create OIDC provider: %w", err)
	}

	// Configure OAuth2
	oauth2Config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURI,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

	// Create ID token verifier
	verifier := provider.Verifier(&oidc.Config{ClientID: clientID})

	// Create Admin API client with service account
	adminClient := NewAdminClient(keycloakURL, realm, clientID, clientSecret)

	return &Client{
		IssuerURL:            issuerURL,
		Provider:             provider,
		OAuth2Config:         oauth2Config,
		AdminClient:          adminClient,
		Verifier:             verifier,
		httpClient:           &http.Client{Timeout: 5 * time.Second},
		introspectionEnabled: false,
	}, nil
}

// IsIntrospectionEnabled returns whether introspection is enabled
func (c *Client) IsIntrospectionEnabled() bool {
	return c.introspectionEnabled
}

func (c *Client) Shutdown() {
	if c.introspectionEnabled {
		close(c.introspectionDone)
	}
}
