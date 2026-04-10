package gemini

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"google.golang.org/genai"
)

// allowedOutputMIMETypes is the set of MIME types accepted for generated images.
var allowedOutputMIMETypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/webp": true,
}

// contentGenerator abstracts the genai model generation call for testability.
type contentGenerator interface {
	GenerateContent(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error)
}

// Client wraps the genai SDK client and enforces concurrency limits for pro models.
type Client struct {
	generator    contentGenerator
	timeoutSecs  int
	proSemaphore chan struct{}
}

// genaiClientFactory creates a genai.Client. Overridden in tests to inject failures.
var genaiClientFactory = func(ctx context.Context, config *genai.ClientConfig) (*genai.Client, error) {
	return genai.NewClient(ctx, config)
}

// OverrideClientFactory replaces genaiClientFactory with the given function and returns a
// restore function that resets it to the original. Intended for use in tests outside this
// package that need to force client creation failures (e.g. to test cache error paths).
// Call the returned function via defer to restore the original factory.
func OverrideClientFactory(factory func(context.Context, *genai.ClientConfig) (*genai.Client, error)) func() {
	original := genaiClientFactory
	genaiClientFactory = factory
	return func() { genaiClientFactory = original }
}

// NewClient creates a new Gemini API client with the given configuration.
// proConcurrency sets the maximum number of concurrent requests for the pro model.
func NewClient(startupContext context.Context, apiKey string, timeoutSecs int, proConcurrency int) (*Client, error) {
	inner, clientError := genaiClientFactory(startupContext, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if clientError != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", clientError)
	}

	return &Client{
		generator:    inner.Models,
		timeoutSecs:  timeoutSecs,
		proSemaphore: make(chan struct{}, proConcurrency),
	}, nil
}

// GenerateImage creates a new image from a text prompt using the specified model alias.
func (client *Client) GenerateImage(requestContext context.Context, modelAlias string, prompt string, options GenerateOptions) (*ImageResult, error) {
	modelInfo, lookupError := LookupModel(modelAlias)
	if lookupError != nil {
		return nil, fmt.Errorf("%s: %s", ErrModelUnavail, "unknown model alias")
	}

	if modelInfo.Alias == "nano-banana-pro" {
		select {
		case client.proSemaphore <- struct{}{}:
			defer func() { <-client.proSemaphore }()
		case <-requestContext.Done():
			return nil, fmt.Errorf("%s: %s", ErrServerError, "request cancelled while waiting for pro model slot")
		}
	}

	modelName, contents, config := buildGenerateInputs(prompt, modelInfo.GeminiID, options.AspectRatio)

	timeoutContext, cancel := context.WithTimeout(requestContext, time.Duration(client.timeoutSecs)*time.Second)
	defer cancel()

	startTime := time.Now()
	resp, generateError := client.generator.GenerateContent(timeoutContext, modelName, contents, config)
	if generateError != nil {
		safeCode, safeMsg := MapError(generateError)
		return nil, fmt.Errorf("%s: %s", safeCode, safeMsg)
	}

	return extractImage(resp, modelAlias, startTime)
}

// EditImage modifies an existing image using text instructions and the specified model alias.
// imageData must be raw decoded bytes (not base64).
func (client *Client) EditImage(requestContext context.Context, modelAlias string, imageData []byte, mimeType string, instructions string) (*ImageResult, error) {
	modelInfo, lookupError := LookupModel(modelAlias)
	if lookupError != nil {
		return nil, fmt.Errorf("%s: %s", ErrModelUnavail, "unknown model alias")
	}

	if modelInfo.Alias == "nano-banana-pro" {
		select {
		case client.proSemaphore <- struct{}{}:
			defer func() { <-client.proSemaphore }()
		case <-requestContext.Done():
			return nil, fmt.Errorf("%s: %s", ErrServerError, "request cancelled while waiting for pro model slot")
		}
	}

	modelName, contents, config := buildEditInputs(instructions, modelInfo.GeminiID, imageData, mimeType)

	timeoutContext, cancel := context.WithTimeout(requestContext, time.Duration(client.timeoutSecs)*time.Second)
	defer cancel()

	startTime := time.Now()
	resp, generateError := client.generator.GenerateContent(timeoutContext, modelName, contents, config)
	if generateError != nil {
		safeCode, safeMsg := MapError(generateError)
		return nil, fmt.Errorf("%s: %s", safeCode, safeMsg)
	}

	return extractImage(resp, modelAlias, startTime)
}

// buildGenerateInputs constructs the model name, contents, and config for image generation.
// aspectRatio is optional; an empty string omits the aspect ratio config.
func buildGenerateInputs(prompt string, modelID string, aspectRatio string) (modelName string, contents []*genai.Content, config *genai.GenerateContentConfig) {
	modelName = modelID

	contents = []*genai.Content{
		genai.NewContentFromText(prompt, genai.RoleUser),
	}

	config = &genai.GenerateContentConfig{
		ResponseModalities: []string{"TEXT", "IMAGE"},
	}

	if aspectRatio != "" {
		// Aspect ratio is conveyed via system instruction as the SDK does not have
		// a dedicated aspect ratio field for GenerateContent (it does for GenerateImages).
		config.SystemInstruction = genai.NewContentFromText(
			fmt.Sprintf("Generate the image with aspect ratio: %s", aspectRatio),
			genai.RoleUser,
		)
	}

	return modelName, contents, config
}

// buildEditInputs constructs the model name, contents, and config for image editing.
// imageData must be raw decoded bytes (not base64).
func buildEditInputs(instructions string, modelID string, imageData []byte, mimeType string) (modelName string, contents []*genai.Content, config *genai.GenerateContentConfig) {
	modelName = modelID

	imagePart := &genai.Part{
		InlineData: &genai.Blob{
			Data:     imageData,
			MIMEType: mimeType,
		},
	}
	textPart := &genai.Part{
		Text: instructions,
	}

	contents = []*genai.Content{
		genai.NewContentFromParts([]*genai.Part{imagePart, textPart}, genai.RoleUser),
	}

	config = &genai.GenerateContentConfig{
		ResponseModalities: []string{"TEXT", "IMAGE"},
	}

	return modelName, contents, config
}

// extractImage pulls the first inline image part from a GenerateContentResponse.
// Returns the Nano Banana alias in ModelUsed, not the Gemini model ID.
func extractImage(resp *genai.GenerateContentResponse, modelAlias string, startTime time.Time) (*ImageResult, error) {
	if resp == nil {
		return nil, fmt.Errorf("%s: %s", ErrContentPolicy, "response blocked by content safety policy")
	}

	if len(resp.Candidates) == 0 {
		return nil, fmt.Errorf("%s: %s", ErrContentPolicy, "response blocked by content safety policy")
	}

	candidate := resp.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return nil, fmt.Errorf("%s: %s", ErrGenerationFail, "no content parts in response")
	}

	for _, part := range candidate.Content.Parts {
		if part.InlineData == nil {
			continue
		}

		outputMIME := part.InlineData.MIMEType
		if !allowedOutputMIMETypes[outputMIME] {
			return nil, fmt.Errorf("%s: %s", ErrGenerationFail, "unexpected output MIME type")
		}

		encoded := base64.StdEncoding.EncodeToString(part.InlineData.Data)
		elapsedMS := time.Since(startTime).Milliseconds()

		return &ImageResult{
			ImageBase64:    encoded,
			MIMEType:       outputMIME,
			ModelUsed:      modelAlias,
			GenerationTime: elapsedMS,
		}, nil
	}

	// All parts were text-only or contained no inline data.
	return nil, fmt.Errorf("%s: %s", ErrGenerationFail, "no image data in response")
}
