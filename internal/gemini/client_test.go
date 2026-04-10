package gemini

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/genai"
)

// mockContentGenerator is a test double for the contentGenerator interface.
type mockContentGenerator struct {
	response *genai.GenerateContentResponse
	error    error
}

func (mock *mockContentGenerator) GenerateContent(_ context.Context, _ string, _ []*genai.Content, _ *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	return mock.response, mock.error
}

// newTestClient creates a Client with a mock generator for testing.
func newTestClient(mock *mockContentGenerator) *Client {
	return &Client{
		generator:    mock,
		timeoutSecs:  30,
		proSemaphore: make(chan struct{}, 2),
	}
}

// validImageResponse returns a GenerateContentResponse with a valid PNG image part.
func validImageResponse() *genai.GenerateContentResponse {
	return &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{
							InlineData: &genai.Blob{
								Data:     []byte("fake-png-data"),
								MIMEType: "image/png",
							},
						},
					},
				},
			},
		},
	}
}

// TestBuildGenerateInputs verifies model name, content parts, and config construction.
func TestBuildGenerateInputs(test *testing.T) {
	test.Run("basic prompt without aspect ratio", func(test *testing.T) {
		modelName, contents, config := buildGenerateInputs("a cat on a mountain", "gemini-test-model", "")

		if modelName != "gemini-test-model" {
			test.Errorf("expected model name %q, got %q", "gemini-test-model", modelName)
		}

		if len(contents) != 1 {
			test.Fatalf("expected 1 content, got %d", len(contents))
		}

		if len(contents[0].Parts) != 1 {
			test.Fatalf("expected 1 part, got %d", len(contents[0].Parts))
		}

		if contents[0].Parts[0].Text != "a cat on a mountain" {
			test.Errorf("expected prompt text in part, got %q", contents[0].Parts[0].Text)
		}

		if config == nil {
			test.Fatal("expected non-nil config")
		}

		if config.SystemInstruction != nil {
			test.Error("expected no system instruction when aspect ratio is empty")
		}
	})

	test.Run("prompt with aspect ratio sets system instruction", func(test *testing.T) {
		_, _, config := buildGenerateInputs("a sunset", "gemini-test-model", "16:9")

		if config == nil {
			test.Fatal("expected non-nil config")
		}

		if config.SystemInstruction == nil {
			test.Fatal("expected system instruction for aspect ratio")
		}

		if len(config.SystemInstruction.Parts) == 0 {
			test.Fatal("expected non-empty system instruction parts")
		}

		instructionText := config.SystemInstruction.Parts[0].Text
		if !strings.Contains(instructionText, "16:9") {
			test.Errorf("expected aspect ratio in system instruction, got %q", instructionText)
		}
	})
}

// TestBuildEditInputs verifies model name, contents with both image and text parts.
func TestBuildEditInputs(test *testing.T) {
	imageData := []byte{0x89, 0x50, 0x4E, 0x47} // PNG magic bytes
	mimeType := "image/png"
	instructions := "make the sky more blue"
	modelID := "gemini-edit-model"

	modelName, contents, config := buildEditInputs(instructions, modelID, imageData, mimeType)

	if modelName != modelID {
		test.Errorf("expected model name %q, got %q", modelID, modelName)
	}

	if len(contents) != 1 {
		test.Fatalf("expected 1 content, got %d", len(contents))
	}

	parts := contents[0].Parts
	if len(parts) != 2 {
		test.Fatalf("expected 2 parts (image + text), got %d", len(parts))
	}

	// Find image and text parts (order may vary).
	var foundImage, foundText bool
	for _, part := range parts {
		if part.InlineData != nil {
			foundImage = true
			if part.InlineData.MIMEType != mimeType {
				test.Errorf("expected MIME type %q, got %q", mimeType, part.InlineData.MIMEType)
			}
			if string(part.InlineData.Data) != string(imageData) {
				test.Error("image data mismatch")
			}
		}
		if part.Text == instructions {
			foundText = true
		}
	}

	if !foundImage {
		test.Error("expected an image part with inline data")
	}
	if !foundText {
		test.Error("expected a text part with instructions")
	}

	if config == nil {
		test.Fatal("expected non-nil config")
	}
}

// TestExtractImage verifies image extraction from various response shapes.
func TestExtractImage(test *testing.T) {
	startTime := time.Now()

	test.Run("nil response returns content policy error", func(test *testing.T) {
		_, err := extractImage(nil, "nano-banana-2", startTime)
		if err == nil {
			test.Fatal("expected error for nil response")
		}
		if !strings.Contains(err.Error(), ErrContentPolicy) {
			test.Errorf("expected content policy error, got %q", err.Error())
		}
	})

	test.Run("empty candidates returns content policy error", func(test *testing.T) {
		resp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{},
		}
		_, err := extractImage(resp, "nano-banana-2", startTime)
		if err == nil {
			test.Fatal("expected error for empty candidates")
		}
		if !strings.Contains(err.Error(), ErrContentPolicy) {
			test.Errorf("expected content policy error, got %q", err.Error())
		}
	})

	test.Run("text-only response returns generation failed error", func(test *testing.T) {
		resp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{
					Content: &genai.Content{
						Parts: []*genai.Part{
							{Text: "Here is a description instead of an image."},
						},
					},
				},
			},
		}
		_, err := extractImage(resp, "nano-banana-2", startTime)
		if err == nil {
			test.Fatal("expected error for text-only response")
		}
		if !strings.Contains(err.Error(), ErrGenerationFail) {
			test.Errorf("expected generation_failed error, got %q", err.Error())
		}
	})

	test.Run("nil content returns generation failed error", func(test *testing.T) {
		resp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{Content: nil},
			},
		}
		_, err := extractImage(resp, "nano-banana-2", startTime)
		if err == nil {
			test.Fatal("expected error for nil content")
		}
		if !strings.Contains(err.Error(), ErrGenerationFail) {
			test.Errorf("expected generation_failed error, got %q", err.Error())
		}
	})

	test.Run("valid image response returns ImageResult with alias", func(test *testing.T) {
		rawImageData := []byte("fake-png-data")
		resp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{
					Content: &genai.Content{
						Parts: []*genai.Part{
							{
								InlineData: &genai.Blob{
									Data:     rawImageData,
									MIMEType: "image/png",
								},
							},
						},
					},
				},
			},
		}

		result, err := extractImage(resp, "nano-banana-2", startTime)
		if err != nil {
			test.Fatalf("unexpected error: %v", err)
		}

		if result.MIMEType != "image/png" {
			test.Errorf("expected MIME type image/png, got %q", result.MIMEType)
		}

		if result.ModelUsed != "nano-banana-2" {
			test.Errorf("expected ModelUsed to be alias %q, got %q", "nano-banana-2", result.ModelUsed)
		}

		expectedBase64 := base64.StdEncoding.EncodeToString(rawImageData)
		if result.ImageBase64 != expectedBase64 {
			test.Errorf("base64 mismatch: expected %q, got %q", expectedBase64, result.ImageBase64)
		}

		if result.GenerationTime < 0 {
			test.Errorf("expected non-negative generation time, got %d", result.GenerationTime)
		}
	})

	test.Run("disallowed MIME type returns generation failed error", func(test *testing.T) {
		resp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{
					Content: &genai.Content{
						Parts: []*genai.Part{
							{
								InlineData: &genai.Blob{
									Data:     []byte("gif-data"),
									MIMEType: "image/gif",
								},
							},
						},
					},
				},
			},
		}

		_, err := extractImage(resp, "nano-banana-2", startTime)
		if err == nil {
			test.Fatal("expected error for disallowed MIME type")
		}
		if !strings.Contains(err.Error(), ErrGenerationFail) {
			test.Errorf("expected generation_failed error, got %q", err.Error())
		}
	})

	test.Run("jpeg and webp MIME types are accepted", func(test *testing.T) {
		for _, mimeType := range []string{"image/jpeg", "image/webp"} {
			resp := &genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Parts: []*genai.Part{
								{
									InlineData: &genai.Blob{
										Data:     []byte("image-data"),
										MIMEType: mimeType,
									},
								},
							},
						},
					},
				},
			}

			result, err := extractImage(resp, "nano-banana-2", startTime)
			if err != nil {
				test.Errorf("expected no error for MIME type %q, got: %v", mimeType, err)
				continue
			}
			if result.MIMEType != mimeType {
				test.Errorf("expected MIME type %q, got %q", mimeType, result.MIMEType)
			}
		}
	})
}

// --- NewClient tests ---

func TestNewClient_Success(test *testing.T) {
	client, clientError := NewClient(context.Background(), "fake-test-key", 60, 3)
	if clientError != nil {
		test.Fatalf("unexpected error: %v", clientError)
	}
	if client == nil {
		test.Fatal("expected non-nil client")
	}
	if client.generator == nil {
		test.Error("expected non-nil generator")
	}
	if client.timeoutSecs != 60 {
		test.Errorf("expected timeout 60, got %d", client.timeoutSecs)
	}
}

func TestNewClient_FactoryError(test *testing.T) {
	original := genaiClientFactory
	defer func() { genaiClientFactory = original }()

	genaiClientFactory = func(_ context.Context, _ *genai.ClientConfig) (*genai.Client, error) {
		return nil, errors.New("simulated SDK init failure")
	}

	_, clientError := NewClient(context.Background(), "fake-key", 30, 2)
	if clientError == nil {
		test.Fatal("expected error when factory fails")
	}
	if !strings.Contains(clientError.Error(), "failed to create genai client") {
		test.Errorf("expected wrapped error message, got: %v", clientError)
	}
}

// --- GenerateImage tests ---

func TestGenerateImage_Success(test *testing.T) {
	mock := &mockContentGenerator{response: validImageResponse()}
	client := newTestClient(mock)

	// Use a valid alias from the registry — save/restore for this test.
	originalRegistry := registry
	defer func() { registry = originalRegistry }()
	registry = map[string]ModelInfo{
		"test-model": {Alias: "test-model", GeminiID: "gemini-test"},
	}

	result, generateError := client.GenerateImage(context.Background(), "test-model", "a sunset", GenerateOptions{})
	if generateError != nil {
		test.Fatalf("unexpected error: %v", generateError)
	}
	if result.ModelUsed != "test-model" {
		test.Errorf("expected model_used 'test-model', got %q", result.ModelUsed)
	}
	if result.MIMEType != "image/png" {
		test.Errorf("expected mime_type 'image/png', got %q", result.MIMEType)
	}
}

func TestGenerateImage_WithAspectRatio(test *testing.T) {
	mock := &mockContentGenerator{response: validImageResponse()}
	client := newTestClient(mock)

	originalRegistry := registry
	defer func() { registry = originalRegistry }()
	registry = map[string]ModelInfo{
		"test-model": {Alias: "test-model", GeminiID: "gemini-test"},
	}

	result, generateError := client.GenerateImage(context.Background(), "test-model", "a sunset", GenerateOptions{AspectRatio: "16:9"})
	if generateError != nil {
		test.Fatalf("unexpected error: %v", generateError)
	}
	if result.MIMEType != "image/png" {
		test.Errorf("expected mime_type 'image/png', got %q", result.MIMEType)
	}
}

func TestGenerateImage_UnknownModel(test *testing.T) {
	mock := &mockContentGenerator{}
	client := newTestClient(mock)

	_, generateError := client.GenerateImage(context.Background(), "nonexistent-model", "a cat", GenerateOptions{})
	if generateError == nil {
		test.Fatal("expected error for unknown model")
	}
	if !strings.Contains(generateError.Error(), ErrModelUnavail) {
		test.Errorf("expected model_unavailable error, got: %v", generateError)
	}
}

func TestGenerateImage_APIError(test *testing.T) {
	mock := &mockContentGenerator{error: errors.New("internal API failure")}
	client := newTestClient(mock)

	originalRegistry := registry
	defer func() { registry = originalRegistry }()
	registry = map[string]ModelInfo{
		"test-model": {Alias: "test-model", GeminiID: "gemini-test"},
	}

	_, generateError := client.GenerateImage(context.Background(), "test-model", "a portrait", GenerateOptions{})
	if generateError == nil {
		test.Fatal("expected error when API fails")
	}
	// Must contain a safe error code, not the raw error text
	if strings.Contains(generateError.Error(), "internal API failure") {
		test.Error("SECURITY: raw error text leaked into response")
	}
}

func TestGenerateImage_ProModelAcquiresSlot(test *testing.T) {
	mock := &mockContentGenerator{response: validImageResponse()}
	client := newTestClient(mock)

	originalRegistry := registry
	defer func() { registry = originalRegistry }()
	registry = map[string]ModelInfo{
		"nano-banana-pro": {Alias: "nano-banana-pro", GeminiID: "gemini-pro"},
	}

	result, generateError := client.GenerateImage(context.Background(), "nano-banana-pro", "a portrait", GenerateOptions{})
	if generateError != nil {
		test.Fatalf("unexpected error: %v", generateError)
	}
	if result.ModelUsed != "nano-banana-pro" {
		test.Errorf("expected model_used 'nano-banana-pro', got %q", result.ModelUsed)
	}
}

func TestGenerateImage_ProModelContextCancelled(test *testing.T) {
	mock := &mockContentGenerator{}
	// Create a client with semaphore of size 1 and fill it.
	client := &Client{
		generator:    mock,
		timeoutSecs:  30,
		proSemaphore: make(chan struct{}, 1),
	}
	client.proSemaphore <- struct{}{} // fill the slot

	originalRegistry := registry
	defer func() { registry = originalRegistry }()
	registry = map[string]ModelInfo{
		"nano-banana-pro": {Alias: "nano-banana-pro", GeminiID: "gemini-pro"},
	}

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, generateError := client.GenerateImage(cancelCtx, "nano-banana-pro", "a portrait", GenerateOptions{})
	if generateError == nil {
		test.Fatal("expected error when context is cancelled")
	}
	if !strings.Contains(generateError.Error(), ErrServerError) {
		test.Errorf("expected server_error, got: %v", generateError)
	}
}

// --- EditImage tests ---

func TestEditImage_Success(test *testing.T) {
	mock := &mockContentGenerator{response: validImageResponse()}
	client := newTestClient(mock)

	originalRegistry := registry
	defer func() { registry = originalRegistry }()
	registry = map[string]ModelInfo{
		"test-model": {Alias: "test-model", GeminiID: "gemini-test"},
	}

	result, editError := client.EditImage(context.Background(), "test-model", []byte("image-data"), "image/png", "make it brighter")
	if editError != nil {
		test.Fatalf("unexpected error: %v", editError)
	}
	if result.ModelUsed != "test-model" {
		test.Errorf("expected model_used 'test-model', got %q", result.ModelUsed)
	}
}

func TestEditImage_UnknownModel(test *testing.T) {
	mock := &mockContentGenerator{}
	client := newTestClient(mock)

	_, editError := client.EditImage(context.Background(), "nonexistent-model", []byte("data"), "image/png", "edit it")
	if editError == nil {
		test.Fatal("expected error for unknown model")
	}
	if !strings.Contains(editError.Error(), ErrModelUnavail) {
		test.Errorf("expected model_unavailable error, got: %v", editError)
	}
}

func TestEditImage_APIError(test *testing.T) {
	mock := &mockContentGenerator{error: errors.New("safety blocked")}
	client := newTestClient(mock)

	originalRegistry := registry
	defer func() { registry = originalRegistry }()
	registry = map[string]ModelInfo{
		"test-model": {Alias: "test-model", GeminiID: "gemini-test"},
	}

	_, editError := client.EditImage(context.Background(), "test-model", []byte("data"), "image/png", "edit it")
	if editError == nil {
		test.Fatal("expected error when API fails")
	}
}

func TestEditImage_ProModelAcquiresSlot(test *testing.T) {
	mock := &mockContentGenerator{response: validImageResponse()}
	client := newTestClient(mock)

	originalRegistry := registry
	defer func() { registry = originalRegistry }()
	registry = map[string]ModelInfo{
		"nano-banana-pro": {Alias: "nano-banana-pro", GeminiID: "gemini-pro"},
	}

	result, editError := client.EditImage(context.Background(), "nano-banana-pro", []byte("data"), "image/png", "edit it")
	if editError != nil {
		test.Fatalf("unexpected error: %v", editError)
	}
	if result.ModelUsed != "nano-banana-pro" {
		test.Errorf("expected model_used 'nano-banana-pro', got %q", result.ModelUsed)
	}
}

func TestEditImage_ProModelContextCancelled(test *testing.T) {
	mock := &mockContentGenerator{}
	client := &Client{
		generator:    mock,
		timeoutSecs:  30,
		proSemaphore: make(chan struct{}, 1),
	}
	client.proSemaphore <- struct{}{} // fill the slot

	originalRegistry := registry
	defer func() { registry = originalRegistry }()
	registry = map[string]ModelInfo{
		"nano-banana-pro": {Alias: "nano-banana-pro", GeminiID: "gemini-pro"},
	}

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()

	_, editError := client.EditImage(cancelCtx, "nano-banana-pro", []byte("data"), "image/png", "edit it")
	if editError == nil {
		test.Fatal("expected error when context is cancelled")
	}
	if !strings.Contains(editError.Error(), ErrServerError) {
		test.Errorf("expected server_error, got: %v", editError)
	}
}

// TestOverrideClientFactory verifies that OverrideClientFactory swaps the factory and
// that the returned restore function resets it to the original.
func TestOverrideClientFactory(test *testing.T) {
	originalFactory := genaiClientFactory

	called := false
	restore := OverrideClientFactory(func(_ context.Context, _ *genai.ClientConfig) (*genai.Client, error) {
		called = true
		return nil, errors.New("overridden factory error")
	})

	_, clientError := NewClient(context.Background(), "test-key", 30, 2)
	if !called {
		test.Error("expected overridden factory to be called")
	}
	if clientError == nil {
		test.Error("expected error from overridden factory")
	}

	restore()

	// After restore, genaiClientFactory should point to the original.
	// We can't compare function pointers directly, but we can verify the
	// factory is no longer the override by checking it's not the one we set.
	// The best check is that a new call uses the real factory (which may fail
	// for a different reason, but it should not set called=true again).
	called = false
	_, _ = NewClient(context.Background(), "test-key", 30, 2)
	if called {
		test.Error("expected original factory to be restored, but override was still called")
	}

	_ = originalFactory // suppress unused warning if any
}

// TestProSemaphoreCancellation verifies that context cancellation releases the pro slot wait.
func TestProSemaphoreCancellation(test *testing.T) {
	// Create a client with a semaphore of size 1 to easily fill it.
	client := &Client{
		generator:    nil, // not used in this test
		timeoutSecs:  30,
		proSemaphore: make(chan struct{}, 1),
	}

	// Fill the semaphore so the next acquire blocks.
	client.proSemaphore <- struct{}{}

	cancelCtx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		// Simulate the pro-semaphore acquire logic from GenerateImage.
		select {
		case client.proSemaphore <- struct{}{}:
			defer func() { <-client.proSemaphore }()
			done <- nil
		case <-cancelCtx.Done():
			done <- cancelCtx.Err()
		}
	}()

	// Cancel the context to unblock the goroutine.
	cancel()

	select {
	case err := <-done:
		if err == nil {
			test.Error("expected context cancellation error, got nil")
		}
		if err != context.Canceled {
			test.Errorf("expected context.Canceled, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		test.Error("timed out waiting for semaphore cancellation")
	}
}
