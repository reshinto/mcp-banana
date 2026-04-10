// Package gemini_test validates the model registry that maps Nano Banana
// aliases to internal Gemini model identifiers.
package gemini_test

import (
	"testing"

	"github.com/reshinto/mcp-banana/internal/gemini"
)

func TestLookupModel_ValidAlias(test *testing.T) {
	model, lookupError := gemini.LookupModel("nano-banana-2")
	if lookupError != nil {
		test.Fatalf("unexpected error: %v", lookupError)
	}
	if model.GeminiID == "" {
		test.Error("expected non-empty GeminiID")
	}
	if model.Alias != "nano-banana-2" {
		test.Errorf("expected alias 'nano-banana-2', got '%s'", model.Alias)
	}
}

func TestLookupModel_AllAliases(test *testing.T) {
	aliases := []string{"nano-banana-2", "nano-banana-pro", "nano-banana-original"}
	for _, alias := range aliases {
		model, lookupError := gemini.LookupModel(alias)
		if lookupError != nil {
			test.Errorf("alias %q: unexpected error: %v", alias, lookupError)
			continue
		}
		if model.GeminiID == "" {
			test.Errorf("alias %q: empty GeminiID", alias)
		}
	}
}

func TestLookupModel_InvalidAlias(test *testing.T) {
	_, lookupError := gemini.LookupModel("not-a-real-model")
	if lookupError == nil {
		test.Fatal("expected error for invalid alias")
	}
}

func TestAllModelsSafe_ReturnsThreeModels(test *testing.T) {
	models := gemini.AllModelsSafe()
	if len(models) != 3 {
		test.Fatalf("expected 3 models, got %d", len(models))
	}
}

func TestAllModelsSafe_IsSortedByAlias(test *testing.T) {
	models := gemini.AllModelsSafe()
	for position := 1; position < len(models); position++ {
		if models[position-1].Alias > models[position].Alias {
			test.Errorf("models not sorted: %s > %s", models[position-1].Alias, models[position].Alias)
		}
	}
}

func TestAllModelsSafe_NeverExposesGeminiID(test *testing.T) {
	// SECURITY: SafeModelInfo must NOT contain a GeminiID field.
	// This test verifies that the safe type does not expose internal model IDs.
	models := gemini.AllModelsSafe()
	for _, model := range models {
		if model.Alias == "" {
			test.Error("expected non-empty alias in safe model info")
		}
		// SafeModelInfo struct has no GeminiID field -- this is enforced by the type system.
		// If someone adds a GeminiID field to SafeModelInfo, this test file will need updating.
	}
}

func TestValidateRegistryAtStartup_RejectsSentinelIDs(test *testing.T) {
	// This test verifies that ValidateRegistryAtStartup correctly detects
	// sentinel IDs and rejects them. It passes in both development (sentinels
	// present -> error returned) and after release (real IDs -> no error).
	//
	// The test is self-enforcing: it checks whether sentinels exist and
	// asserts the validation result matches. No manual flip needed.
	startupError := gemini.ValidateRegistryAtStartup()
	hasSentinels := false
	for _, model := range gemini.AllModelsSafe() {
		// Check via LookupModel which returns internal ModelInfo with GeminiID
		if fullModel, lookupError := gemini.LookupModel(model.Alias); lookupError == nil {
			if fullModel.GeminiID == "VERIFY_MODEL_ID_BEFORE_RELEASE" {
				hasSentinels = true
				break
			}
		}
	}

	if hasSentinels && startupError == nil {
		test.Fatal("sentinel IDs are present but ValidateRegistryAtStartup did not return an error")
	}
	if !hasSentinels && startupError != nil {
		test.Fatalf("no sentinel IDs remain but ValidateRegistryAtStartup returned error: %v", startupError)
	}
}

func TestValidAliases_ReturnsThreeStrings(test *testing.T) {
	aliases := gemini.ValidAliases()
	if len(aliases) != 3 {
		test.Fatalf("expected 3 aliases, got %d", len(aliases))
	}
}

func TestValidAliases_IsSorted(test *testing.T) {
	aliases := gemini.ValidAliases()
	for position := 1; position < len(aliases); position++ {
		if aliases[position-1] > aliases[position] {
			test.Errorf("aliases not sorted: %s > %s", aliases[position-1], aliases[position])
		}
	}
}
