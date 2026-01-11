package keycloak

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type Client struct {
	IssuerURL    string
	Provider     *oidc.Provider
	OAuth2Config *oauth2.Config
	AdminClient  *AdminClient
	Verifier     *oidc.IDTokenVerifier
}

func NewClient(ctx context.Context, keycloakURL, realm, clientID, clientSecret, redirectURI string) (*Client, error) {
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
		IssuerURL:    issuerURL,
		Provider:     provider,
		OAuth2Config: oauth2Config,
		AdminClient:  adminClient,
		Verifier:     verifier,
	}, nil
}
