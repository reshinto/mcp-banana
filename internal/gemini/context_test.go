package gemini

import (
	"context"
	"testing"
)

// TestWithAPIKey_StoresKeyInContext verifies that WithAPIKey stores the API key
// in the context and that APIKeyFromContext retrieves the exact same value.
func TestWithAPIKey_StoresKeyInContext(test *testing.T) {
	original := context.Background()
	enriched := WithAPIKey(original, "test-api-key-abc123")

	result := APIKeyFromContext(enriched)
	if result != "test-api-key-abc123" {
		test.Errorf("expected 'test-api-key-abc123', got %q", result)
	}
}

// TestAPIKeyFromContext_EmptyWhenNotSet verifies that APIKeyFromContext returns
// an empty string when no key has been stored in the context.
func TestAPIKeyFromContext_EmptyWhenNotSet(test *testing.T) {
	result := APIKeyFromContext(context.Background())
	if result != "" {
		test.Errorf("expected empty string for context with no key, got %q", result)
	}
}

// TestWithAPIKey_OriginalContextUnchanged verifies that WithAPIKey returns a new
// context and does not modify the original.
func TestWithAPIKey_OriginalContextUnchanged(test *testing.T) {
	original := context.Background()
	_ = WithAPIKey(original, "injected-key")

	// The original context must still return empty.
	result := APIKeyFromContext(original)
	if result != "" {
		test.Errorf("expected original context to be unchanged, but got %q", result)
	}
}

// TestWithAPIKey_OverridesExistingKey verifies that calling WithAPIKey twice
// with different keys results in the most recent key being returned.
func TestWithAPIKey_OverridesExistingKey(test *testing.T) {
	firstCtx := WithAPIKey(context.Background(), "first-key")
	secondCtx := WithAPIKey(firstCtx, "second-key")

	result := APIKeyFromContext(secondCtx)
	if result != "second-key" {
		test.Errorf("expected second key to override first, got %q", result)
	}
}
