package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/effiware/cloak-apps/internal/keycloak"
	"github.com/effiware/cloak-apps/internal/server/session"
	"golang.org/x/oauth2"
)

type Handlers struct {
	keycloakClient *keycloak.Client
	sessionStore   *session.Store
}

func NewHandlers(keycloakClient *keycloak.Client, sessionStore *session.Store) *Handlers {
	return &Handlers{
		keycloakClient: keycloakClient,
		sessionStore:   sessionStore,
	}
}

// HandleLogin redirects to Keycloak login
func (h *Handlers) HandleLogin(w http.ResponseWriter, r *http.Request) {
	// Generate state parameter for CSRF protection
	state, err := generateRandomState()
	if err != nil {
		slog.Error("Failed to generate state,", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Store state in session for verification
	sess, _ := h.sessionStore.Get(r, session.SessionName)
	sess.Values["oauth_state"] = state
	if err := sess.Save(r, w); err != nil {
		slog.Error("Failed to save session,", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Redirect to Keycloak authorization URL
	authURL := h.keycloakClient.OAuth2Config.AuthCodeURL(state, oauth2.AccessTypeOffline)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// HandleCallback processes OAuth2 callback
func (h *Handlers) HandleCallback(w http.ResponseWriter, r *http.Request) {
	// Verify state parameter
	sess, err := h.sessionStore.Get(r, session.SessionName)
	if err != nil {
		slog.Error("Failed to get session,", "error", err)
		http.Error(w, "Invalid session", http.StatusBadRequest)
		return
	}

	savedState, ok := sess.Values["oauth_state"].(string)
	if !ok || savedState == "" {
		slog.Error("Failed to get saved state")
		http.Error(w, "Missing state", http.StatusBadRequest)
		return
	}

	receivedState := r.URL.Query().Get("state")
	if receivedState != savedState {
		slog.Error("Received state doesn't match saved state")
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return
	}

	// Clear state from session
	delete(sess.Values, "oauth_state")
	if err := sess.Save(r, w); err != nil {
		slog.Error("Failed to save state after deletion,", "error", err)
		http.Error(w, "Failed to save state after deletion", http.StatusBadRequest)
		return
	}

	// Check for error from Keycloak
	if errorParam := r.URL.Query().Get("error"); errorParam != "" {
		errorDesc := r.URL.Query().Get("error_description")
		slog.Error("OAuth,", "error", errorParam, "description", errorDesc)
		http.Error(w, fmt.Sprintf("Authentication failed: %s", errorDesc), http.StatusUnauthorized)
		return
	}

	// Exchange code for token
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Missing code parameter", http.StatusBadRequest)
		return
	}

	token, err := h.keycloakClient.OAuth2Config.Exchange(r.Context(), code)
	if err != nil {
		slog.Error("Failed to exchange code for token,", "error", err)
		http.Error(w, "Failed to authenticate", http.StatusInternalServerError)
		return
	}

	// Save token to session
	if err := h.sessionStore.SaveToken(w, r, token); err != nil {
		slog.Error("Failed to save token,", "error", err)
		http.Error(w, "Failed to save session", http.StatusInternalServerError)
		return
	}

	// Redirect to home page
	http.Redirect(w, r, "/", http.StatusFound)
}

// HandleLogout clears session and redirects to Keycloak logout
func (h *Handlers) HandleLogout(w http.ResponseWriter, r *http.Request) {
	token, _ := h.sessionStore.GetToken(r)

	if err := h.sessionStore.Clear(w, r); err != nil {
		slog.Error("Failed to clear session,", "error", err)
		http.Error(w, "Failed to clear session,", http.StatusBadRequest)
		return
	}

	logoutURL := fmt.Sprintf("%s/protocol/openid-connect/logout", h.keycloakClient.IssuerURL)

	// Add post_logout_redirect_uri if we have an ID token
	if token != nil {
		if idToken, ok := token.Extra("id_token").(string); ok && idToken != "" {
			// Redirect back to home page after logout
			redirectURI := h.keycloakClient.OAuth2Config.RedirectURL
			// Extract base URL from redirect URI (remove /auth/callback)
			baseURL := redirectURI[:len(redirectURI)-len("/auth/callback")]
			logoutURL = fmt.Sprintf("%s?id_token_hint=%s&post_logout_redirect_uri=%s",
				logoutURL, idToken, baseURL)
		}
	}

	http.Redirect(w, r, logoutURL, http.StatusFound)
}

// HandleSSORedirect redirects user to specific client's SSO login
func (h *Handlers) HandleSSORedirect(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client")
	if clientID == "" {
		http.Error(w, "Missing client parameter", http.StatusBadRequest)
		return
	}

	// Build Keycloak authorization URL for the specific client
	// This will redirect user to the target application after Keycloak authentication
	authURL := fmt.Sprintf("%s/protocol/openid-connect/auth?client_id=%s&response_type=code",
		h.keycloakClient.IssuerURL, clientID)

	http.Redirect(w, r, authURL, http.StatusFound)
}

// generateRandomState creates a random state string for CSRF protection
func generateRandomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
