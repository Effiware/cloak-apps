package keycloak

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const tokenExpiryBuffer = 30 * time.Second

type AdminClient struct {
	BaseURL      string
	Realm        string
	ClientID     string
	ClientSecret string
	httpClient   *http.Client
	token        *TokenResponse
	tokenExpiry  time.Time
	tokenMutex   sync.Mutex
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
	IconEmoji  string   `json:"icon_emoji,omitempty"`
	SSOEnabled bool     `json:"sso_enabled"`
	Tags       []string `json:"tags,omitempty"`
	Order      int      `json:"order,omitempty"`
}

type RoleRepresentation struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func NewAdminClient(baseURL, realm, clientID, clientSecret string) *AdminClient {
	return &AdminClient{
		BaseURL:      baseURL,
		Realm:        realm,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (ac *AdminClient) GetServiceAccountToken(ctx context.Context) error {
	tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", ac.BaseURL, ac.Realm)

	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", ac.ClientID)
	data.Set("client_secret", ac.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := ac.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to get service account token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return fmt.Errorf("failed to decode token response: %w", err)
	}

	ac.token = &tokenResp
	// Store token expiry time for proactive refresh
	ac.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	slog.Debug("Service account token obtained",
		"expires_in", tokenResp.ExpiresIn,
		"expires_at", ac.tokenExpiry.Format(time.RFC3339))

	return nil
}

// ensureValidToken (thread-safe) checks if the current token is valid and refreshes it if needed
func (ac *AdminClient) ensureValidToken(ctx context.Context) error {
	ac.tokenMutex.Lock()
	defer ac.tokenMutex.Unlock()

	// Check if token is nil or expired (with safety buffer)
	if ac.token == nil || time.Now().Add(tokenExpiryBuffer).After(ac.tokenExpiry) {
		if ac.token != nil {
			slog.Debug("Token expired or expiring soon, proactively refreshing",
				"expires_at", ac.tokenExpiry.Format(time.RFC3339),
				"time_until_expiry", time.Until(ac.tokenExpiry))
		}
		return ac.GetServiceAccountToken(ctx)
	}

	return nil
}

func (ac *AdminClient) GetClients(ctx context.Context) ([]ClientRepresentation, error) {
	if err := ac.ensureValidToken(ctx); err != nil {
		return nil, err
	}

	clientsURL := fmt.Sprintf("%s/admin/realms/%s/clients", ac.BaseURL, ac.Realm)

	req, err := http.NewRequestWithContext(ctx, "GET", clientsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create clients request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", ac.token.AccessToken))
	req.Header.Set("Content-Type", "application/json")

	resp, err := ac.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get clients: %w", err)
	}
	defer resp.Body.Close()

	// Fallback: If we still get 401 (e.g., clock skew, manual revocation), retry once
	if resp.StatusCode == http.StatusUnauthorized {
		slog.Warn("Token unexpectedly invalid despite proactive refresh, forcing refresh")

		ac.tokenMutex.Lock()
		if err := ac.GetServiceAccountToken(ctx); err != nil {
			ac.tokenMutex.Unlock()
			return nil, fmt.Errorf("failed to refresh token: %w", err)
		}
		ac.tokenMutex.Unlock()

		// Retry the request with new token
		req, err = http.NewRequestWithContext(ctx, "GET", clientsURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create retry request: %w", err)
		}
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", ac.token.AccessToken))
		req.Header.Set("Content-Type", "application/json")

		resp, err = ac.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to retry clients request: %w", err)
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("clients request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var clients []ClientRepresentation
	if err := json.NewDecoder(resp.Body).Decode(&clients); err != nil {
		return nil, fmt.Errorf("failed to decode clients response: %w", err)
	}

	return clients, nil
}

func (ac *AdminClient) GetClientScopes(ctx context.Context) ([]ClientScopeRepresentation, error) {
	if err := ac.ensureValidToken(ctx); err != nil {
		return nil, err
	}

	scopesURL := fmt.Sprintf("%s/admin/realms/%s/client-scopes", ac.BaseURL, ac.Realm)

	req, err := http.NewRequestWithContext(ctx, "GET", scopesURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create client scopes request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", ac.token.AccessToken))
	req.Header.Set("Content-Type", "application/json")

	resp, err := ac.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get client scopes: %w", err)
	}
	defer resp.Body.Close()

	// Fallback: If we still get 401 (e.g., clock skew, manual revocation), retry once
	if resp.StatusCode == http.StatusUnauthorized {
		slog.Warn("Token unexpectedly invalid despite proactive refresh, forcing refresh")

		ac.tokenMutex.Lock()
		if err := ac.GetServiceAccountToken(ctx); err != nil {
			ac.tokenMutex.Unlock()
			return nil, fmt.Errorf("failed to refresh token: %w", err)
		}
		ac.tokenMutex.Unlock()

		// Retry the request with new token
		req, err = http.NewRequestWithContext(ctx, "GET", scopesURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create retry request: %w", err)
		}
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", ac.token.AccessToken))
		req.Header.Set("Content-Type", "application/json")

		resp, err = ac.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to retry client scopes request: %w", err)
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("client scopes request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var scopes []ClientScopeRepresentation
	if err := json.NewDecoder(resp.Body).Decode(&scopes); err != nil {
		return nil, fmt.Errorf("failed to decode client scopes response: %w", err)
	}

	return scopes, nil
}

func (ac *AdminClient) GetClientRoles(ctx context.Context, clientUUID string) ([]RoleRepresentation, error) {
	if err := ac.ensureValidToken(ctx); err != nil {
		return nil, err
	}

	rolesURL := fmt.Sprintf("%s/admin/realms/%s/clients/%s/roles", ac.BaseURL, ac.Realm, clientUUID)

	req, err := http.NewRequestWithContext(ctx, "GET", rolesURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create client roles request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", ac.token.AccessToken))
	req.Header.Set("Content-Type", "application/json")

	resp, err := ac.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get client roles: %w", err)
	}
	defer resp.Body.Close()

	// Fallback: If we still get 401 (e.g., clock skew, manual revocation), retry once
	if resp.StatusCode == http.StatusUnauthorized {
		slog.Warn("Token unexpectedly invalid despite proactive refresh, forcing refresh")

		ac.tokenMutex.Lock()
		if err := ac.GetServiceAccountToken(ctx); err != nil {
			ac.tokenMutex.Unlock()
			return nil, fmt.Errorf("failed to refresh token: %w", err)
		}
		ac.tokenMutex.Unlock()

		// Retry the request with new token
		req, err = http.NewRequestWithContext(ctx, "GET", rolesURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create retry request: %w", err)
		}
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", ac.token.AccessToken))
		req.Header.Set("Content-Type", "application/json")

		resp, err = ac.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to retry client roles request: %w", err)
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("client roles request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var roles []RoleRepresentation
	if err := json.NewDecoder(resp.Body).Decode(&roles); err != nil {
		return nil, fmt.Errorf("failed to decode client roles response: %w", err)
	}

	return roles, nil
}

func ParseDescriptionJSON(description string) (*DescriptionMetadata, error) {
	if description == "" {
		return &DescriptionMetadata{
			Text:       "",
			SSOEnabled: false, // Default to no SSO
		}, nil
	}

	var metadata DescriptionMetadata
	if err := json.Unmarshal([]byte(description), &metadata); err != nil {
		// If JSON parsing fails, treat as plain text (with SSO disabled)
		return &DescriptionMetadata{
			Text:       description,
			SSOEnabled: false,
		}, nil
	}

	// Set default for SSO if not specified
	if metadata.Text == "" && description != "" {
		metadata.Text = description
	}

	return &metadata, nil
}
