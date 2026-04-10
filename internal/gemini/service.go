// Package gemini provides the Gemini API client and model registry.
//
// GeminiService defines the interface for image generation operations,
// enabling dependency injection and mock testing without a real API key.
package gemini

import "context"

// GeminiService defines the interface for image generation operations.
// This interface enables dependency injection and mock testing without
// requiring a real Gemini API key or network access.
//
// The *Client type satisfies this interface. Tests can use a mock
// implementation to verify handler behavior in isolation.
type GeminiService interface {
	// GenerateImage creates a new image from a text prompt.
	// modelAlias is a Nano Banana alias (not a Gemini model ID).
	GenerateImage(requestContext context.Context, modelAlias string, prompt string, options GenerateOptions) (*ImageResult, error)

	// EditImage modifies an existing image using text instructions.
	// imageData is the raw decoded bytes (not base64).
	EditImage(requestContext context.Context, modelAlias string, imageData []byte, mimeType string, instructions string) (*ImageResult, error)
}

// GenerateOptions holds optional parameters for image generation.
type GenerateOptions struct {
	AspectRatio string
}

// ImageResult is the safe output of image generation/editing.
// Contains only data safe to return to Claude Code.
type ImageResult struct {
	ImageBase64    string `json:"image_base64"`
	MIMEType       string `json:"mime_type"`
	ModelUsed      string `json:"model_used"` // Nano Banana alias, NOT Gemini ID
	GenerationTime int64  `json:"generation_time_ms"`
}
