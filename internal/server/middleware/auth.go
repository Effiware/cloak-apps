package middleware

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
				slog.Warn("Failed to obtain token from the session store")
				http.Redirect(w, r, "/auth/login", http.StatusFound)
				return
			} else if token == nil {
				slog.Info("Token not present in the session store")
				http.Redirect(w, r, "/auth/login", http.StatusFound)
				return
			}

			if !token.Valid() {
				slog.Debug("Token expired for")
				sessionStore.Clear(w, r)
				http.Redirect(w, r, "/auth/login", http.StatusFound)
				return
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
				slog.Error("Invalid token, redirecting to the login page")
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

			slog.Debug("Authenticated", "User", userInfo.PreferredUsername, "ClientRoles", userInfo.ClientRoles)
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
