package keycloak

import (
	"context"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type Client struct {
	Provider     *oidc.Provider
	OAuth2Config *oauth2.Config
	AdminClient  *AdminClient
	Verifier     *oidc.IDTokenVerifier
}

func NewClient(ctx context.Context, issuerURL, clientID, clientSecret, redirectURI string) (*Client, error) {
	// Initialize OIDC provider
	// Configure OAuth2
	// Create Admin API client with service account
	// Return configured client
}
