package gemini

import "context"

// contextKey is the private key type for gemini context values.
// Using a named type prevents collisions with keys from other packages.
type contextKey string

// apiKeyContextKey is the context key for storing a per-request Gemini API key.
const apiKeyContextKey contextKey = "gemini-api-key"

// WithAPIKey stores a Gemini API key in the context for use by per-request client resolution.
// The key is available via APIKeyFromContext in downstream handlers.
func WithAPIKey(ctx context.Context, apiKey string) context.Context {
	return context.WithValue(ctx, apiKeyContextKey, apiKey)
}

// APIKeyFromContext retrieves the per-request Gemini API key stored by WithAPIKey.
// Returns an empty string if no key is present in the context.
func APIKeyFromContext(ctx context.Context) string {
	value, _ := ctx.Value(apiKeyContextKey).(string)
	return value
}
