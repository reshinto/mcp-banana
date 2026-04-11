package gemini

import (
	"context"
	"fmt"
	"sync"
)

// ClientCache manages a pool of Gemini clients, one per unique API key.
// All clients are created on demand from per-request keys resolved by
// middleware and cached for reuse across requests from the same user.
type ClientCache struct {
	mutex          sync.RWMutex
	clients        map[string]GeminiService
	timeoutSecs    int
	proConcurrency int
}

// NewClientCache creates an empty client cache.
// timeoutSecs and proConcurrency are applied to all newly created per-user clients.
func NewClientCache(timeoutSecs int, proConcurrency int) *ClientCache {
	return &ClientCache{
		clients:        make(map[string]GeminiService),
		timeoutSecs:    timeoutSecs,
		proConcurrency: proConcurrency,
	}
}

// afterReadMiss is called in tests between the read-lock miss and write-lock
// acquisition to create a deterministic window for exercising the double-check
// path. It is nil in production and has no runtime cost.
var afterReadMiss func()

// GetClient returns a Gemini client for the given API key.
// If apiKey is empty, an error is returned — all requests must have a key
// resolved by middleware. If a client for apiKey is already cached, it is
// returned without creating a new one. Otherwise a new client is created,
// cached, and returned.
func (cache *ClientCache) GetClient(ctx context.Context, apiKey string) (GeminiService, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	// Fast path: check cache under read lock.
	cache.mutex.RLock()
	existing, found := cache.clients[apiKey]
	cache.mutex.RUnlock()
	if found {
		return existing, nil
	}

	// afterReadMiss is nil in production; tests set it to inject a rendezvous
	// point that makes the double-check path deterministically reachable.
	if afterReadMiss != nil {
		afterReadMiss()
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

// SetClientForKey injects a pre-built GeminiService into the cache for the given key.
// This is intended for tests that need to control client behavior without
// going through the real client factory.
func (cache *ClientCache) SetClientForKey(apiKey string, client GeminiService) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	cache.clients[apiKey] = client
}
