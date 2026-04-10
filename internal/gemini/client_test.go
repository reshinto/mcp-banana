package gemini

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"google.golang.org/genai"
)

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

// TestProSemaphoreCancellation verifies that context cancellation releases the pro slot wait.
func TestProSemaphoreCancellation(test *testing.T) {
	// Create a client with a semaphore of size 1 to easily fill it.
	client := &Client{
		inner:        nil, // not used in this test
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
