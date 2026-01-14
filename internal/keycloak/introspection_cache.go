package keycloak

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// IntrospectionResult represents cached introspection response
type IntrospectionResult struct {
	Active bool
	Claims map[string]interface{}
}

// cacheEntry holds cached introspection result with expiration time
type cacheEntry struct {
	result    *IntrospectionResult
	expiresAt time.Time
}

// IntrospectionCache provides thread-safe caching for token introspection results
type IntrospectionCache struct {
	cache   map[string]*cacheEntry
	ttl     time.Duration
	mu      sync.RWMutex
	done    chan struct{}
	cleanup *time.Ticker
}

// NewIntrospectionCache creates a new introspection cache with periodic cleanup
func NewIntrospectionCache(ttl time.Duration) *IntrospectionCache {
	cache := &IntrospectionCache{
		cache:   make(map[string]*cacheEntry),
		ttl:     ttl,
		done:    make(chan struct{}),
		cleanup: time.NewTicker(ttl * 2), // Cleanup twice per TTL period
	}

	// Start background cleanup goroutine
	go cache.cleanupLoop()

	return cache
}

// Get retrieves cached introspection result for the given token
// Returns (result, true) if cached and not expired, (nil, false) otherwise
func (ic *IntrospectionCache) Get(token string) (*IntrospectionResult, bool) {
	key := hashToken(token)

	ic.mu.RLock()
	defer ic.mu.RUnlock()

	entry, exists := ic.cache[key]
	if !exists {
		return nil, false
	}

	// Check if expired
	if time.Now().After(entry.expiresAt) {
		return nil, false
	}

	return entry.result, true
}

// Set stores an introspection result in the cache
func (ic *IntrospectionCache) Set(token string, result *IntrospectionResult) {
	key := hashToken(token)

	ic.mu.Lock()
	defer ic.mu.Unlock()

	ic.cache[key] = &cacheEntry{
		result:    result,
		expiresAt: time.Now().Add(ic.ttl),
	}
}

// cleanupLoop runs periodically to remove expired cache entries
func (ic *IntrospectionCache) cleanupLoop() {
	for {
		select {
		case <-ic.done:
			ic.cleanup.Stop()
			return
		case <-ic.cleanup.C:
			ic.removeExpired()
		}
	}
}

// removeExpired removes all expired entries from the cache
func (ic *IntrospectionCache) removeExpired() {
	now := time.Now()

	ic.mu.Lock()
	defer ic.mu.Unlock()

	for key, entry := range ic.cache {
		if now.After(entry.expiresAt) {
			delete(ic.cache, key)
		}
	}
}

// Close stops the cleanup goroutine and clears the cache
func (ic *IntrospectionCache) Close() {
	close(ic.done)

	ic.mu.Lock()
	defer ic.mu.Unlock()
	ic.cache = make(map[string]*cacheEntry)
}

// hashToken creates a SHA-256 hash of the token for use as cache key
// This prevents storing raw tokens in memory and provides fixed-size keys
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
