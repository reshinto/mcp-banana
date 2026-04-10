package gemini

import (
	"strings"
	"testing"
)

// TestValidateRegistryAtStartup_AllVerified tests the success path where all models
// have verified GeminiIDs (no sentinels). This requires modifying the package-level
// registry, so it must be in the internal test package.
func TestValidateRegistryAtStartup_AllVerified(test *testing.T) {
	original := registry
	defer func() { registry = original }()

	registry = map[string]ModelInfo{
		"test-model": {Alias: "test-model", GeminiID: "gemini-real-model-id"},
	}

	if registryError := ValidateRegistryAtStartup(); registryError != nil {
		test.Errorf("expected nil for verified registry, got: %v", registryError)
	}
}

// TestValidateRegistryAtStartup_SentinelError tests the error path where a model
// has the sentinel GeminiID value, which should cause validation to fail.
func TestValidateRegistryAtStartup_SentinelError(test *testing.T) {
	original := registry
	defer func() { registry = original }()

	registry = map[string]ModelInfo{
		"unverified-model": {Alias: "unverified-model", GeminiID: "VERIFY_MODEL_ID_BEFORE_RELEASE"},
	}

	registryError := ValidateRegistryAtStartup()
	if registryError == nil {
		test.Fatal("expected error for registry containing sentinel GeminiID, got nil")
	}
	if !strings.Contains(registryError.Error(), "unverified-model") {
		test.Errorf("expected error to mention the alias 'unverified-model', got: %v", registryError)
	}
	if !strings.Contains(registryError.Error(), "unverified GeminiID") {
		test.Errorf("expected error to mention 'unverified GeminiID', got: %v", registryError)
	}
}
