// Package gemini provides the Gemini API client and model registry for
// mcp-banana. It maps Nano Banana model aliases to internal Gemini model
// identifiers and wraps all Gemini API interactions.
//
// SECURITY: Gemini model IDs (GeminiID) are internal-only and must NEVER
// appear in any MCP tool response, log entry, or error message returned
// to Claude Code. Only the Nano Banana aliases are safe to expose.
package gemini

import (
	"fmt"
	"sort"
)

// ModelInfo describes a Nano Banana model and its mapping to a Gemini model ID.
// This type is INTERNAL -- use SafeModelInfo for data returned to Claude Code.
//
// SECURITY: The GeminiID field must never be included in any response to
// Claude Code. It is used only for internal Gemini API calls.
type ModelInfo struct {
	Alias          string
	GeminiID       string // INTERNAL ONLY -- never expose to Claude Code
	Description    string
	Capabilities   []string
	TypicalLatency string
	BestFor        string
}

// SafeModelInfo contains only the fields safe to expose to Claude Code.
// It deliberately excludes GeminiID to prevent accidental leakage of
// internal Gemini model identifiers.
type SafeModelInfo struct {
	Alias          string   `json:"id"`
	Description    string   `json:"description"`
	Capabilities   []string `json:"capabilities"`
	TypicalLatency string   `json:"typical_latency"`
	BestFor        string   `json:"best_for"`
}

// registry is the single source of truth for model alias-to-ID mapping.
//
// IMPORTANT: The GeminiID values below were verified against the official
// Gemini API documentation (https://ai.google.dev/gemini-api/docs/models).
// The sentinel value `VERIFY_MODEL_ID_BEFORE_RELEASE` will cause a startup
// failure (see ValidateRegistryAtStartup) to prevent accidental deployment
// with unverified IDs.
var registry = map[string]ModelInfo{
	"nano-banana-2": {
		Alias:          "nano-banana-2",
		GeminiID:       "gemini-3.1-flash-image-preview",
		Description:    "Fast, high-volume image generation. Under 10 seconds.",
		Capabilities:   []string{"generate", "edit"},
		TypicalLatency: "5-10s",
		BestFor:        "Iterative work, drafts, batch generation",
	},
	"nano-banana-pro": {
		Alias:          "nano-banana-pro",
		GeminiID:       "gemini-3-pro-image-preview",
		Description:    "Professional quality with advanced reasoning. 15-45 seconds.",
		Capabilities:   []string{"generate", "edit"},
		TypicalLatency: "15-45s",
		BestFor:        "Final assets, photorealistic images, complex scenes",
	},
	"nano-banana-original": {
		Alias:          "nano-banana-original",
		GeminiID:       "gemini-2.5-flash-image",
		Description:    "Speed and efficiency optimized. 3-8 seconds.",
		Capabilities:   []string{"generate", "edit"},
		TypicalLatency: "3-8s",
		BestFor:        "Quick previews, high-volume batch work",
	},
}

// LookupModel returns the full ModelInfo (including GeminiID) for internal use.
// Returns an error if the alias is not in the allowlist.
// SECURITY: The returned ModelInfo contains GeminiID -- do not expose to clients.
func LookupModel(alias string) (ModelInfo, error) {
	model, exists := registry[alias]
	if !exists {
		return ModelInfo{}, fmt.Errorf("unknown model alias: %q", alias)
	}
	return model, nil
}

// AllModelsSafe returns all registered models as SafeModelInfo (no GeminiID).
// The output is sorted by alias for deterministic ordering.
// Use this for any data that will be returned to Claude Code.
func AllModelsSafe() []SafeModelInfo {
	models := make([]SafeModelInfo, 0, len(registry))
	for _, model := range registry {
		models = append(models, SafeModelInfo{
			Alias:          model.Alias,
			Description:    model.Description,
			Capabilities:   model.Capabilities,
			TypicalLatency: model.TypicalLatency,
			BestFor:        model.BestFor,
		})
	}
	sort.Slice(models, func(first, second int) bool {
		return models[first].Alias < models[second].Alias
	})
	return models
}

// ValidAliases returns the list of valid model alias strings.
func ValidAliases() []string {
	aliases := make([]string, 0, len(registry))
	for alias := range registry {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}

// ValidateRegistryAtStartup checks that no model alias still has a sentinel
// GeminiID. This prevents accidental deployment with unverified model IDs.
// Call this during server startup before accepting any requests.
func ValidateRegistryAtStartup() error {
	for alias, model := range registry {
		if model.GeminiID == "VERIFY_MODEL_ID_BEFORE_RELEASE" {
			return fmt.Errorf("model %q has unverified GeminiID -- verify at https://ai.google.dev/gemini-api/docs/models before release", alias)
		}
	}
	return nil
}
