package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/reshinto/mcp-banana/internal/gemini"
)

// mockGeminiService is a test double for gemini.GeminiService.
type mockGeminiService struct {
	generateResult *gemini.ImageResult
	generateError  error
	editResult     *gemini.ImageResult
	editError      error
}

// GenerateImage returns the mock's configured result or error.
func (mock *mockGeminiService) GenerateImage(requestContext context.Context, modelAlias string, prompt string, options gemini.GenerateOptions) (*gemini.ImageResult, error) {
	return mock.generateResult, mock.generateError
}

// EditImage returns the mock's configured result or error.
func (mock *mockGeminiService) EditImage(requestContext context.Context, modelAlias string, imageData []byte, mimeType string, instructions string) (*gemini.ImageResult, error) {
	return mock.editResult, mock.editError
}

// makeRequest builds a CallToolRequest with the given argument map.
func makeRequest(args map[string]any) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	return req
}

// validPNGBase64 returns a minimal valid PNG encoded as base64.
// The magic bytes are: 0x89 'P' 'N' 'G' followed by enough padding.
func validPNGBase64() string {
	// Minimal PNG header: 8 magic bytes + extra padding to exceed minImageBytes (12)
	header := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0x00, 0x00, 0x00, 0x0D, 0xFF, 0xFF}
	return base64.StdEncoding.EncodeToString(header)
}

// stubImageResult returns a minimal ImageResult for use in mock returns.
func stubImageResult() *gemini.ImageResult {
	return &gemini.ImageResult{
		ImageBase64:    "abc123",
		MIMEType:       "image/png",
		ModelUsed:      "nano-banana-2",
		GenerationTime: 100,
	}
}

// --- generate_image tests ---

func TestGenerateImageHandler_Success(test *testing.T) {
	svc := &mockGeminiService{generateResult: stubImageResult()}
	handler := NewGenerateImageHandler(svc, 10*1024*1024)

	req := makeRequest(map[string]any{"prompt": "a sunny beach"})
	result, err := handler(context.Background(), req)

	if err != nil {
		test.Fatalf("expected no Go error, got: %v", err)
	}
	if result.IsError {
		test.Fatalf("expected success result, got error: %v", result.Content)
	}

	var decoded gemini.ImageResult
	textContent := extractTextContent(test, result)
	if jsonErr := json.Unmarshal([]byte(textContent), &decoded); jsonErr != nil {
		test.Fatalf("response is not valid JSON: %v", jsonErr)
	}
	if decoded.ModelUsed != "nano-banana-2" {
		test.Errorf("expected model_used nano-banana-2, got %q", decoded.ModelUsed)
	}
	if decoded.MIMEType != "image/png" {
		test.Errorf("expected mime_type image/png, got %q", decoded.MIMEType)
	}
}

func TestGenerateImageHandler_EmptyPrompt(test *testing.T) {
	svc := &mockGeminiService{}
	handler := NewGenerateImageHandler(svc, 10*1024*1024)

	req := makeRequest(map[string]any{"prompt": ""})
	result, err := handler(context.Background(), req)

	if err != nil {
		test.Fatalf("expected no Go error, got: %v", err)
	}
	if !result.IsError {
		test.Fatal("expected error result for empty prompt")
	}
	textContent := extractTextContent(test, result)
	if !strings.Contains(textContent, "invalid_prompt") {
		test.Errorf("expected error to contain 'invalid_prompt', got: %q", textContent)
	}
}

func TestGenerateImageHandler_InvalidModel(test *testing.T) {
	svc := &mockGeminiService{}
	handler := NewGenerateImageHandler(svc, 10*1024*1024)

	req := makeRequest(map[string]any{"prompt": "hello", "model": "not-a-real-model"})
	result, err := handler(context.Background(), req)

	if err != nil {
		test.Fatalf("expected no Go error, got: %v", err)
	}
	if !result.IsError {
		test.Fatal("expected error result for invalid model")
	}
	textContent := extractTextContent(test, result)
	if !strings.Contains(textContent, "invalid_model") {
		test.Errorf("expected error to contain 'invalid_model', got: %q", textContent)
	}
}

func TestGenerateImageHandler_GeminiError(test *testing.T) {
	svc := &mockGeminiService{generateError: errors.New("quota exceeded somewhere")}
	handler := NewGenerateImageHandler(svc, 10*1024*1024)

	req := makeRequest(map[string]any{"prompt": "a portrait"})
	result, err := handler(context.Background(), req)

	if err != nil {
		test.Fatalf("expected no Go error, got: %v", err)
	}
	if !result.IsError {
		test.Fatal("expected error result for Gemini error")
	}
	textContent := extractTextContent(test, result)
	// Must NOT contain the raw error text
	if strings.Contains(textContent, "quota exceeded somewhere") {
		test.Errorf("raw error text must not appear in response, got: %q", textContent)
	}
	// Must contain a safe error code
	if !strings.Contains(textContent, "quota_exceeded") && !strings.Contains(textContent, "generation_failed") {
		test.Errorf("expected a safe error code, got: %q", textContent)
	}
}

func TestGenerateImageHandler_InvalidAspectRatio(test *testing.T) {
	svc := &mockGeminiService{}
	handler := NewGenerateImageHandler(svc, 10*1024*1024)

	req := makeRequest(map[string]any{"prompt": "a sunset", "aspect_ratio": "99:99"})
	result, err := handler(context.Background(), req)

	if err != nil {
		test.Fatalf("expected no Go error, got: %v", err)
	}
	if !result.IsError {
		test.Fatal("expected error result for invalid aspect ratio")
	}
	textContent := extractTextContent(test, result)
	if !strings.Contains(textContent, "invalid_aspect_ratio") {
		test.Errorf("expected error to contain 'invalid_aspect_ratio', got: %q", textContent)
	}
}

// --- list_models tests ---

func TestListModelsHandler_NoGeminiID(test *testing.T) {
	handler := NewListModelsHandler()
	req := makeRequest(nil)
	result, err := handler(context.Background(), req)

	if err != nil {
		test.Fatalf("expected no Go error, got: %v", err)
	}
	if result.IsError {
		test.Fatalf("expected success result, got error: %v", result.Content)
	}

	textContent := extractTextContent(test, result)

	// SECURITY: GeminiID must never appear in output
	if strings.Contains(textContent, "gemini_id") || strings.Contains(textContent, "GeminiID") {
		test.Errorf("SECURITY: response must not contain gemini_id or GeminiID, got: %q", textContent)
	}

	// Verify it is valid JSON array
	var models []map[string]any
	if jsonErr := json.Unmarshal([]byte(textContent), &models); jsonErr != nil {
		test.Fatalf("response is not valid JSON array: %v", jsonErr)
	}
	if len(models) == 0 {
		test.Error("expected at least one model in list")
	}
}

// --- recommend_model tests ---

func TestRecommendModelHandler_Success(test *testing.T) {
	handler := NewRecommendModelHandler()
	req := makeRequest(map[string]any{
		"task_description": "create a quick draft illustration",
		"priority":         "speed",
	})
	result, err := handler(context.Background(), req)

	if err != nil {
		test.Fatalf("expected no Go error, got: %v", err)
	}
	if result.IsError {
		test.Fatalf("expected success result, got error: %v", result.Content)
	}

	textContent := extractTextContent(test, result)
	var decoded map[string]any
	if jsonErr := json.Unmarshal([]byte(textContent), &decoded); jsonErr != nil {
		test.Fatalf("response is not valid JSON: %v", jsonErr)
	}
	if _, ok := decoded["recommended_model"]; !ok {
		test.Error("expected 'recommended_model' field in response")
	}
}

func TestRecommendModelHandler_InvalidPriority(test *testing.T) {
	handler := NewRecommendModelHandler()
	req := makeRequest(map[string]any{
		"task_description": "create an image",
		"priority":         "turbo-mode",
	})
	result, err := handler(context.Background(), req)

	if err != nil {
		test.Fatalf("expected no Go error, got: %v", err)
	}
	if !result.IsError {
		test.Fatal("expected error result for invalid priority")
	}
	textContent := extractTextContent(test, result)
	if !strings.Contains(textContent, "invalid_priority") {
		test.Errorf("expected 'invalid_priority' in error, got: %q", textContent)
	}
}

func TestRecommendModelHandler_EmptyTaskDescription(test *testing.T) {
	handler := NewRecommendModelHandler()
	req := makeRequest(map[string]any{"task_description": ""})
	result, err := handler(context.Background(), req)

	if err != nil {
		test.Fatalf("expected no Go error, got: %v", err)
	}
	if !result.IsError {
		test.Fatal("expected error result for empty task description")
	}
	textContent := extractTextContent(test, result)
	if !strings.Contains(textContent, "invalid_task_description") {
		test.Errorf("expected 'invalid_task_description' in error, got: %q", textContent)
	}
}

// --- edit_image tests ---

func TestEditImageHandler_Success(test *testing.T) {
	svc := &mockGeminiService{editResult: stubImageResult()}
	handler := NewEditImageHandler(svc, 10*1024*1024)

	req := makeRequest(map[string]any{
		"instructions": "make it brighter",
		"image":        validPNGBase64(),
		"mime_type":    "image/png",
	})
	result, err := handler(context.Background(), req)

	if err != nil {
		test.Fatalf("expected no Go error, got: %v", err)
	}
	if result.IsError {
		test.Fatalf("expected success result, got error: %v", result.Content)
	}

	textContent := extractTextContent(test, result)
	var decoded gemini.ImageResult
	if jsonErr := json.Unmarshal([]byte(textContent), &decoded); jsonErr != nil {
		test.Fatalf("response is not valid JSON: %v", jsonErr)
	}
	if decoded.ModelUsed != "nano-banana-2" {
		test.Errorf("expected model_used nano-banana-2, got %q", decoded.ModelUsed)
	}
}

func TestEditImageHandler_EmptyInstructions(test *testing.T) {
	svc := &mockGeminiService{}
	handler := NewEditImageHandler(svc, 10*1024*1024)

	req := makeRequest(map[string]any{
		"instructions": "",
		"image":        validPNGBase64(),
		"mime_type":    "image/png",
	})
	result, err := handler(context.Background(), req)

	if err != nil {
		test.Fatalf("expected no Go error, got: %v", err)
	}
	if !result.IsError {
		test.Fatal("expected error result for empty instructions")
	}
	textContent := extractTextContent(test, result)
	if !strings.Contains(textContent, "invalid_prompt") {
		test.Errorf("expected 'invalid_prompt' in error, got: %q", textContent)
	}
}

func TestEditImageHandler_InvalidModel(test *testing.T) {
	svc := &mockGeminiService{}
	handler := NewEditImageHandler(svc, 10*1024*1024)

	req := makeRequest(map[string]any{
		"instructions": "make it brighter",
		"model":        "not-a-real-model",
		"image":        validPNGBase64(),
		"mime_type":    "image/png",
	})
	result, err := handler(context.Background(), req)

	if err != nil {
		test.Fatalf("expected no Go error, got: %v", err)
	}
	if !result.IsError {
		test.Fatal("expected error result for invalid model")
	}
	textContent := extractTextContent(test, result)
	if !strings.Contains(textContent, "invalid_model") {
		test.Errorf("expected 'invalid_model' in error, got: %q", textContent)
	}
}

func TestEditImageHandler_GeminiError(test *testing.T) {
	svc := &mockGeminiService{editError: errors.New("internal failure")}
	handler := NewEditImageHandler(svc, 10*1024*1024)

	req := makeRequest(map[string]any{
		"instructions": "make it brighter",
		"image":        validPNGBase64(),
		"mime_type":    "image/png",
	})
	result, err := handler(context.Background(), req)

	if err != nil {
		test.Fatalf("expected no Go error, got: %v", err)
	}
	if !result.IsError {
		test.Fatal("expected error result for Gemini error")
	}
	textContent := extractTextContent(test, result)
	if strings.Contains(textContent, "internal failure") {
		test.Errorf("raw error text must not appear in response, got: %q", textContent)
	}
}

func TestEditImageHandler_InvalidImage(test *testing.T) {
	svc := &mockGeminiService{}
	handler := NewEditImageHandler(svc, 10*1024*1024)

	req := makeRequest(map[string]any{
		"instructions": "make it darker",
		"image":        base64.StdEncoding.EncodeToString([]byte("not-an-image")),
		"mime_type":    "image/png",
	})
	result, err := handler(context.Background(), req)

	if err != nil {
		test.Fatalf("expected no Go error, got: %v", err)
	}
	if !result.IsError {
		test.Fatal("expected error result for invalid image data")
	}
	textContent := extractTextContent(test, result)
	if !strings.Contains(textContent, "invalid_image") {
		test.Errorf("expected 'invalid_image' in error, got: %q", textContent)
	}
}

// extractTextContent pulls the text content string from a CallToolResult.
func extractTextContent(test *testing.T, result *mcp.CallToolResult) string {
	test.Helper()
	if len(result.Content) == 0 {
		test.Fatal("result has no content")
	}
	textContent, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		test.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	return textContent.Text
}
