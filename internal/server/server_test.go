package server_test

import (
	"bytes"
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
	"github.com/reshinto/mcp-banana/internal/credentials"
	"github.com/reshinto/mcp-banana/internal/gemini"
	"github.com/reshinto/mcp-banana/internal/oauth"
	"github.com/reshinto/mcp-banana/internal/server"
)

// testClientCache returns a ClientCache for use in server tests.
func testClientCache() *gemini.ClientCache {
	return gemini.NewClientCache(30, 2)
}

// noAuthConfig returns a config with no auth configured (SSH tunnel mode).
func noAuthConfig() *config.Config {
	return &config.Config{
		RateLimit:         100,
		GlobalConcurrency: 8,
		MaxImageBytes:     4 * 1024 * 1024,
	}
}

// createCredentialsFile creates a temporary credentials JSON file with the given
// token-to-key mappings and returns the file path. The file is cleaned up when
// the test finishes.
func createCredentialsFile(test *testing.T, entries map[string]string) string {
	test.Helper()
	credFile, createError := os.CreateTemp("", "creds-*.json")
	if createError != nil {
		test.Fatalf("failed to create temp credentials file: %v", createError)
	}
	test.Cleanup(func() { _ = os.Remove(credFile.Name()) })

	data, _ := json.Marshal(entries)
	_, _ = credFile.Write(data)
	_ = credFile.Close()
	return credFile.Name()
}

// createCredStore creates a credentials.Store backed by a temporary file with
// the given token-to-key mappings.
func createCredStore(test *testing.T, entries map[string]string) *credentials.Store {
	test.Helper()
	filePath := createCredentialsFile(test, entries)
	store, storeError := credentials.NewStore(filePath)
	if storeError != nil {
		test.Fatalf("failed to create credentials store: %v", storeError)
	}
	return store
}

// buildHandler wires the full stack (MCP server + middleware) using the provided
// config and credentials store. Pass nil credStore for no-auth mode.
func buildHandler(cfg *config.Config, credStore *credentials.Store) http.Handler {
	mcpSrv := server.NewMCPServer(testClientCache(), cfg.MaxImageBytes)
	return server.NewHTTPHandler(mcpSrv, cfg, slog.Default(), nil, nil, credStore)
}

// --- Middleware: auth ---

func TestMiddlewareCorrectBearerToken(test *testing.T) {
	credStore := createCredStore(test, map[string]string{"test-secret": "AIzaTestKey"})
	handler := buildHandler(noAuthConfig(), credStore)

	req := httptest.NewRequest(http.MethodPost, "/mcp", jsonRPCBody("tools/list"))
	req.Header.Set("Authorization", "Bearer test-secret")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		test.Fatalf("expected non-401, got 401")
	}
}

func TestMiddlewareWrongBearerToken(test *testing.T) {
	credStore := createCredStore(test, map[string]string{"test-secret": "AIzaTestKey"})
	handler := buildHandler(noAuthConfig(), credStore)

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
	credStore := createCredStore(test, map[string]string{"test-secret": "AIzaTestKey"})
	handler := buildHandler(noAuthConfig(), credStore)

	req := httptest.NewRequest(http.MethodPost, "/mcp", jsonRPCBody("tools/list"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		test.Fatalf("expected 401, got %d", rec.Code)
	}
	assertJSONError(test, rec.Body.String(), "unauthorized")
}

func TestMiddlewareNoAuthConfigured_PassesThrough(test *testing.T) {
	// When no credStore and no oauthStore are set, auth is skipped (SSH tunnel mode)
	handler := buildHandler(noAuthConfig(), nil)

	req := httptest.NewRequest(http.MethodPost, "/mcp", jsonRPCBody("tools/list"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		test.Fatalf("expected non-401 when no auth configured, got 401")
	}
}

func TestMiddlewareCredentialsFileLookup(test *testing.T) {
	credStore := createCredStore(test, map[string]string{
		"alice-token-abc": "AIzaAliceKey",
		"bob-token-def":   "AIzaBobKey",
	})
	handler := buildHandler(noAuthConfig(), credStore)

	// Valid token from credentials file should pass
	validReq := httptest.NewRequest(http.MethodPost, "/mcp", jsonRPCBody("tools/list"))
	validReq.Header.Set("Authorization", "Bearer alice-token-abc")
	validRec := httptest.NewRecorder()
	handler.ServeHTTP(validRec, validReq)

	if validRec.Code == http.StatusUnauthorized {
		test.Fatalf("expected non-401 for valid credentials file token, got 401")
	}

	// Invalid token should be rejected
	invalidReq := httptest.NewRequest(http.MethodPost, "/mcp", jsonRPCBody("tools/list"))
	invalidReq.Header.Set("Authorization", "Bearer unknown-token")
	invalidRec := httptest.NewRecorder()
	handler.ServeHTTP(invalidRec, invalidReq)

	if invalidRec.Code != http.StatusUnauthorized {
		test.Fatalf("expected 401 for invalid token with credentials file, got %d", invalidRec.Code)
	}
}

// --- Middleware: Gemini key resolved into context ---

func TestMiddlewareGeminiKeyResolvedFromCredentials(test *testing.T) {
	credStore := createCredStore(test, map[string]string{"my-token": "AIzaResolvedKey"})

	var capturedKey string
	captureHandler := http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		capturedKey = gemini.APIKeyFromContext(request.Context())
	})
	wrappedHandler := server.WrapWithMiddleware(noAuthConfig(), slog.Default(), captureHandler, nil, credStore)

	req := httptest.NewRequest(http.MethodPost, "/anything", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer my-token")
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if capturedKey != "AIzaResolvedKey" {
		test.Errorf("expected resolved key in context, got %q", capturedKey)
	}
}

func TestMiddlewareNoGeminiKeyInSSHTunnelMode(test *testing.T) {
	// No credStore, no oauthStore — SSH tunnel mode, no key in context
	var capturedKey string
	captureHandler := http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		capturedKey = gemini.APIKeyFromContext(request.Context())
	})
	wrappedHandler := server.WrapWithMiddleware(noAuthConfig(), slog.Default(), captureHandler, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/anything", strings.NewReader("{}"))
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if capturedKey != "" {
		test.Errorf("expected empty key in context when no auth configured, got %q", capturedKey)
	}
}

// --- Middleware: rate limit ---

func TestMiddlewareRateLimitExhaustion(test *testing.T) {
	cfg := &config.Config{
		RateLimit:         1, // burst of 1 token; second request is rejected
		GlobalConcurrency: 8,
		MaxImageBytes:     4 * 1024 * 1024,
	}
	handler := buildHandler(cfg, nil)

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
	credStore := createCredStore(test, map[string]string{"test-secret": "AIzaTestKey"})
	handler := buildHandler(noAuthConfig(), credStore)

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
		RateLimit:         1000,
		GlobalConcurrency: 8,
		MaxImageBytes:     4 * 1024 * 1024,
	}

	panicHandler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("deliberate test panic")
	})

	wrappedHandler := server.WrapWithMiddleware(cfg, slog.Default(), panicHandler, nil, nil)

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
	handler := buildHandler(noAuthConfig(), nil)

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
	handler := buildHandler(noAuthConfig(), nil)

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
	credStore := createCredStore(test, map[string]string{"test-secret": "AIzaTestKey"})
	handler := buildHandler(noAuthConfig(), credStore)

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
		RateLimit:         1000,
		GlobalConcurrency: 1, // single slot
		MaxImageBytes:     4 * 1024 * 1024,
	}

	// Create a handler that blocks until released.
	blockChan := make(chan struct{})
	blockingHandler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-blockChan
	})
	wrappedHandler := server.WrapWithMiddleware(cfg, slog.Default(), blockingHandler, nil, nil)

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
		RateLimit:         1000,
		GlobalConcurrency: 8,
		MaxImageBytes:     4 * 1024 * 1024,
	}

	noopHandler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})
	wrappedHandler := server.WrapWithMiddleware(cfg, slog.Default(), noopHandler, nil, nil)

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
		RateLimit:         120, // 120 per minute = 2/sec, so 1/limit = 0.5 → retryAfterSecs = 0 → clamped to 1
		GlobalConcurrency: 8,
		MaxImageBytes:     4 * 1024 * 1024,
	}
	handler := buildHandler(cfg, nil)

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

// --- Middleware: OAuth access token with credentials ---

// buildHandlerWithOAuthAndCreds creates a handler with an oauth.Store and credentials store.
func buildHandlerWithOAuthAndCreds(cfg *config.Config, oauthStore *oauth.Store, credStore *credentials.Store) http.Handler {
	mcpSrv := server.NewMCPServer(testClientCache(), cfg.MaxImageBytes)
	return server.NewHTTPHandler(mcpSrv, cfg, slog.Default(), oauthStore, nil, credStore)
}

// TestMiddlewareOAuthTokenWithCredentials verifies that a valid OAuth access token
// with a provider identity is resolved to a Gemini key via the credentials file.
func TestMiddlewareOAuthTokenWithCredentials(test *testing.T) {
	oauthStore := oauth.NewStore()
	oauthStore.StoreAccessToken(&oauth.TokenData{
		Token:            "valid-oauth-token-xyz",
		ClientID:         "test-client",
		ProviderIdentity: "google:user@example.com",
		ExpiresAt:        time.Now().Add(1 * time.Hour),
	})

	credStore := createCredStore(test, map[string]string{
		"google:user@example.com": "AIzaOAuthUserKey",
	})

	cfg := noAuthConfig()
	handler := buildHandlerWithOAuthAndCreds(cfg, oauthStore, credStore)

	req := httptest.NewRequest(http.MethodPost, "/mcp", jsonRPCBody("tools/list"))
	req.Header.Set("Authorization", "Bearer valid-oauth-token-xyz")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		test.Fatalf("expected OAuth token to be accepted, got 401")
	}
}

// TestMiddlewareOAuthTokenRejectedWhenExpired verifies that an expired OAuth access token
// is rejected even when an oauth.Store is configured.
func TestMiddlewareOAuthTokenRejectedWhenExpired(test *testing.T) {
	oauthStore := oauth.NewStore()
	oauthStore.StoreAccessToken(&oauth.TokenData{
		Token:            "expired-oauth-token",
		ClientID:         "test-client",
		ProviderIdentity: "google:user@example.com",
		ExpiresAt:        time.Now().Add(-1 * time.Hour), // already expired
	})

	credStore := createCredStore(test, map[string]string{
		"google:user@example.com": "AIzaOAuthUserKey",
	})

	cfg := noAuthConfig()
	handler := buildHandlerWithOAuthAndCreds(cfg, oauthStore, credStore)

	req := httptest.NewRequest(http.MethodPost, "/mcp", jsonRPCBody("tools/list"))
	req.Header.Set("Authorization", "Bearer expired-oauth-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		test.Fatalf("expected 401 for expired OAuth token, got %d", rec.Code)
	}
}

// --- NewHTTPHandler: OAuth routes ---

// TestNewHTTPHandler_OAuthRoutesMounted verifies that when oauthStore and OAuthBaseURL
// are configured, the OAuth discovery endpoint is registered and reachable.
func TestNewHTTPHandler_OAuthRoutesMounted(test *testing.T) {
	cfg := &config.Config{
		RateLimit:         100,
		GlobalConcurrency: 8,
		MaxImageBytes:     4 * 1024 * 1024,
		OAuthBaseURL:      "https://example.com",
	}

	store := oauth.NewStore()
	providers := []oauth.ProviderConfig{
		oauth.NewGoogleProvider("client-id", "client-secret"),
	}

	mcpSrv := server.NewMCPServer(testClientCache(), cfg.MaxImageBytes)
	handler := server.NewHTTPHandler(mcpSrv, cfg, slog.Default(), store, providers, nil)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// The discovery endpoint should return a 200 with JSON metadata.
	if rec.Code != http.StatusOK {
		test.Fatalf("expected 200 from OAuth discovery endpoint, got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
		test.Errorf("expected JSON content type from discovery endpoint, got %q", rec.Header().Get("Content-Type"))
	}
}

// TestNewHTTPHandler_OAuthRoutesNotMounted verifies that when oauthStore is nil,
// the OAuth discovery endpoint is NOT registered.
func TestNewHTTPHandler_OAuthRoutesNotMounted(test *testing.T) {
	cfg := &config.Config{
		RateLimit:         100,
		GlobalConcurrency: 8,
		MaxImageBytes:     4 * 1024 * 1024,
		OAuthBaseURL:      "https://example.com",
	}

	mcpSrv := server.NewMCPServer(testClientCache(), cfg.MaxImageBytes)
	// Passing nil store — OAuth routes should NOT be mounted.
	handler := server.NewHTTPHandler(mcpSrv, cfg, slog.Default(), nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Without OAuth routes, this should return 404 (not found) or pass through to the
	// MCP handler which also won't recognise the path.
	if rec.Code == http.StatusOK {
		test.Fatalf("expected non-200 when OAuth routes not mounted, got %d", rec.Code)
	}
}

// TestNewHTTPHandler_OAuthRoutesNotMountedWhenNoBaseURL verifies that even when
// oauthStore is non-nil, OAuth routes are not mounted if OAuthBaseURL is empty.
func TestNewHTTPHandler_OAuthRoutesNotMountedWhenNoBaseURL(test *testing.T) {
	cfg := &config.Config{
		RateLimit:         100,
		GlobalConcurrency: 8,
		MaxImageBytes:     4 * 1024 * 1024,
		OAuthBaseURL:      "", // no base URL → routes must not be mounted
	}

	store := oauth.NewStore()
	mcpSrv := server.NewMCPServer(testClientCache(), cfg.MaxImageBytes)
	handler := server.NewHTTPHandler(mcpSrv, cfg, slog.Default(), store, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		test.Fatalf("expected non-200 when OAuthBaseURL is empty, got %d", rec.Code)
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
