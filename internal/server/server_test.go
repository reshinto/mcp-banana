package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/reshinto/mcp-banana/internal/config"
	"github.com/reshinto/mcp-banana/internal/gemini"
	"github.com/reshinto/mcp-banana/internal/server"
)

// mockGeminiService is a test double for gemini.GeminiService.
type mockGeminiService struct{}

func (mockGeminiService) GenerateImage(_ context.Context, _ string, _ string, _ gemini.GenerateOptions) (*gemini.ImageResult, error) {
	return &gemini.ImageResult{ImageBase64: "abc", MIMEType: "image/png", ModelUsed: "nano"}, nil
}

func (mockGeminiService) EditImage(_ context.Context, _ string, _ []byte, _ string, _ string) (*gemini.ImageResult, error) {
	return &gemini.ImageResult{ImageBase64: "def", MIMEType: "image/jpeg", ModelUsed: "nano"}, nil
}

// defaultTestConfig returns a config suitable for most middleware tests.
func defaultTestConfig() *config.Config {
	return &config.Config{
		AuthToken:         "test-secret",
		RateLimit:         100,
		GlobalConcurrency: 8,
		MaxImageBytes:     4 * 1024 * 1024,
	}
}

// buildHandler wires the full stack (MCP server + middleware) using the provided config.
func buildHandler(cfg *config.Config) http.Handler {
	mcpSrv := server.NewMCPServer(mockGeminiService{}, cfg.MaxImageBytes)
	return server.NewHTTPHandler(mcpSrv, cfg, slog.Default())
}

// --- Middleware: auth ---

func TestMiddlewareCorrectBearerToken(test *testing.T) {
	handler := buildHandler(defaultTestConfig())

	req := httptest.NewRequest(http.MethodPost, "/mcp", jsonRPCBody("tools/list"))
	req.Header.Set("Authorization", "Bearer test-secret")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		test.Fatalf("expected non-401, got 401")
	}
}

func TestMiddlewareWrongBearerToken(test *testing.T) {
	handler := buildHandler(defaultTestConfig())

	req := httptest.NewRequest(http.MethodPost, "/mcp", jsonRPCBody("tools/list"))
	req.Header.Set("Authorization", "Bearer wrong-secret")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		test.Fatalf("expected 401, got %d", rec.Code)
	}
	assertJSONError(test, rec.Body.String(), "unauthorized")
}

func TestMiddlewareMissingBearerToken(test *testing.T) {
	handler := buildHandler(defaultTestConfig())

	req := httptest.NewRequest(http.MethodPost, "/mcp", jsonRPCBody("tools/list"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		test.Fatalf("expected 401, got %d", rec.Code)
	}
	assertJSONError(test, rec.Body.String(), "unauthorized")
}

// --- Middleware: rate limit ---

func TestMiddlewareRateLimitExhaustion(test *testing.T) {
	cfg := &config.Config{
		AuthToken:         "",
		RateLimit:         1, // burst of 1 token; second request is rejected
		GlobalConcurrency: 8,
		MaxImageBytes:     4 * 1024 * 1024,
	}
	handler := buildHandler(cfg)

	// First non-healthz request consumes the single burst token.
	firstReq := httptest.NewRequest(http.MethodPost, "/mcp", jsonRPCBody("tools/list"))
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, firstReq)

	// Second request should be rate limited.
	secondReq := httptest.NewRequest(http.MethodPost, "/mcp", jsonRPCBody("tools/list"))
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)

	if secondRec.Code != http.StatusTooManyRequests {
		test.Fatalf("expected 429, got %d", secondRec.Code)
	}
	assertJSONError(test, secondRec.Body.String(), "rate_limited")
	if secondRec.Header().Get("Retry-After") == "" {
		test.Fatal("expected Retry-After header to be set")
	}
}

// --- Middleware: health bypass ---

func TestHealthzBypassesAuthAndRateLimit(test *testing.T) {
	handler := buildHandler(defaultTestConfig())

	// No Authorization header — should still succeed for /healthz.
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		test.Fatalf("expected 200 for /healthz, got %d", rec.Code)
	}
}

// --- Middleware: panic recovery ---

func TestMiddlewarePanicRecovery(test *testing.T) {
	cfg := &config.Config{
		AuthToken:         "",
		RateLimit:         1000,
		GlobalConcurrency: 8,
		MaxImageBytes:     4 * 1024 * 1024,
	}

	panicHandler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("deliberate test panic")
	})

	wrappedHandler := server.WrapWithMiddleware(cfg, slog.Default(), panicHandler)

	req := httptest.NewRequest(http.MethodPost, "/anything", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		test.Fatalf("expected 500 after panic, got %d", rec.Code)
	}
	assertJSONError(test, rec.Body.String(), "server_error")
}

// --- Integration: tools/list via /mcp ---

func TestIntegrationToolsList(test *testing.T) {
	cfg := &config.Config{
		AuthToken:         "",
		RateLimit:         100,
		GlobalConcurrency: 8,
		MaxImageBytes:     4 * 1024 * 1024,
	}
	handler := buildHandler(cfg)

	body := jsonRPCBody("tools/list")
	req := httptest.NewRequest(http.MethodPost, "/mcp", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	responseBody := rec.Body.String()
	if strings.Contains(responseBody, `"error":"unauthorized"`) {
		test.Fatal("middleware incorrectly blocked tools/list")
	}
}

// --- Integration: oversized body ---

func TestOversizedBodyRejected(test *testing.T) {
	cfg := &config.Config{
		AuthToken:         "",
		RateLimit:         1000,
		GlobalConcurrency: 8,
		MaxImageBytes:     4 * 1024 * 1024,
	}
	handler := buildHandler(cfg)

	oversizedBody := bytes.Repeat([]byte("x"), 16*1024*1024) // 16 MB > 15 MB limit
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(oversizedBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		test.Fatalf("expected 413, got %d", rec.Code)
	}
	assertJSONError(test, rec.Body.String(), "request_too_large")
}

// --- helpers ---

// jsonRPCBody returns a reader with a minimal JSON-RPC 2.0 request for the given method.
func jsonRPCBody(method string) io.Reader {
	payload := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"%s","params":{}}`, method)
	return strings.NewReader(payload)
}

// assertJSONError verifies that responseBody contains the expected error key.
func assertJSONError(test *testing.T, responseBody string, expectedKey string) {
	test.Helper()
	var parsed map[string]string
	if decodeErr := json.Unmarshal([]byte(responseBody), &parsed); decodeErr != nil {
		test.Fatalf("response is not valid JSON: %s — body: %q", decodeErr, responseBody)
	}
	if parsed["error"] != expectedKey {
		test.Fatalf("expected error key %q, got %q — body: %q", expectedKey, parsed["error"], responseBody)
	}
}
