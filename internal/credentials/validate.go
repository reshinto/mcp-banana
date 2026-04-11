package credentials

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/genai"
)

// modelLister abstracts the Models.List call so that validateWithClient can be
// tested without making real HTTP requests.
type modelLister interface {
	List(ctx context.Context, config *genai.ListModelsConfig) (genai.Page[genai.Model], error)
}

// validateWithClient performs Gemini key validation using the provided
// modelLister. Extracted from the production closure so it can be unit-tested
// with a mock lister.
func validateWithClient(ctx context.Context, lister modelLister) error {
	page, listError := lister.List(ctx, &genai.ListModelsConfig{PageSize: 1})
	if listError != nil {
		return fmt.Errorf("gemini API key validation failed: %w", listError)
	}
	if len(page.Items) == 0 {
		return errors.New("gemini API key validation returned no results")
	}
	return nil
}

// newGenaiClient creates a new genai client. Package-level variable so tests
// can inject a mock without making real network calls.
var newGenaiClient = func(ctx context.Context, config *genai.ClientConfig) (*genai.Client, error) {
	return genai.NewClient(ctx, config)
}

// defaultGeminiKeyValidator creates a real genai client and validates the key
// by listing models. Extracted as a named function so the closure variable
// assignment is trivially thin and fully coverable.
func defaultGeminiKeyValidator(ctx context.Context, apiKey string) error {
	client, clientError := newGenaiClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if clientError != nil {
		return fmt.Errorf("invalid gemini API key: %w", clientError)
	}

	return validateWithClient(ctx, client.Models)
}

// geminiKeyValidator is the function used to validate a Gemini API key.
// It is a package-level variable so tests can replace it without making real API calls.
var geminiKeyValidator = defaultGeminiKeyValidator

// OverrideGeminiKeyValidator replaces the Gemini key validation function and returns
// a restore function that reinstates the original. Intended for use in tests outside
// this package to avoid real API calls.
func OverrideGeminiKeyValidator(override func(ctx context.Context, apiKey string) error) func() {
	original := geminiKeyValidator
	geminiKeyValidator = override
	return func() { geminiKeyValidator = original }
}

// ValidateGeminiKey checks whether the given API key is a valid Gemini API key
// by making a lightweight API call. Returns nil if valid, an error otherwise.
func ValidateGeminiKey(ctx context.Context, apiKey string) error {
	if apiKey == "" {
		return errors.New("gemini API key must not be empty")
	}
	return geminiKeyValidator(ctx, apiKey)
}
