package session

import (
	"encoding/gob"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/sessions"
	"golang.org/x/oauth2"
)

const SessionName = "cloak-apps-session"

const (
	sessionDir       = "/tmp/sessions" // TODO: Temporary solution to have things cleaned by OS
	sessionMaxLength = 8192
	keyAccessToken   = "access_token"
	keyTokenType     = "token_type"
	keyRefreshToken  = "refresh_token"
	keyExpiry        = "expiry"
	keyIDToken       = "id_token"
)

func init() {
	// Register oauth2.Token for session encoding
	gob.Register(&oauth2.Token{})
	gob.Register(time.Time{})
}

// Store wraps different session store backends (filesystem, cookie, redis)
type Store struct {
	store     sessions.Store
	maxAge    int
	storeType string // "filesystem", "cookie", or "redis"
}

// StoreConfig holds configuration for creating a session store
type StoreConfig struct {
	Secret    string
	MaxAge    int
	Secure    bool
	StoreType string // "filesystem", "cookie", or "redis"
	RedisURL  string // Only for redis store
}

// NewStore creates a new session store based on configuration
func NewStore(cfg StoreConfig) (*Store, error) {
	switch cfg.StoreType {
	case "filesystem":
		return newFilesystemStore(cfg)
	case "cookie":
		return newCookieStore(cfg)
	case "redis":
		return nil, fmt.Errorf("redis store not yet implemented")
	default:
		return nil, fmt.Errorf("unknown store type: %s", cfg.StoreType)
	}
}

// newFilesystemStore creates a filesystem-based session store (Tier 1)
func newFilesystemStore(cfg StoreConfig) (*Store, error) {
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create session directory: %w", err)
	}

	store := sessions.NewFilesystemStore(sessionDir, []byte(cfg.Secret))
	store.MaxLength(sessionMaxLength)
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   cfg.MaxAge,
		HttpOnly: true,
		Secure:   cfg.Secure,
		SameSite: http.SameSiteLaxMode,
	}

	return &Store{
		store:     store,
		maxAge:    cfg.MaxAge,
		storeType: "filesystem",
	}, nil
}

// newCookieStore creates a cookie-based session store (Tier 2)
func newCookieStore(cfg StoreConfig) (*Store, error) {
	store := sessions.NewCookieStore([]byte(cfg.Secret))
	// Note: CookieStore doesn't have MaxLength method (cookies auto-limited by browser)
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   cfg.MaxAge,
		HttpOnly: true,
		Secure:   cfg.Secure,
		SameSite: http.SameSiteLaxMode,
	}

	return &Store{
		store:     store,
		maxAge:    cfg.MaxAge,
		storeType: "cookie",
	}, nil
}

func (s *Store) SaveToken(w http.ResponseWriter, r *http.Request, token *oauth2.Token) error {
	session, err := s.store.Get(r, SessionName)
	if err != nil {
		return err
	}

	// For CookieStore (Tier 2), only store access_token to reduce cookie size
	// (introspection requires access_token, not id_token)
	if s.storeType == "cookie" {
		// Only store access_token - needed for Keycloak introspection
		session.Values[keyAccessToken] = token.AccessToken
	} else {
		// For filesystem/redis stores, store all tokens (needed for token refresh)
		session.Values[keyAccessToken] = token.AccessToken
		session.Values[keyTokenType] = token.TokenType
		session.Values[keyRefreshToken] = token.RefreshToken
		session.Values[keyExpiry] = token.Expiry

		// Store ID token if present
		if idToken, ok := token.Extra("id_token").(string); ok {
			session.Values[keyIDToken] = idToken
		}
	}

	return session.Save(r, w)
}

func (s *Store) GetToken(r *http.Request) (*oauth2.Token, error) {
	session, err := s.store.Get(r, SessionName)
	if err != nil {
		return nil, err
	}

	// For CookieStore (Tier 2), only access_token is stored
	if s.storeType == "cookie" {
		accessToken, ok := session.Values[keyAccessToken].(string)
		if !ok || accessToken == "" {
			return nil, nil
		}

		// Return a minimal token with only access_token
		// (introspection will be used for validation, not token.Valid())
		token := &oauth2.Token{
			AccessToken: accessToken,
		}
		return token, nil
	}

	// For filesystem/redis stores, retrieve full token
	accessToken, ok := session.Values[keyAccessToken].(string)
	if !ok || accessToken == "" {
		return nil, nil
	}

	// Reconstruct token
	token := &oauth2.Token{
		AccessToken:  accessToken,
		TokenType:    getStringValue(session.Values, keyTokenType),
		RefreshToken: getStringValue(session.Values, keyRefreshToken),
	}

	// Get expiry
	if expiry, ok := session.Values[keyExpiry].(time.Time); ok {
		token.Expiry = expiry
	}

	// Add ID token as extra
	if idToken, ok := session.Values[keyIDToken].(string); ok && idToken != "" {
		token = token.WithExtra(map[string]interface{}{
			"id_token": idToken,
		})
	}

	return token, nil
}

func (s *Store) Get(r *http.Request, name string) (*sessions.Session, error) {
	return s.store.Get(r, name)
}

func (s *Store) Clear(w http.ResponseWriter, r *http.Request) error {
	session, err := s.store.Get(r, SessionName)
	if err != nil {
		// Even if we can't get the session, try to clear it
		session, _ = s.store.New(r, SessionName)
	}

	session.Values = make(map[interface{}]interface{}) // Clear all values
	session.Options.MaxAge = -1                        // Delete cookie

	return session.Save(r, w)
}

func getStringValue(values map[interface{}]interface{}, key string) string {
	if val, ok := values[key].(string); ok {
		return val
	}
	return ""
}
