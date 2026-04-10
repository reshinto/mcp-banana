package gemini

import (
	"context"
	"fmt"
	"sync"
)

// ClientCache manages a pool of Gemini clients, one per unique API key.
// The default client (from server config) is always available and returned
// when no per-request key is provided. Additional clients are created on demand
// and cached for reuse across requests from the same user.
type ClientCache struct {
	mutex          sync.RWMutex
	defaultClient  *Client
	clients        map[string]*Client
	timeoutSecs    int
	proConcurrency int
}

// NewClientCache creates a cache with a pre-configured default client.
// The default client is used when no per-request API key is present.
// timeoutSecs and proConcurrency are applied to all newly created per-user clients.
func NewClientCache(defaultClient *Client, timeoutSecs int, proConcurrency int) *ClientCache {
	return &ClientCache{
		defaultClient:  defaultClient,
		clients:        make(map[string]*Client),
		timeoutSecs:    timeoutSecs,
		proConcurrency: proConcurrency,
	}
}

// GetClient returns a Gemini client for the given API key.
// If apiKey is empty, the default client is returned.
// If a client for apiKey is already cached, it is returned without creating a new one.
// Otherwise a new client is created, cached, and returned.
func (cache *ClientCache) GetClient(ctx context.Context, apiKey string) (*Client, error) {
	if apiKey == "" {
		return cache.defaultClient, nil
	}

	// Fast path: check cache under read lock.
	cache.mutex.RLock()
	existing, found := cache.clients[apiKey]
	cache.mutex.RUnlock()
	if found {
		return existing, nil
	}

	// Slow path: create and cache a new client under write lock.
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	// Re-check after acquiring write lock to avoid double-create.
	if existing, found = cache.clients[apiKey]; found {
		return existing, nil
	}

	created, clientError := NewClient(ctx, apiKey, cache.timeoutSecs, cache.proConcurrency)
	if clientError != nil {
		return nil, fmt.Errorf("failed to create per-user gemini client: %w", clientError)
	}

	cache.clients[apiKey] = created
	return created, nil
}
