package middleware

import (
	"context"
	"log"
	"net/http"

	"github.com/effiware/cloak-apps/internal/keycloak"
	"github.com/effiware/cloak-apps/internal/server/session"
	"github.com/golang-jwt/jwt/v5"
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
			// Get token from session
			token, err := sessionStore.GetToken(r)
			if err != nil || token == nil {
				// Redirect to login
				http.Redirect(w, r, "/auth/login", http.StatusFound)
				return
			}

			// Verify token is still valid
			if !token.Valid() {
				// Token expired, redirect to login
				sessionStore.Clear(w, r)
				http.Redirect(w, r, "/auth/login", http.StatusFound)
				return
			}

			// Get ID token from oauth2 token
			rawIDToken, ok := token.Extra("id_token").(string)
			if !ok {
				// No ID token, redirect to login
				sessionStore.Clear(w, r)
				http.Redirect(w, r, "/auth/login", http.StatusFound)
				return
			}

			// Verify ID token
			idToken, err := keycloakClient.Verifier.Verify(r.Context(), rawIDToken)
			if err != nil {
				// Invalid token, redirect to login
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
				http.Error(w, "Failed to parse claims", http.StatusInternalServerError)
				return
			}

			// Extract user info
			userInfo := &UserInfo{
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

			// Debug logging for token claims
			log.Printf("[DEBUG] User %s authenticated - ClientRoles: %+v",
				userInfo.PreferredUsername, userInfo.ClientRoles)
			log.Printf("[DEBUG] User %s has roles in %d clients",
				userInfo.PreferredUsername, len(userInfo.ClientRoles))

			// Store user info in context
			ctx := context.WithValue(r.Context(), UserContextKey, userInfo)

			// Call next handler
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetUserFromContext(ctx context.Context) (*UserInfo, bool) {
	userInfo, ok := ctx.Value(UserContextKey).(*UserInfo)
	return userInfo, ok
}

// OptionalAuth middleware that doesn't redirect if not authenticated
// Useful for pages that can work both with and without authentication
func OptionalAuth(keycloakClient *keycloak.Client, sessionStore *session.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get token from session
			token, err := sessionStore.GetToken(r)
			if err != nil || token == nil || !token.Valid() {
				// No valid token, continue without user info
				next.ServeHTTP(w, r)
				return
			}

			// Get ID token from oauth2 token
			rawIDToken, ok := token.Extra("id_token").(string)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}

			// Verify ID token
			idToken, err := keycloakClient.Verifier.Verify(r.Context(), rawIDToken)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			// Parse claims
			var claims jwt.MapClaims
			if err := idToken.Claims(&claims); err != nil {
				next.ServeHTTP(w, r)
				return
			}

			// Extract basic user info (simplified for optional auth)
			userInfo := &UserInfo{
				Sub:               getStringClaim(claims, "sub"),
				Email:             getStringClaim(claims, "email"),
				Name:              getStringClaim(claims, "name"),
				PreferredUsername: getStringClaim(claims, "preferred_username"),
			}

			// Store user info in context
			ctx := context.WithValue(r.Context(), UserContextKey, userInfo)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func getStringClaim(claims jwt.MapClaims, key string) string {
	if val, ok := claims[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}
