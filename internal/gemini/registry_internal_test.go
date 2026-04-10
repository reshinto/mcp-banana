package gemini

import "testing"

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
