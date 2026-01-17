package keycloak

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/effiware/cloak-apps/utils"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/singleflight"
)

var (
	tracer = otel.Tracer("cloak-apps")

	// Metrics instruments (initialized via InitAdminMetrics after MeterProvider is set)
	tokenRefreshCounter metric.Int64Counter
	apiCallDuration     metric.Float64Histogram
)

// InitAdminMetrics initializes metrics instruments. Must be called after MeterProvider is set.
func InitAdminMetrics() {
	meter := otel.Meter("cloak-apps")
	var err error

	tokenRefreshCounter, err = meter.Int64Counter(
		"keycloak.token.refresh.total",
		metric.WithDescription("Total number of service account token refresh operations"),
		metric.WithUnit("{operation}"),
	)
	if err != nil {
		slog.Error("Failed to create token refresh counter", "error", err)
	}

	apiCallDuration, err = meter.Float64Histogram(
		"keycloak.api.duration",
		metric.WithDescription("Duration of Keycloak Admin API calls"),
		metric.WithUnit("s"),
	)
	if err != nil {
		slog.Error("Failed to create API call duration histogram", "error", err)
	}
}

const (
	keycloakDataTTL               = 5 * time.Minute
	tokenExpiryBuffer             = 30 * time.Second
	tokenRefreshSingleflightGroup = "refresh_token"
)

type AdminClient struct {
	BaseURL          string
	Realm            string
	ClientID         string
	ClientSecret     string
	httpClient       *http.Client
	token            *TokenResponse
	tokenExpiry      time.Time
	tokenMutex       sync.RWMutex
	tokenGroup       singleflight.Group
	cachedGetClients utils.Circuit
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
	transport := &http.Transport{
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 5, // Conservative: Increase it when many users expected
		IdleConnTimeout:     60 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableKeepAlives:   false,
	}

	ac := &AdminClient{
		BaseURL:      baseURL,
		Realm:        realm,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: otelhttp.NewTransport(transport),
		},
	}

	ac.cachedGetClients = utils.CacheFirstTTL(ac.getClients, keycloakDataTTL)

	return ac
}

// setTokenAndExpiry is a helper function to set new token and tokenExpiry in a thread-safe manner
func (ac *AdminClient) setTokenAndExpiry(ctx context.Context, tr *TokenResponse, te time.Time) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("mutex.write_lock.acquiring")
	defer span.AddEvent("mutex.write_lock.released")

	ac.tokenMutex.Lock()
	defer ac.tokenMutex.Unlock()
	ac.token = tr
	ac.tokenExpiry = te
}

// getToken is a helper function to obtain current token in a thread-safe manner
func (ac *AdminClient) getToken(ctx context.Context) *TokenResponse {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("mutex.read_lock.acquiring")
	defer span.AddEvent("mutex.read_lock.released")

	ac.tokenMutex.RLock()
	defer ac.tokenMutex.RUnlock()

	return ac.token
}

// getTokenAndTokenExpiry is a helper function to obtain current token and tokenExpiry in a thread-safe manner
func (ac *AdminClient) getTokenAndTokenExpiry(ctx context.Context) (*TokenResponse, time.Time) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("mutex.read_lock.acquiring")
	defer span.AddEvent("mutex.read_lock.released")

	ac.tokenMutex.RLock()
	defer ac.tokenMutex.RUnlock()

	return ac.token, ac.tokenExpiry
}

func (ac *AdminClient) getServiceAccountToken(ctx context.Context) error {
	ctx, span := tracer.Start(ctx, "keycloak.GetServiceAccountToken")
	defer span.End()

	tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", ac.BaseURL, ac.Realm)

	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", ac.ClientID)
	data.Set("client_secret", ac.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create token request")
		tokenRefreshCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
		return fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := ac.httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get service account token")
		tokenRefreshCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
		return fmt.Errorf("failed to get service account token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
		span.RecordError(err)
		span.SetStatus(codes.Error, "token request failed")
		tokenRefreshCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
		return err
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to decode token response")
		tokenRefreshCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
		return fmt.Errorf("failed to decode token response: %w", err)
	}

	ac.setTokenAndExpiry(ctx, &tokenResp, time.Now().Add(time.Duration(tokenResp.ExpiresIn)*time.Second))
	slog.Debug("Service account token obtained", "expires_in", strconv.Itoa(tokenResp.ExpiresIn/60)+"min")
	tokenRefreshCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "success")))

	return nil
}

// ensureValidToken checks if the current token is valid and refreshes it if needed
func (ac *AdminClient) ensureValidToken(ctx context.Context) error {
	token, tokenExpiry := ac.getTokenAndTokenExpiry(ctx)

	// Check if token is nil or expired (with safety buffer)
	if token == nil || time.Now().Add(tokenExpiryBuffer).After(tokenExpiry) {
		if token != nil {
			slog.Debug("Token expired or expiring soon, proactively refreshing",
				"expires_at", tokenExpiry.Format(time.RFC3339),
				"time_until_expiry", time.Until(tokenExpiry))
		}

		// Use singleflight to ensure only one refresh happens
		_, err, _ := ac.tokenGroup.Do(tokenRefreshSingleflightGroup, func() (interface{}, error) {
			return nil, ac.getServiceAccountToken(ctx)
		})
		return err
	}

	return nil
}

func (ac *AdminClient) setFreshBearerToken(ctx context.Context, req *http.Request) error {
	if err := ac.ensureValidToken(ctx); err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", ac.getToken(ctx).AccessToken))

	return nil
}

func (ac *AdminClient) getClients(ctx context.Context) ([]byte, error) {
	clientsURL := fmt.Sprintf("%s/admin/realms/%s/clients", ac.BaseURL, ac.Realm)

	req, err := http.NewRequestWithContext(ctx, "GET", clientsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create clients request: %w", err)
	}

	resBody, err := utils.SendRetryableRequest(
		ctx, req, []int{http.StatusUnauthorized}, 1, ac.setFreshBearerToken, ac.httpClient,
	)
	if err != nil {
		return nil, err
	}

	return resBody, nil
}

func (ac *AdminClient) GetClients(ctx context.Context) ([]ClientRepresentation, error) {
	ctx, span := tracer.Start(ctx, "keycloak.GetClients")
	defer span.End()
	start := time.Now()

	resBody, err := ac.cachedGetClients(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get clients")
		apiCallDuration.Record(ctx, time.Since(start).Seconds(),
			metric.WithAttributes(attribute.String("operation", "get_clients"), attribute.String("status", "error")))
		return nil, err
	}

	var clients []ClientRepresentation
	if err := json.Unmarshal(resBody, &clients); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to decode clients response")
		apiCallDuration.Record(ctx, time.Since(start).Seconds(),
			metric.WithAttributes(attribute.String("operation", "get_clients"), attribute.String("status", "error")))
		return nil, fmt.Errorf("failed to decode clients response: %w", err)
	}

	apiCallDuration.Record(ctx, time.Since(start).Seconds(),
		metric.WithAttributes(attribute.String("operation", "get_clients"), attribute.String("status", "success")))
	return clients, nil
}

func (ac *AdminClient) GetClientScopes(ctx context.Context) ([]ClientScopeRepresentation, error) {
	ctx, span := tracer.Start(ctx, "keycloak.GetClientScopes")
	defer span.End()
	start := time.Now()

	scopesURL := fmt.Sprintf("%s/admin/realms/%s/client-scopes", ac.BaseURL, ac.Realm)

	req, err := http.NewRequestWithContext(ctx, "GET", scopesURL, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create client scopes request")
		apiCallDuration.Record(ctx, time.Since(start).Seconds(),
			metric.WithAttributes(attribute.String("operation", "get_client_scopes"), attribute.String("status", "error")))
		return nil, fmt.Errorf("failed to create client scopes request: %w", err)
	}

	resBody, err := utils.SendRetryableRequest(
		ctx, req, []int{http.StatusUnauthorized}, 1, ac.setFreshBearerToken, ac.httpClient,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get client scopes")
		apiCallDuration.Record(ctx, time.Since(start).Seconds(),
			metric.WithAttributes(attribute.String("operation", "get_client_scopes"), attribute.String("status", "error")))
		return nil, err
	}

	var scopes []ClientScopeRepresentation
	if err := json.Unmarshal(resBody, &scopes); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to decode client scopes response")
		apiCallDuration.Record(ctx, time.Since(start).Seconds(),
			metric.WithAttributes(attribute.String("operation", "get_client_scopes"), attribute.String("status", "error")))
		return nil, fmt.Errorf("failed to decode client scopes response: %w", err)
	}

	apiCallDuration.Record(ctx, time.Since(start).Seconds(),
		metric.WithAttributes(attribute.String("operation", "get_client_scopes"), attribute.String("status", "success")))
	return scopes, nil
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
