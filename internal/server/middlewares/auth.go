package middlewares

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/effiware/cloak-apps/internal/keycloak"
	"github.com/effiware/cloak-apps/internal/server/session"
	"github.com/effiware/cloak-apps/internal/version"
	"github.com/mitchellh/mapstructure"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer(version.ServiceName)

type contextKey string

const UserContextKey contextKey = "user"

// isHTMXRequest checks if the request is from HTMX
func isHTMXRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// redirectToLogin handles redirect for both regular and HTMX requests
// For HTMX requests, uses HX-Redirect header to trigger full page navigation
// For regular requests, uses standard HTTP 302 redirect
func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	if isHTMXRequest(r) {
		// HTMX request: use HX-Redirect header for full page navigation
		w.Header().Set("HX-Redirect", "/auth/login")
		w.WriteHeader(http.StatusOK)
		return
	}
	// Regular request: standard redirect
	http.Redirect(w, r, "/auth/login", http.StatusFound)
}

type UserInfo struct {
	Sub               string
	Email             string
	Name              string
	PreferredUsername string
	GivenName         string
	FamilyName        string
	Roles             []string
	ClientRoles       map[string][]string // clientID -> roles
	EmailVerified     bool
}

func AuthRequired(keycloakClient *keycloak.Client, sessionStore *session.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, span := tracer.Start(r.Context(), "auth.ValidateToken")
			defer span.End()

			authMethod := "jwt"
			if keycloakClient.IsIntrospectionEnabled() {
				authMethod = "introspection"
			}
			span.SetAttributes(attribute.String("auth.method", authMethod))

			token, err := sessionStore.GetToken(r)
			if err != nil {
				span.AddEvent("session.token_missing", trace.WithAttributes(attribute.String("reason", "error")))
				slog.WarnContext(ctx, "Failed to read token from the session store, redirecting to login", "error", err)
				redirectToLogin(w, r)
				return
			} else if token == nil {
				span.AddEvent("session.token_missing", trace.WithAttributes(attribute.String("reason", "not_present")))
				slog.InfoContext(ctx, "No token in the session store, redirecting to login")
				redirectToLogin(w, r)
				return
			}

			var userInfo *UserInfo

			if keycloakClient.IsIntrospectionEnabled() {
				// Tier 2: Token introspection for CookieStore, traditional JWT verification for others
				accessToken := token.AccessToken
				if accessToken == "" {
					span.AddEvent("token.access_token_missing")
					slog.WarnContext(ctx, "No access token in session, redirecting to login")
					sessionStore.Clear(w, r)
					redirectToLogin(w, r)
					return
				}

				result, err := keycloakClient.IntrospectToken(ctx, accessToken)
				if err != nil {
					span.RecordError(err)
					span.SetStatus(codes.Error, "introspection failed")
					slog.ErrorContext(ctx, "Token introspection failed, redirecting to login", "error", err)
					sessionStore.Clear(w, r)
					redirectToLogin(w, r)
					return
				}

				if !result.Active {
					span.AddEvent("token.inactive")
					slog.InfoContext(ctx, "Token inactive (expired or revoked), redirecting to login")
					sessionStore.Clear(w, r)
					redirectToLogin(w, r)
					return
				}

				// Build UserInfo from introspection claims
				userInfo = buildUserInfoFromClaims(ctx, result.Claims)
			} else {
				// Tier 1: Token refresh + JWT verification
				if !token.Valid() {
					span.AddEvent("token.refresh_needed")
					slog.InfoContext(ctx, "Token expired, refreshing")

					tokenSource := keycloakClient.OAuth2Config.TokenSource(ctx, token)
					newToken, err := tokenSource.Token()
					if err != nil {
						span.RecordError(err)
						span.SetStatus(codes.Error, "token refresh failed")
						slog.WarnContext(ctx, "Token refresh failed, redirecting to login", "error", err)
						sessionStore.Clear(w, r)
						redirectToLogin(w, r)
						return
					}

					if err := sessionStore.SaveToken(w, r, newToken); err != nil {
						span.RecordError(err)
						span.SetStatus(codes.Error, "failed to save refreshed token")
						slog.ErrorContext(ctx, "Failed to save refreshed token to session", "error", err)
						sessionStore.Clear(w, r)
						redirectToLogin(w, r)
						return
					}

					span.AddEvent("token.refreshed")
					slog.DebugContext(ctx, "Token refreshed")
					token = newToken
				}

				// Get ID token from oauth2 token
				rawIDToken, ok := token.Extra("id_token").(string)
				if !ok {
					span.AddEvent("token.id_token_missing")
					slog.WarnContext(ctx, "No id_token in session, redirecting to login")
					sessionStore.Clear(w, r)
					redirectToLogin(w, r)
					return
				}

				idToken, err := keycloakClient.Verifier.Verify(ctx, rawIDToken)
				if err != nil {
					span.RecordError(err)
					span.SetStatus(codes.Error, "invalid token")
					slog.InfoContext(ctx, "Invalid id_token, redirecting to login", "error", err)
					sessionStore.Clear(w, r)
					redirectToLogin(w, r)
					return
				}

				var claims struct {
					Sub               string `json:"sub"`
					Email             string `json:"email"`
					Name              string `json:"name"`
					PreferredUsername string `json:"preferred_username"`
					GivenName         string `json:"given_name"`
					FamilyName        string `json:"family_name"`
					EmailVerified     bool   `json:"email_verified"`
					RealmAccess       struct {
						Roles []string `json:"roles"`
					} `json:"realm_access"`
					ResourceAccess map[string]struct {
						Roles []string `json:"roles"`
					} `json:"resource_access"`
				}

				if err := idToken.Claims(&claims); err != nil {
					span.RecordError(err)
					span.SetStatus(codes.Error, "failed to parse claims")
					slog.ErrorContext(ctx, "Failed to parse claims", "error", err)
					http.Error(w, "Failed to parse claims", http.StatusInternalServerError)
					return
				}

				userInfo = &UserInfo{
					Sub:               claims.Sub,
					Email:             claims.Email,
					Name:              claims.Name,
					PreferredUsername: claims.PreferredUsername,
					GivenName:         claims.GivenName,
					FamilyName:        claims.FamilyName,
					EmailVerified:     claims.EmailVerified,
					Roles:             claims.RealmAccess.Roles,
					ClientRoles:       make(map[string][]string),
				}

				// Extract client roles
				for clientID, access := range claims.ResourceAccess {
					userInfo.ClientRoles[clientID] = access.Roles
				}
			}

			span.SetAttributes(attribute.String("user.sub", userInfo.Sub))
			slog.DebugContext(ctx, "Authenticated", "user_sub", userInfo.Sub, "client_count", len(userInfo.ClientRoles))
			ctx = context.WithValue(ctx, UserContextKey, userInfo)

			// Call next handler with traced context
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// buildUserInfoFromClaims constructs UserInfo from introspection result claims using mapstructure
func buildUserInfoFromClaims(ctx context.Context, claims map[string]interface{}) *UserInfo {
	var claimsStruct struct {
		Sub               string `mapstructure:"sub"`
		Email             string `mapstructure:"email"`
		Name              string `mapstructure:"name"`
		PreferredUsername string `mapstructure:"preferred_username"`
		GivenName         string `mapstructure:"given_name"`
		FamilyName        string `mapstructure:"family_name"`
		EmailVerified     bool   `mapstructure:"email_verified"`
		RealmAccess       struct {
			Roles []string `mapstructure:"roles"`
		} `mapstructure:"realm_access"`
		ResourceAccess map[string]struct {
			Roles []string `mapstructure:"roles"`
		} `mapstructure:"resource_access"`
	}

	// Configure decoder to be lenient with type conversions
	config := &mapstructure.DecoderConfig{
		WeaklyTypedInput: true, // Convert types loosely (e.g., allows interface{} -> concrete types)
		Result:           &claimsStruct,
		TagName:          "mapstructure",
	}

	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to create claims decoder", "error", err)
		return &UserInfo{ClientRoles: make(map[string][]string)}
	}

	if err := decoder.Decode(claims); err != nil {
		slog.WarnContext(ctx, "Failed to decode introspection claims", "error", err)
		// Return empty UserInfo rather than failing the request (graceful degradation)
		return &UserInfo{ClientRoles: make(map[string][]string)}
	}

	userInfo := &UserInfo{
		Sub:               claimsStruct.Sub,
		Email:             claimsStruct.Email,
		Name:              claimsStruct.Name,
		PreferredUsername: claimsStruct.PreferredUsername,
		GivenName:         claimsStruct.GivenName,
		FamilyName:        claimsStruct.FamilyName,
		EmailVerified:     claimsStruct.EmailVerified,
		Roles:             claimsStruct.RealmAccess.Roles,
		ClientRoles:       make(map[string][]string),
	}

	// Extract client roles from resource_access
	for clientID, access := range claimsStruct.ResourceAccess {
		userInfo.ClientRoles[clientID] = access.Roles
	}

	return userInfo
}

func GetUserFromContext(ctx context.Context) (*UserInfo, bool) {
	userInfo, ok := ctx.Value(UserContextKey).(*UserInfo)
	return userInfo, ok
}
