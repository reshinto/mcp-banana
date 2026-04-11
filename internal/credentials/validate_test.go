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
