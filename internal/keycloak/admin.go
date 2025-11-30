package keycloak

import (
	"context"
	"net/http"
)

type AdminClient struct {
	BaseURL      string
	Realm        string
	ClientID     string
	ClientSecret string
	httpClient   *http.Client
	token        *TokenResponse
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type ClientRepresentation struct {
	ID                   string            `json:"id"`
	ClientID             string            `json:"clientId"`
	Name                 string            `json:"name"`        // → display_name
	Description          string            `json:"description"` // JSON string to parse
	BaseURL              string            `json:"baseUrl"`     // Home URL → app_url
	Enabled              bool              `json:"enabled"`
	Attributes           map[string]string `json:"attributes"` // Contains Logo URL
	DefaultClientScopes  []string          `json:"defaultClientScopes"`
	OptionalClientScopes []string          `json:"optionalClientScopes"`
}

type ClientScopeRepresentation struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Protocol    string `json:"protocol"`
}

// DescriptionMetadata represents parsed JSON from Description field
type DescriptionMetadata struct {
	Text       string   `json:"text"`
	Tooltip    string   `json:"tooltip,omitempty"`
	SSOEnabled bool     `json:"sso_enabled"`
	Tags       []string `json:"tags,omitempty"`
	Order      int      `json:"order,omitempty"`
}

func (ac *AdminClient) GetServiceAccountToken(ctx context.Context) error
func (ac *AdminClient) GetClients(ctx context.Context) ([]ClientRepresentation, error)
func (ac *AdminClient) GetClientScopes(ctx context.Context) ([]ClientScopeRepresentation, error)
func (ac *AdminClient) GetClientRoles(ctx context.Context, clientUUID string) ([]RoleRepresentation, error)
func ParseDescriptionJSON(description string) (*DescriptionMetadata, error)
