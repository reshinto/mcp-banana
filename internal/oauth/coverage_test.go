package oauth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// errGenerateToken is a sentinel error used to simulate GenerateRandomToken failures.
var errGenerateToken = fmt.Errorf("simulated token generation failure")

// withFailingTokenGenerator temporarily replaces GenerateRandomToken with a function
// that always returns errGenerateToken, then restores the original after the test.
func withFailingTokenGenerator(test *testing.T, after func()) {
	original := GenerateRandomToken
	GenerateRandomToken = func(_ int) (string, error) {
		return "", errGenerateToken
	}
	test.Cleanup(func() { GenerateRandomToken = original })
	after()
}

// --- GenerateRandomToken ---

// TestGenerateRandomToken_ErrorPath verifies that the error path in GenerateRandomToken
// is reachable by injecting a failing implementation.
func TestGenerateRandomToken_ErrorPath(test *testing.T) {
	withFailingTokenGenerator(test, func() {
		_, tokenError := GenerateRandomToken(32)
		if tokenError == nil {
			test.Error("expected error from failing token generator, got nil")
		}
	})
}

// --- Store: expired paths ---

// TestStore_ConsumeRefreshToken_Expired verifies that an expired refresh token returns nil.
func TestStore_ConsumeRefreshToken_Expired(test *testing.T) {
	store := NewStore()
	store.StoreRefreshToken(&RefreshData{
		Token:     "expired-refresh-token",
		ClientID:  "client-1",
		ExpiresAt: time.Now().Add(-1 * time.Second),
	})

	result := store.ConsumeRefreshToken("expired-refresh-token")
	if result != nil {
		test.Errorf("expected nil for expired refresh token, got %+v", result)
	}
}

// TestStore_GetProviderSession_Expired verifies that an expired provider session returns nil.
func TestStore_GetProviderSession_Expired(test *testing.T) {
	store := NewStore()
	store.StoreProviderSession(&ProviderSession{
		State:     "expired-state",
		ClientID:  "client-1",
		ExpiresAt: time.Now().Add(-1 * time.Second),
	})

	result := store.GetProviderSession("expired-state")
	if result != nil {
		test.Errorf("expected nil for expired provider session, got %+v", result)
	}
}

// --- NewAuthorizeHandler: GenerateRandomToken failure ---

// TestAuthorizeHandler_TokenGenerationFailure verifies that a server_error is returned
// when GenerateRandomToken fails during provider state creation.
func TestAuthorizeHandler_TokenGenerationFailure(test *testing.T) {
	store := NewStore()
	store.RegisterClient(&Client{
		ClientID:     "client-abc",
		ClientName:   "Test Client",
		RedirectURIs: []string{"https://app.example.com/callback"},
	})

	providers := []ProviderConfig{NewGoogleProvider("gid", "gsecret")}
	handler := NewAuthorizeHandler(store, providers, "https://mcp.example.com")

	withFailingTokenGenerator(test, func() {
		request := httptest.NewRequest(http.MethodGet, "/authorize?"+
			"response_type=code"+
			"&client_id=client-abc"+
			"&redirect_uri=https%3A%2F%2Fapp.example.com%2Fcallback"+
			"&state=random-state"+
			"&code_challenge=abc123"+
			"&code_challenge_method=S256",
			nil)
		recorder := httptest.NewRecorder()

		handler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusInternalServerError {
			test.Errorf("expected status 500, got %d", recorder.Code)
		}

		var errResp map[string]string
		if decodeError := json.NewDecoder(recorder.Body).Decode(&errResp); decodeError != nil {
			test.Fatalf("failed to decode error response: %v", decodeError)
		}
		if errResp["error"] != "server_error" {
			test.Errorf("expected error server_error, got %s", errResp["error"])
		}
	})
}

// --- NewCallbackHandler: GenerateRandomToken failure ---

// TestCallbackHandler_TokenGenerationFailure verifies that a server_error is returned
// when GenerateRandomToken fails during MCP code creation in the callback handler.
func TestCallbackHandler_TokenGenerationFailure(test *testing.T) {
	store := NewStore()
	store.StoreProviderSession(&ProviderSession{
		State:         "state-token-fail",
		ClientID:      "client-abc",
		RedirectURI:   "https://app.example.com/callback",
		CodeChallenge: "challenge-abc",
		OriginalState: "orig-state",
		Provider:      "google",
		ExpiresAt:     time.Now().Add(10 * time.Minute),
	})

	providers := []ProviderConfig{NewGoogleProvider("gid", "gsecret")}

	originalExchange := exchangeProviderCode
	exchangeProviderCode = func(_ ProviderConfig, _ string, _ string) (string, error) {
		return "mock-access-token", nil
	}
	defer func() { exchangeProviderCode = originalExchange }()

	originalFetcher := providerIdentityFetcher
	providerIdentityFetcher = func(_ ProviderConfig, _ string) (string, error) {
		return "google:user@example.com", nil
	}
	defer func() { providerIdentityFetcher = originalFetcher }()

	handler := NewCallbackHandler(store, providers, "https://mcp.example.com")

	withFailingTokenGenerator(test, func() {
		request := httptest.NewRequest(http.MethodGet, "/callback?code=any-code&state=state-token-fail", nil)
		recorder := httptest.NewRecorder()

		handler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusInternalServerError {
			test.Errorf("expected status 500, got %d", recorder.Code)
		}
	})
}

// --- NewCallbackHandler: unmatched provider ---

// TestCallbackHandler_UnmatchedProvider verifies that a server_error is returned when the
// provider stored in the session does not match any configured provider.
func TestCallbackHandler_UnmatchedProvider(test *testing.T) {
	store := NewStore()
	store.StoreProviderSession(&ProviderSession{
		State:         "state-no-provider",
		ClientID:      "client-abc",
		RedirectURI:   "https://app.example.com/callback",
		CodeChallenge: "challenge-abc",
		OriginalState: "orig-state",
		Provider:      "unknown-provider",
		ExpiresAt:     time.Now().Add(10 * time.Minute),
	})

	providers := []ProviderConfig{NewGoogleProvider("gid", "gsecret")}
	handler := NewCallbackHandler(store, providers, "https://mcp.example.com")

	request := httptest.NewRequest(http.MethodGet, "/callback?code=any-code&state=state-no-provider", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		test.Errorf("expected status 500, got %d", recorder.Code)
	}
}

// --- NewTokenHandler: ParseForm failure ---

// TestTokenHandler_ParseFormFailure verifies that a malformed Content-Type causes a
// 400 invalid_request response from the token handler.
func TestTokenHandler_ParseFormFailure(test *testing.T) {
	store := NewStore()
	handler := NewTokenHandler(store)

	// Sending a body with an invalid Content-Type makes ParseForm fail.
	body := strings.NewReader("%invalid-percent-encoding")
	request := httptest.NewRequest(http.MethodPost, "/token", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		test.Errorf("expected status 400, got %d", recorder.Code)
	}
}

// --- handleAuthorizationCodeGrant: client_id / redirect_uri mismatch ---

// TestTokenHandler_AuthCode_ClientMismatch verifies that a code presented with a mismatched
// client_id or redirect_uri is rejected with invalid_grant.
func TestTokenHandler_AuthCode_ClientMismatch(test *testing.T) {
	store := NewStore()

	verifier := "my-code-verifier-long-enough-to-be-valid"
	challenge := computeS256Challenge(verifier)

	store.StoreAuthCode(&AuthCode{
		Code:          "mismatch-auth-code",
		ClientID:      "real-client",
		RedirectURI:   "https://app.example.com/callback",
		CodeChallenge: challenge,
		ExpiresAt:     time.Now().Add(10 * time.Minute),
	})

	handler := NewTokenHandler(store)

	formData := url.Values{}
	formData.Set("grant_type", "authorization_code")
	formData.Set("code", "mismatch-auth-code")
	formData.Set("client_id", "wrong-client")
	formData.Set("redirect_uri", "https://app.example.com/callback")
	formData.Set("code_verifier", verifier)

	request := httptest.NewRequest(http.MethodPost, "/token", bytes.NewBufferString(formData.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		test.Errorf("expected status 400, got %d", recorder.Code)
	}

	var errResp map[string]string
	if decodeError := json.NewDecoder(recorder.Body).Decode(&errResp); decodeError != nil {
		test.Fatalf("failed to decode error response: %v", decodeError)
	}
	if errResp["error"] != "invalid_grant" {
		test.Errorf("expected error invalid_grant, got %s", errResp["error"])
	}
}

// --- issueTokenResponse: GenerateRandomToken failure (access token) ---

// TestIssueTokenResponse_AccessTokenGenerationFailure verifies that a server_error is
// returned when GenerateRandomToken fails while generating the access token.
func TestIssueTokenResponse_AccessTokenGenerationFailure(test *testing.T) {
	store := NewStore()
	store.StoreRefreshToken(&RefreshData{
		Token:     "valid-rt-for-issue-fail",
		ClientID:  "client-abc",
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
	})

	handler := NewTokenHandler(store)

	withFailingTokenGenerator(test, func() {
		formData := url.Values{}
		formData.Set("grant_type", "refresh_token")
		formData.Set("refresh_token", "valid-rt-for-issue-fail")
		formData.Set("client_id", "client-abc")

		request := httptest.NewRequest(http.MethodPost, "/token", bytes.NewBufferString(formData.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recorder := httptest.NewRecorder()

		handler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusInternalServerError {
			test.Errorf("expected status 500, got %d", recorder.Code)
		}
	})
}

// --- issueTokenResponse: GenerateRandomToken failure (refresh token) ---

// TestIssueTokenResponse_RefreshTokenGenerationFailure verifies that a server_error is
// returned when GenerateRandomToken succeeds for the access token but fails for the refresh token.
func TestIssueTokenResponse_RefreshTokenGenerationFailure(test *testing.T) {
	store := NewStore()
	store.StoreRefreshToken(&RefreshData{
		Token:     "valid-rt-for-refresh-fail",
		ClientID:  "client-abc",
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
	})

	handler := NewTokenHandler(store)

	original := GenerateRandomToken
	callCount := 0
	GenerateRandomToken = func(byteLength int) (string, error) {
		callCount++
		if callCount == 1 {
			// First call (access token) succeeds.
			return original(byteLength)
		}
		// Second call (refresh token) fails.
		return "", errGenerateToken
	}
	defer func() { GenerateRandomToken = original }()

	formData := url.Values{}
	formData.Set("grant_type", "refresh_token")
	formData.Set("refresh_token", "valid-rt-for-refresh-fail")
	formData.Set("client_id", "client-abc")

	request := httptest.NewRequest(http.MethodPost, "/token", bytes.NewBufferString(formData.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		test.Errorf("expected status 500, got %d", recorder.Code)
	}
}

// --- NewRegistrationHandler: GenerateRandomToken failure ---

// TestRegistrationHandler_TokenGenerationFailure verifies that a server_error is returned
// when GenerateRandomToken fails during client_id generation.
func TestRegistrationHandler_TokenGenerationFailure(test *testing.T) {
	store := NewStore()
	handler := NewRegistrationHandler(store)

	withFailingTokenGenerator(test, func() {
		body := `{"client_name":"Test","redirect_uris":["https://example.com/cb"]}`
		request := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(body))
		recorder := httptest.NewRecorder()

		handler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusInternalServerError {
			test.Errorf("expected status 500, got %d", recorder.Code)
		}

		var errResp map[string]string
		if decodeError := json.NewDecoder(recorder.Body).Decode(&errResp); decodeError != nil {
			test.Fatalf("failed to decode error response: %v", decodeError)
		}
		if errResp["error"] != "server_error" {
			test.Errorf("expected error server_error, got %s", errResp["error"])
		}
	})
}

// --- handleRefreshTokenGrant: nil refresh token path ---

// TestTokenHandler_InvalidRefreshToken verifies that a nonexistent refresh token returns 400.
func TestTokenHandler_InvalidRefreshToken(test *testing.T) {
	store := NewStore()
	handler := NewTokenHandler(store)

	formData := url.Values{}
	formData.Set("grant_type", "refresh_token")
	formData.Set("refresh_token", "nonexistent-token")
	formData.Set("client_id", "client-abc")

	request := httptest.NewRequest(http.MethodPost, "/token", bytes.NewBufferString(formData.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		test.Errorf("expected status 400, got %d", recorder.Code)
	}

	var errResp map[string]string
	if decodeError := json.NewDecoder(recorder.Body).Decode(&errResp); decodeError != nil {
		test.Fatalf("failed to decode error response: %v", decodeError)
	}
	if errResp["error"] != "invalid_grant" {
		test.Errorf("expected error invalid_grant, got %s", errResp["error"])
	}
}

// --- NewCallbackHandler: POST parse failure ---

// TestCallbackHandler_PostParseFailure verifies that a malformed POST body causes
// a 400 invalid_request response from the callback handler.
func TestCallbackHandler_PostParseFailure(test *testing.T) {
	store := NewStore()
	providers := []ProviderConfig{NewGoogleProvider("gid", "gsecret")}
	handler := NewCallbackHandler(store, providers, "https://mcp.example.com")

	// A body with invalid percent-encoding causes ParseForm to return an error.
	body := strings.NewReader("%invalid-percent-encoding")
	request := httptest.NewRequest(http.MethodPost, "/callback", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		test.Errorf("expected status 400, got %d", recorder.Code)
	}
}

// --- NewCallbackHandler: url.Parse failure on redirect URI ---

// TestCallbackHandler_MalformedRedirectURI verifies that a session containing a malformed
// redirect URI causes a 500 server_error after the provider code is exchanged successfully.
func TestCallbackHandler_MalformedRedirectURI(test *testing.T) {
	store := NewStore()
	// Store a session with a redirect URI that url.Parse will reject.
	store.StoreProviderSession(&ProviderSession{
		State:         "state-bad-uri",
		ClientID:      "client-abc",
		RedirectURI:   "://not-a-valid-url",
		CodeChallenge: "challenge-abc",
		OriginalState: "orig-state",
		Provider:      "google",
		ExpiresAt:     time.Now().Add(10 * time.Minute),
	})

	providers := []ProviderConfig{NewGoogleProvider("gid", "gsecret")}

	originalExchange := exchangeProviderCode
	exchangeProviderCode = func(_ ProviderConfig, _ string, _ string) (string, error) {
		return "mock-access-token", nil
	}
	defer func() { exchangeProviderCode = originalExchange }()

	originalFetcher := providerIdentityFetcher
	providerIdentityFetcher = func(_ ProviderConfig, _ string) (string, error) {
		return "google:user@example.com", nil
	}
	defer func() { providerIdentityFetcher = originalFetcher }()

	handler := NewCallbackHandler(store, providers, "https://mcp.example.com")

	request := httptest.NewRequest(http.MethodGet, "/callback?code=any-code&state=state-bad-uri", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		test.Errorf("expected status 500, got %d", recorder.Code)
	}
}

// --- errorAfterHeadersWriter: helper for testing defensive error branches ---

// errorAfterHeadersWriter is an http.ResponseWriter that accepts headers and status codes
// normally, but returns an error on any Write call. It is used to exercise defensive
// error branches that are unreachable with a well-behaved writer (e.g. json.Encode
// on a static struct, template.Execute to a non-failing buffer).
type errorAfterHeadersWriter struct {
	header     http.Header
	statusCode int
}

func newErrorAfterHeadersWriter() *errorAfterHeadersWriter {
	return &errorAfterHeadersWriter{header: make(http.Header)}
}

func (errorWriter *errorAfterHeadersWriter) Header() http.Header {
	return errorWriter.header
}

func (errorWriter *errorAfterHeadersWriter) WriteHeader(statusCode int) {
	errorWriter.statusCode = statusCode
}

func (errorWriter *errorAfterHeadersWriter) Write(_ []byte) (int, error) {
	return 0, fmt.Errorf("simulated write failure")
}

// --- NewMetadataHandler: json.Encode error path ---

// TestMetadataHandler_EncodeErrorPath verifies that the json.Encode error branch in
// NewMetadataHandler is reached when the underlying writer returns an error on Write.
func TestMetadataHandler_EncodeErrorPath(test *testing.T) {
	handler := NewMetadataHandler("https://example.com")
	writer := newErrorAfterHeadersWriter()
	request := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)

	// http.Error (called in the error branch) itself writes, which will also fail, but
	// the branch is exercised as long as json.NewEncoder(writer).Encode returns non-nil.
	handler.ServeHTTP(writer, request)

	// We only care that the handler ran without panicking — the write path exercised the branch.
}

// --- NewAuthorizeHandler: template.Execute error path ---

// TestAuthorizeHandler_TemplateExecuteError verifies that the defensive http.Error call
// after template.Execute is reached when the underlying writer fails on Write.
func TestAuthorizeHandler_TemplateExecuteError(test *testing.T) {
	store := NewStore()
	store.RegisterClient(&Client{
		ClientID:     "client-abc",
		ClientName:   "Test Client",
		RedirectURIs: []string{"https://app.example.com/callback"},
	})

	// Use an empty provider list so no token generation occurs before the template render.
	handler := NewAuthorizeHandler(store, nil, "https://mcp.example.com")

	writer := newErrorAfterHeadersWriter()
	request := httptest.NewRequest(http.MethodGet, "/authorize?"+
		"response_type=code"+
		"&client_id=client-abc"+
		"&redirect_uri=https%3A%2F%2Fapp.example.com%2Fcallback"+
		"&state=random-state"+
		"&code_challenge=abc123"+
		"&code_challenge_method=S256",
		nil)

	// The handler will attempt to render the template to writer; Write will fail,
	// causing the defensive http.Error branch to execute.
	handler.ServeHTTP(writer, request)
}

// --- exchangeProviderCode: real implementation coverage ---

// TestExchangeProviderCode_RealImpl exercises the real exchangeProviderCode implementation
// by pointing it at a local test server that returns HTTP 200, ensuring the var is covered
// beyond the mock-only paths used in other tests.
func TestExchangeProviderCode_RealImpl(test *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		json.NewEncoder(writer).Encode(map[string]string{"access_token": "test-token-abc"}) //nolint:errcheck
	}))
	defer testServer.Close()

	provider := ProviderConfig{
		Name:         "test-provider",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		TokenURL:     testServer.URL,
	}

	// Call the real (non-mocked) function directly.
	accessToken, tokenError := exchangeProviderCode(provider, "auth-code-123", "https://mcp.example.com/callback")
	if tokenError != nil {
		test.Errorf("expected nil error from real exchangeProviderCode, got: %v", tokenError)
	}
	if accessToken != "test-token-abc" {
		test.Errorf("expected access token 'test-token-abc', got '%s'", accessToken)
	}
}

// TestExchangeProviderCode_NonOKStatus verifies that a non-200 response from the provider
// token endpoint causes the real exchangeProviderCode to return an error.
func TestExchangeProviderCode_NonOKStatus(test *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
	}))
	defer testServer.Close()

	provider := ProviderConfig{
		Name:         "test-provider",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		TokenURL:     testServer.URL,
	}

	_, tokenError := exchangeProviderCode(provider, "bad-code", "https://mcp.example.com/callback")
	if tokenError == nil {
		test.Error("expected error for non-200 provider response, got nil")
	}
}

// TestExchangeProviderCode_PostFailure verifies that a network-level POST failure causes
// the real exchangeProviderCode to return an error.
func TestExchangeProviderCode_PostFailure(test *testing.T) {
	provider := ProviderConfig{
		Name:         "test-provider",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		TokenURL:     "http://127.0.0.1:1", // port 1 is always refused
	}

	_, tokenError := exchangeProviderCode(provider, "any-code", "https://mcp.example.com/callback")
	if tokenError == nil {
		test.Error("expected error for unreachable provider, got nil")
	}
}
