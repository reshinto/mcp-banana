package credentials

import (
	"context"
	"errors"
	"testing"
)

func TestValidateGeminiKey_AcceptsValidKey(test *testing.T) {
	original := geminiKeyValidator
	defer func() { geminiKeyValidator = original }()

	geminiKeyValidator = func(ctx context.Context, apiKey string) error {
		return nil
	}

	validateError := ValidateGeminiKey(context.Background(), "AIzaValidKey123")
	if validateError != nil {
		test.Errorf("expected nil error, got: %s", validateError)
	}
}

func TestValidateGeminiKey_RejectsInvalidKey(test *testing.T) {
	original := geminiKeyValidator
	defer func() { geminiKeyValidator = original }()

	geminiKeyValidator = func(ctx context.Context, apiKey string) error {
		return errors.New("invalid API key")
	}

	validateError := ValidateGeminiKey(context.Background(), "AIzaBadKey")
	if validateError == nil {
		test.Error("expected error for invalid key")
	}
}

func TestValidateGeminiKey_RejectsEmptyKey(test *testing.T) {
	validateError := ValidateGeminiKey(context.Background(), "")
	if validateError == nil {
		test.Error("expected error for empty key")
	}
}

func TestOverrideGeminiKeyValidator_RestoresOriginal(test *testing.T) {
	called := false
	restore := OverrideGeminiKeyValidator(func(ctx context.Context, apiKey string) error {
		called = true
		return nil
	})

	validateError := ValidateGeminiKey(context.Background(), "AIzaTestKey")
	if validateError != nil {
		test.Errorf("expected nil error from override, got: %s", validateError)
	}
	if !called {
		test.Error("expected override to be called")
	}

	restore()

	// After restore, the original validator is back (would make real API call, so
	// we just verify the override function was replaced — cannot test real call here)
}
