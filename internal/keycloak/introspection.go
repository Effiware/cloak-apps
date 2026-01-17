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
	"time"

	"github.com/effiware/cloak-apps/utils"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// IntrospectionResponse represents the response from Keycloak's introspection endpoint
// See RFC 7662: https://datatracker.ietf.org/doc/html/rfc7662#section-2.2
type IntrospectionResponse struct {
	Active            bool   `json:"active"`
	Sub               string `json:"sub,omitempty"`
	Email             string `json:"email,omitempty"`
	Name              string `json:"name,omitempty"`
	PreferredUsername string `json:"preferred_username,omitempty"`
	GivenName         string `json:"given_name,omitempty"`
	FamilyName        string `json:"family_name,omitempty"`
	EmailVerified     bool   `json:"email_verified,omitempty"`

	// Additional fields that might be present
	Exp       int64  `json:"exp,omitempty"`
	Iat       int64  `json:"iat,omitempty"`
	ClientID  string `json:"client_id,omitempty"`
	TokenType string `json:"token_type,omitempty"`
	Scope     string `json:"scope,omitempty"`

	// Custom claims (realm_access, resource_access) stored as raw JSON
	RealmAccess    map[string]interface{} `json:"realm_access,omitempty"`
	ResourceAccess map[string]interface{} `json:"resource_access,omitempty"`
}

type IntrospectionResult struct {
	Active bool
	Claims map[string]interface{}
}

// EnableIntrospection activates token introspection with caching
func (c *Client) EnableIntrospection(introspectionCacheTTL time.Duration) {
	c.introspectionEnabled = true
	c.introspectionDone = make(chan struct{})
	c.cachedIntrospectToken = utils.CacheFirstForKeyTTL(
		c.introspectionDone,
		c.introspectToken,
		introspectionCacheTTL,
		true,
	)
}

// introspectToken validates a token using Keycloak's introspection endpoint
func (c *Client) introspectToken(ctx context.Context, token string) ([]byte, error) {
	introspectionURL := fmt.Sprintf("%s/protocol/openid-connect/token/introspect", c.IssuerURL)

	data := url.Values{}
	data.Set("token", token)
	data.Set("token_type_hint", "access_token")

	req, err := http.NewRequestWithContext(ctx, "POST", introspectionURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create introspection request: %w", err)
	}

	// Authenticate with client credentials
	req.SetBasicAuth(c.OAuth2Config.ClientID, c.OAuth2Config.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("introspection request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("introspection failed with status %d: %s", resp.StatusCode, string(body))
	}

	resBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return resBody, nil
}

// IntrospectToken returns the introspection result with claims if the token is valid
func (c *Client) IntrospectToken(ctx context.Context, token string) (*IntrospectionResult, error) {
	ctx, span := tracer.Start(ctx, "keycloak.IntrospectToken")
	defer span.End()

	slog.Debug("Introspecting token,", "token[:50]", token[:50])
	introspBody, err := c.cachedIntrospectToken(ctx, token)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "introspection failed")
		return nil, err
	}

	var introspResp IntrospectionResponse
	if err := json.Unmarshal(introspBody, &introspResp); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to decode introspection response")
		return nil, fmt.Errorf("failed to decode introspection response: %w", err)
	}

	span.SetAttributes(attribute.Bool("token.active", introspResp.Active))

	result := &IntrospectionResult{
		Active: introspResp.Active,
		Claims: make(map[string]interface{}),
	}

	// Only populate claims if token is active
	if introspResp.Active {
		result.Claims["sub"] = introspResp.Sub
		result.Claims["email"] = introspResp.Email
		result.Claims["name"] = introspResp.Name
		result.Claims["preferred_username"] = introspResp.PreferredUsername
		result.Claims["given_name"] = introspResp.GivenName
		result.Claims["family_name"] = introspResp.FamilyName
		result.Claims["email_verified"] = introspResp.EmailVerified
		result.Claims["realm_access"] = introspResp.RealmAccess
		result.Claims["resource_access"] = introspResp.ResourceAccess

		span.SetAttributes(attribute.String("user.sub", introspResp.Sub))
	}

	return result, nil
}
