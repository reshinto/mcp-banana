package credentials

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/genai"
)

// geminiKeyValidator is the function used to validate a Gemini API key.
// It is a package-level variable so tests can replace it without making real API calls.
var geminiKeyValidator = func(ctx context.Context, apiKey string) error {
	client, clientError := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if clientError != nil {
		return fmt.Errorf("invalid gemini API key: %w", clientError)
	}

	// List models as a lightweight validation call
	page, listError := client.Models.List(ctx, &genai.ListModelsConfig{PageSize: 1})
	if listError != nil {
		return fmt.Errorf("gemini API key validation failed: %w", listError)
	}
	if len(page.Items) == 0 {
		return errors.New("gemini API key validation returned no results")
	}
	return nil
}

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
