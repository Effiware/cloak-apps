package middlewares

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/effiware/cloak-apps/internal/keycloak"
	"github.com/effiware/cloak-apps/internal/server/session"
)

type contextKey string

const UserContextKey contextKey = "user"

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
			token, err := sessionStore.GetToken(r)
			if err != nil {
				slog.Warn("Failed to obtain token from the session store, redirecting to the login page")
				http.Redirect(w, r, "/auth/login", http.StatusFound)
				return
			} else if token == nil {
				slog.Info("Token not present in the session store, redirecting to login page")
				http.Redirect(w, r, "/auth/login", http.StatusFound)
				return
			}

			var userInfo *UserInfo

			// Use introspection for CookieStore (Tier 2), traditional JWT verification for others
			if keycloakClient.IsIntrospectionEnabled() {
				// Tier 2: Token introspection
				accessToken := token.AccessToken
				if accessToken == "" {
					slog.Warn("No access_token in session, redirecting to login page")
					sessionStore.Clear(w, r)
					http.Redirect(w, r, "/auth/login", http.StatusFound)
					return
				}

				result, err := keycloakClient.IntrospectToken(r.Context(), accessToken)
				if err != nil {
					slog.Error("Token introspection failed, redirecting to login page", "error", err)
					sessionStore.Clear(w, r)
					http.Redirect(w, r, "/auth/login", http.StatusFound)
					return
				}

				if !result.Active {
					slog.Warn("Token is not active (expired or revoked), redirecting to login page")
					sessionStore.Clear(w, r)
					http.Redirect(w, r, "/auth/login", http.StatusFound)
					return
				}

				// Build UserInfo from introspection claims
				userInfo = buildUserInfoFromClaims(result.Claims)
			} else {
				// Tier 1: Token refresh + JWT verification
				if !token.Valid() {
					slog.Info("Token expired, refreshing using refresh_token")

					tokenSource := keycloakClient.OAuth2Config.TokenSource(r.Context(), token)
					newToken, err := tokenSource.Token()
					if err != nil {
						slog.Warn("Token refresh failed, redirecting to login page", "error", err)
						sessionStore.Clear(w, r)
						http.Redirect(w, r, "/auth/login", http.StatusFound)
						return
					}

					if err := sessionStore.SaveToken(w, r, newToken); err != nil {
						slog.Error("Failed to save refreshed token to session", "error", err)
						sessionStore.Clear(w, r)
						http.Redirect(w, r, "/auth/login", http.StatusFound)
						return
					}

					slog.Debug("Token successfully refreshed")
					token = newToken
				}

				// Get ID token from oauth2 token
				rawIDToken, ok := token.Extra("id_token").(string)
				if !ok {
					slog.Warn("No id_token, redirecting to the login page")
					sessionStore.Clear(w, r)
					http.Redirect(w, r, "/auth/login", http.StatusFound)
					return
				}

				// Verify ID token
				idToken, err := keycloakClient.Verifier.Verify(r.Context(), rawIDToken)
				if err != nil {
					slog.Error("Invalid token, redirecting to login page")
					sessionStore.Clear(w, r)
					http.Redirect(w, r, "/auth/login", http.StatusFound)
					return
				}

				// Parse claims
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
					slog.Error("Failed to parse claims,", "error", err)
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

			slog.Debug("Authenticated", "User", userInfo.PreferredUsername, "ClientRoles", userInfo.ClientRoles)
			ctx := context.WithValue(r.Context(), UserContextKey, userInfo)

			// Call next handler
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// buildUserInfoFromClaims constructs UserInfo from introspection result claims
func buildUserInfoFromClaims(claims map[string]interface{}) *UserInfo {
	userInfo := &UserInfo{
		ClientRoles: make(map[string][]string),
	}

	// Extract simple string fields
	if sub, ok := claims["sub"].(string); ok {
		userInfo.Sub = sub
	}
	if email, ok := claims["email"].(string); ok {
		userInfo.Email = email
	}
	if name, ok := claims["name"].(string); ok {
		userInfo.Name = name
	}
	if preferredUsername, ok := claims["preferred_username"].(string); ok {
		userInfo.PreferredUsername = preferredUsername
	}
	if givenName, ok := claims["given_name"].(string); ok {
		userInfo.GivenName = givenName
	}
	if familyName, ok := claims["family_name"].(string); ok {
		userInfo.FamilyName = familyName
	}
	if emailVerified, ok := claims["email_verified"].(bool); ok {
		userInfo.EmailVerified = emailVerified
	}

	// Extract realm roles
	if realmAccess, ok := claims["realm_access"].(map[string]interface{}); ok {
		if roles, ok := realmAccess["roles"].([]interface{}); ok {
			userInfo.Roles = make([]string, 0, len(roles))
			for _, role := range roles {
				if roleStr, ok := role.(string); ok {
					userInfo.Roles = append(userInfo.Roles, roleStr)
				}
			}
		}
	}

	// Extract resource (client) roles
	if resourceAccess, ok := claims["resource_access"].(map[string]interface{}); ok {
		for clientID, access := range resourceAccess {
			if accessMap, ok := access.(map[string]interface{}); ok {
				if roles, ok := accessMap["roles"].([]interface{}); ok {
					clientRoles := make([]string, 0, len(roles))
					for _, role := range roles {
						if roleStr, ok := role.(string); ok {
							clientRoles = append(clientRoles, roleStr)
						}
					}
					userInfo.ClientRoles[clientID] = clientRoles
				}
			}
		}
	}

	return userInfo
}

func GetUserFromContext(ctx context.Context) (*UserInfo, bool) {
	userInfo, ok := ctx.Value(UserContextKey).(*UserInfo)
	return userInfo, ok
}
