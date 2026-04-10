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
	"os"
	"strings"
	"testing"
	"time"

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

func TestMiddlewareNoAuthConfigured_PassesThrough(test *testing.T) {
	// When no AuthToken and no AuthTokensFile are set, auth is skipped (SSH tunnel mode)
	noAuthConfig := &config.Config{
		AuthToken:         "",
		AuthTokensFile:    "",
		RateLimit:         100,
		GlobalConcurrency: 8,
		MaxImageBytes:     4 * 1024 * 1024,
	}
	handler := buildHandler(noAuthConfig)

	req := httptest.NewRequest(http.MethodPost, "/mcp", jsonRPCBody("tools/list"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		test.Fatalf("expected non-401 when no auth configured, got 401")
	}
}

func TestMiddlewareTokensFile(test *testing.T) {
	// Create a temp tokens file with two tokens
	tokensFile, createError := os.CreateTemp("", "tokens-*.txt")
	if createError != nil {
		test.Fatalf("failed to create temp file: %v", createError)
	}
	test.Cleanup(func() { _ = os.Remove(tokensFile.Name()) })

	_, _ = tokensFile.WriteString("# This is a comment\nalice-token-abc\nbob-token-def\n")
	_ = tokensFile.Close()

	fileAuthConfig := &config.Config{
		AuthToken:         "",
		AuthTokensFile:    tokensFile.Name(),
		RateLimit:         100,
		GlobalConcurrency: 8,
		MaxImageBytes:     4 * 1024 * 1024,
	}
	handler := buildHandler(fileAuthConfig)

	// Valid token from file should pass
	validReq := httptest.NewRequest(http.MethodPost, "/mcp", jsonRPCBody("tools/list"))
	validReq.Header.Set("Authorization", "Bearer alice-token-abc")
	validRec := httptest.NewRecorder()
	handler.ServeHTTP(validRec, validReq)

	if validRec.Code == http.StatusUnauthorized {
		test.Fatalf("expected non-401 for valid file token, got 401")
	}

	// Invalid token should be rejected
	invalidReq := httptest.NewRequest(http.MethodPost, "/mcp", jsonRPCBody("tools/list"))
	invalidReq.Header.Set("Authorization", "Bearer unknown-token")
	invalidRec := httptest.NewRecorder()
	handler.ServeHTTP(invalidRec, invalidReq)

	if invalidRec.Code != http.StatusUnauthorized {
		test.Fatalf("expected 401 for invalid token with file auth, got %d", invalidRec.Code)
	}
}

func TestMiddlewareTokensFileHotReload(test *testing.T) {
	// Create a temp tokens file with one token
	tokensFile, createError := os.CreateTemp("", "tokens-*.txt")
	if createError != nil {
		test.Fatalf("failed to create temp file: %v", createError)
	}
	test.Cleanup(func() { _ = os.Remove(tokensFile.Name()) })

	_, _ = tokensFile.WriteString("initial-token\n")
	_ = tokensFile.Close()

	fileAuthConfig := &config.Config{
		AuthTokensFile:    tokensFile.Name(),
		RateLimit:         100,
		GlobalConcurrency: 8,
		MaxImageBytes:     4 * 1024 * 1024,
	}
	handler := buildHandler(fileAuthConfig)

	// New token should fail initially
	newReq := httptest.NewRequest(http.MethodPost, "/mcp", jsonRPCBody("tools/list"))
	newReq.Header.Set("Authorization", "Bearer new-token")
	newRec := httptest.NewRecorder()
	handler.ServeHTTP(newRec, newReq)

	if newRec.Code != http.StatusUnauthorized {
		test.Fatalf("expected 401 for token not yet in file, got %d", newRec.Code)
	}

	// Update the file with the new token (hot reload, no restart)
	_ = os.WriteFile(tokensFile.Name(), []byte("initial-token\nnew-token\n"), 0644)

	// Now the new token should pass
	retryReq := httptest.NewRequest(http.MethodPost, "/mcp", jsonRPCBody("tools/list"))
	retryReq.Header.Set("Authorization", "Bearer new-token")
	retryRec := httptest.NewRecorder()
	handler.ServeHTTP(retryRec, retryReq)

	if retryRec.Code == http.StatusUnauthorized {
		test.Fatalf("expected non-401 after hot-reload, got 401")
	}
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

// --- Middleware: empty bearer token ---

func TestMiddlewareEmptyBearerToken(test *testing.T) {
	handler := buildHandler(defaultTestConfig())

	req := httptest.NewRequest(http.MethodPost, "/mcp", jsonRPCBody("tools/list"))
	req.Header.Set("Authorization", "Bearer ")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		test.Fatalf("expected 401 for empty bearer token, got %d", rec.Code)
	}
	assertJSONError(test, rec.Body.String(), "unauthorized")
}

// --- Middleware: concurrency timeout ---

func TestMiddlewareConcurrencyTimeout(test *testing.T) {
	cfg := &config.Config{
		AuthToken:         "",
		RateLimit:         1000,
		GlobalConcurrency: 1, // single slot
		MaxImageBytes:     4 * 1024 * 1024,
	}

	// Create a handler that blocks until released.
	blockChan := make(chan struct{})
	blockingHandler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-blockChan
	})
	wrappedHandler := server.WrapWithMiddleware(cfg, slog.Default(), blockingHandler)

	// First request takes the only concurrency slot and blocks.
	go func() {
		firstReq := httptest.NewRequest(http.MethodPost, "/anything", strings.NewReader("{}"))
		firstRec := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(firstRec, firstReq)
	}()

	// Give the goroutine time to acquire the semaphore slot.
	time.Sleep(50 * time.Millisecond)

	// Second request should time out waiting for the semaphore.
	secondReq := httptest.NewRequest(http.MethodPost, "/anything", strings.NewReader("{}"))
	secondRec := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(secondRec, secondReq)

	close(blockChan) // release the blocking handler

	if secondRec.Code != http.StatusServiceUnavailable {
		test.Fatalf("expected 503 for concurrency timeout, got %d", secondRec.Code)
	}
	assertJSONError(test, secondRec.Body.String(), "server_busy")
}

// --- Middleware: non-MaxBytesError read failure ---

// errorReader is an io.ReadCloser that always returns an error on Read.
type errorReader struct{}

func (errorReader) Read(_ []byte) (int, error) {
	return 0, fmt.Errorf("simulated I/O error")
}

func (errorReader) Close() error {
	return nil
}

func TestMiddlewareNonMaxBytesReadError(test *testing.T) {
	cfg := &config.Config{
		AuthToken:         "",
		RateLimit:         1000,
		GlobalConcurrency: 8,
		MaxImageBytes:     4 * 1024 * 1024,
	}

	noopHandler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})
	wrappedHandler := server.WrapWithMiddleware(cfg, slog.Default(), noopHandler)

	req := httptest.NewRequest(http.MethodPost, "/anything", errorReader{})
	rec := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		test.Fatalf("expected 400 for read error, got %d", rec.Code)
	}
	assertJSONError(test, rec.Body.String(), "bad_request")
}

// --- Middleware: Retry-After clamped to 1 ---

func TestMiddlewareRetryAfterClampedToOne(test *testing.T) {
	cfg := &config.Config{
		AuthToken:         "",
		RateLimit:         120, // 120 per minute = 2/sec, so 1/limit = 0.5 → retryAfterSecs = 0 → clamped to 1
		GlobalConcurrency: 8,
		MaxImageBytes:     4 * 1024 * 1024,
	}
	handler := buildHandler(cfg)

	// Exhaust the burst (120 requests) then trigger rate limiting
	for requestIndex := 0; requestIndex < 120; requestIndex++ {
		exhaustReq := httptest.NewRequest(http.MethodPost, "/mcp", jsonRPCBody("tools/list"))
		exhaustRec := httptest.NewRecorder()
		handler.ServeHTTP(exhaustRec, exhaustReq)
	}

	// Next request should be rate limited with Retry-After: 1
	limitedReq := httptest.NewRequest(http.MethodPost, "/mcp", jsonRPCBody("tools/list"))
	limitedRec := httptest.NewRecorder()
	handler.ServeHTTP(limitedRec, limitedReq)

	if limitedRec.Code != http.StatusTooManyRequests {
		test.Fatalf("expected 429, got %d", limitedRec.Code)
	}
	retryAfter := limitedRec.Header().Get("Retry-After")
	if retryAfter != "1" {
		test.Errorf("expected Retry-After '1', got %q", retryAfter)
	}
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
