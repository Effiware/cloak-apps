package session

import (
	"encoding/gob"
	"log/slog"
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

// Store currently is implemented as a [FileSystemStore](https://pkg.go.dev/github.com/gorilla/sessions#FilesystemStore)
// TODO: Consider using SQL/KV -based alternative
type Store struct {
	store  sessions.Store
	maxAge int
}

func NewStore(secret string, maxAge int, secure bool) *Store {
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		slog.Error("Failed to create session directory,", "error", err)
		os.Exit(1)
	}

	store := sessions.NewFilesystemStore(sessionDir, []byte(secret))
	// Even with FilesystemStore, the session ID and metadata are stored in cookies
	store.MaxLength(sessionMaxLength)
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}

	return &Store{
		store:  store,
		maxAge: maxAge,
	}
}

func (s *Store) SaveToken(w http.ResponseWriter, r *http.Request, token *oauth2.Token) error {
	session, err := s.store.Get(r, SessionName)
	if err != nil {
		return err
	}

	session.Values[keyAccessToken] = token.AccessToken
	session.Values[keyTokenType] = token.TokenType
	session.Values[keyRefreshToken] = token.RefreshToken
	session.Values[keyExpiry] = token.Expiry

	// Store ID token if present
	if idToken, ok := token.Extra("id_token").(string); ok {
		session.Values[keyIDToken] = idToken
	}

	return session.Save(r, w)
}

func (s *Store) GetToken(r *http.Request) (*oauth2.Token, error) {
	session, err := s.store.Get(r, SessionName)
	if err != nil {
		return nil, err
	}

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
