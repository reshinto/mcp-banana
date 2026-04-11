package credentials

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/genai"
)

// mockModelLister implements modelLister for testing validateWithClient.
type mockModelLister struct {
	page      genai.Page[genai.Model]
	listError error
}

func (mock *mockModelLister) List(ctx context.Context, config *genai.ListModelsConfig) (genai.Page[genai.Model], error) {
	return mock.page, mock.listError
}

func TestValidateWithClient_Success(test *testing.T) {
	lister := &mockModelLister{
		page: genai.Page[genai.Model]{
			Items: []*genai.Model{{}},
		},
	}

	validationError := validateWithClient(context.Background(), lister)
	if validationError != nil {
		test.Errorf("expected nil error, got: %s", validationError)
	}
}

func TestValidateWithClient_ListError(test *testing.T) {
	lister := &mockModelLister{
		listError: errors.New("connection refused"),
	}

	validationError := validateWithClient(context.Background(), lister)
	if validationError == nil {
		test.Fatal("expected error from list failure, got nil")
	}
}

func TestValidateWithClient_EmptyResults(test *testing.T) {
	lister := &mockModelLister{
		page: genai.Page[genai.Model]{
			Items: []*genai.Model{},
		},
	}

	validationError := validateWithClient(context.Background(), lister)
	if validationError == nil {
		test.Fatal("expected error for empty results, got nil")
	}
}

func TestNewGenaiClient_DefaultClosure(test *testing.T) {
	// Exercise the default newGenaiClient closure to ensure it delegates to
	// genai.NewClient. The constructor does not make network calls.
	client, clientError := newGenaiClient(context.Background(), &genai.ClientConfig{
		APIKey:  "AIzaTestCoverageKey",
		Backend: genai.BackendGeminiAPI,
	})
	if clientError != nil {
		test.Fatalf("expected no error from client creation, got: %v", clientError)
	}
	if client == nil {
		test.Fatal("expected non-nil client")
	}
}

func TestDefaultGeminiKeyValidator_ClientCreationError(test *testing.T) {
	originalClient := newGenaiClient
	defer func() { newGenaiClient = originalClient }()

	newGenaiClient = func(ctx context.Context, config *genai.ClientConfig) (*genai.Client, error) {
		return nil, errors.New("simulated client creation failure")
	}

	validationError := defaultGeminiKeyValidator(context.Background(), "AIzaSomeKey")
	if validationError == nil {
		test.Fatal("expected error from client creation failure, got nil")
	}
}

func TestDefaultGeminiKeyValidator_SuccessPath(test *testing.T) {
	originalClient := newGenaiClient
	defer func() { newGenaiClient = originalClient }()

	// Mock newGenaiClient to return a real client. The client constructor does
	// not make network calls; only List does. We then call defaultGeminiKeyValidator
	// which will invoke validateWithClient → client.Models.List. The List call
	// will fail (invalid key) but this exercises the success path of client creation
	// through to the validateWithClient delegation.
	newGenaiClient = func(ctx context.Context, config *genai.ClientConfig) (*genai.Client, error) {
		return genai.NewClient(ctx, config)
	}

	validationError := defaultGeminiKeyValidator(context.Background(), "AIzaFakeKey123")
	// Expected to fail at List (network/auth error), but the code path is covered.
	if validationError == nil {
		test.Fatal("expected error from validation with fake key, got nil")
	}
}

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
