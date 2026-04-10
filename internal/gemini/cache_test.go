package gemini

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/genai"
)

// newTestClientCache creates a ClientCache with a pre-built test default client.
func newTestClientCache(timeoutSecs int, proConcurrency int) *ClientCache {
	defaultClient := newTestClient(&mockContentGenerator{})
	return NewClientCache(defaultClient, timeoutSecs, proConcurrency)
}

// TestClientCache_DefaultClient verifies that an empty key returns the default client.
func TestClientCache_DefaultClient(test *testing.T) {
	cache := newTestClientCache(30, 2)

	result, clientError := cache.GetClient(context.Background(), "")
	if clientError != nil {
		test.Fatalf("unexpected error: %v", clientError)
	}
	if result != cache.defaultClient {
		test.Error("expected default client for empty key")
	}
}

// TestClientCache_CustomKey verifies that a non-empty key creates and caches a new client.
func TestClientCache_CustomKey(test *testing.T) {
	cache := newTestClientCache(30, 2)

	result, clientError := cache.GetClient(context.Background(), "custom-api-key-xyz")
	if clientError != nil {
		test.Fatalf("unexpected error: %v", clientError)
	}
	if result == nil {
		test.Fatal("expected non-nil client for custom key")
	}
	if result == cache.defaultClient {
		test.Error("expected a different client for custom key, not the default")
	}
}

// TestClientCache_CachedClient verifies that the same key returns the same client instance.
func TestClientCache_CachedClient(test *testing.T) {
	cache := newTestClientCache(30, 2)

	firstClient, firstError := cache.GetClient(context.Background(), "repeat-key")
	if firstError != nil {
		test.Fatalf("first call: unexpected error: %v", firstError)
	}

	secondClient, secondError := cache.GetClient(context.Background(), "repeat-key")
	if secondError != nil {
		test.Fatalf("second call: unexpected error: %v", secondError)
	}

	if firstClient != secondClient {
		test.Error("expected same client pointer on second call (cache miss would create a new one)")
	}
}

// TestClientCache_FactoryError verifies that a factory failure returns an error.
func TestClientCache_FactoryError(test *testing.T) {
	original := genaiClientFactory
	defer func() { genaiClientFactory = original }()

	genaiClientFactory = func(_ context.Context, _ *genai.ClientConfig) (*genai.Client, error) {
		return nil, errors.New("simulated factory failure")
	}

	cache := newTestClientCache(30, 2)
	_, clientError := cache.GetClient(context.Background(), "failing-key")
	if clientError == nil {
		test.Fatal("expected error when factory fails")
	}
}
