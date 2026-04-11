package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/reshinto/mcp-banana/internal/credentials"
)

// --- generateToken: randomSource failure ---

// failingReader is an io.Reader that always returns an error.
type failingReader struct{}

func (failingReader) Read(_ []byte) (int, error) {
	return 0, fmt.Errorf("simulated rand read failure")
}

// TestGenerateToken_RandomSourceFailure verifies that generateToken returns an error
// when the underlying randomSource fails. This covers the rand.Read error branch
// inside generateToken that is unreachable with crypto/rand.Reader.
func TestGenerateToken_RandomSourceFailure(test *testing.T) {
	originalSource := randomSource
	randomSource = failingReader{}
	defer func() { randomSource = originalSource }()

	// Call generateToken directly (not the var) to exercise the original function body.
	_, tokenError := generateToken(32)
	if tokenError == nil {
		test.Error("expected error from failing random source, got nil")
	}
}

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
// when GenerateRandomToken fails during session token creation in the callback handler.
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

	handler := NewCallbackHandler(store, providers, "https://mcp.example.com", newMockCredentialsStore())

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
	handler := NewCallbackHandler(store, providers, "https://mcp.example.com", newMockCredentialsStore())

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
	handler := NewCallbackHandler(store, providers, "https://mcp.example.com", newMockCredentialsStore())

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

// --- NewCallbackHandler: malformed redirect URI now stored in GeminiKeySession ---

// TestCallbackHandler_MalformedRedirectURI verifies that the callback handler successfully
// redirects to the Gemini key prompt even when the redirect URI is malformed. The malformed
// URI is stored in the GeminiKeySession and will cause an error later in issueAuthCodeRedirect.
func TestCallbackHandler_MalformedRedirectURI(test *testing.T) {
	store := NewStore()
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

	handler := NewCallbackHandler(store, providers, "https://mcp.example.com", newMockCredentialsStore())

	request := httptest.NewRequest(http.MethodGet, "/callback?code=any-code&state=state-bad-uri", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusFound {
		test.Errorf("expected status 302, got %d", recorder.Code)
	}
	location := recorder.Header().Get("Location")
	if !containsSubstring(location, "/gemini-key?session=") {
		test.Errorf("expected redirect to /gemini-key, got %s", location)
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

// TestExchangeProviderCode_InvalidJSON verifies that a non-JSON response body
// from the provider token endpoint causes a decode error.
func TestExchangeProviderCode_InvalidJSON(test *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		writer.Write([]byte("not-json")) //nolint:errcheck
	}))
	defer testServer.Close()

	provider := ProviderConfig{
		Name:         "test-provider",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		TokenURL:     testServer.URL,
	}

	_, tokenError := exchangeProviderCode(provider, "code-123", "https://mcp.example.com/callback")
	if tokenError == nil {
		test.Error("expected decode error, got nil")
	}
}

// --- NewCallbackHandler: identity fetch failure ---

// TestCallbackHandler_IdentityFetchFailure verifies that when fetchProviderIdentity
// fails, the callback handler returns 502 with an appropriate error message.
func TestCallbackHandler_IdentityFetchFailure(test *testing.T) {
	store := NewStore()
	store.StoreProviderSession(&ProviderSession{
		State:         "state-identity-fail",
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
		return "", fmt.Errorf("identity fetch failed")
	}
	defer func() { providerIdentityFetcher = originalFetcher }()

	handler := NewCallbackHandler(store, providers, "https://mcp.example.com", newMockCredentialsStore())

	request := httptest.NewRequest(http.MethodGet, "/callback?code=any-code&state=state-identity-fail", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadGateway {
		test.Errorf("expected status 502, got %d", recorder.Code)
	}
}

// --- NewCallbackHandler: returning user appends returning=true ---

// TestCallbackHandler_ReturningUser verifies that when the user already has a key
// in the credentials store, the redirect includes returning=true.
func TestCallbackHandler_ReturningUser(test *testing.T) {
	store := NewStore()
	store.StoreProviderSession(&ProviderSession{
		State:         "state-returning",
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
		return "google:returning@example.com", nil
	}
	defer func() { providerIdentityFetcher = originalFetcher }()

	credStore := newMockCredentialsStore()
	credStore.keys["google:returning@example.com"] = "existing-gemini-key"

	handler := NewCallbackHandler(store, providers, "https://mcp.example.com", credStore)

	request := httptest.NewRequest(http.MethodGet, "/callback?code=any-code&state=state-returning", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusFound {
		test.Errorf("expected status 302, got %d", recorder.Code)
	}

	location := recorder.Header().Get("Location")
	if !containsSubstring(location, "returning=true") {
		test.Errorf("expected returning=true in redirect, got %s", location)
	}
}

// --- NewGeminiKeyPromptHandler: template execute error ---

// TestGeminiKeyPromptHandler_TemplateExecuteError verifies that the defensive
// http.Error call after template.Execute is reached when the writer fails.
func TestGeminiKeyPromptHandler_TemplateExecuteError(test *testing.T) {
	store := NewStore()
	store.StoreGeminiKeySession("prompt-error-session", &GeminiKeySession{
		ProviderIdentity: "google:user@example.com",
		ClientID:         "client-abc",
		RedirectURI:      "https://app.example.com/callback",
		CodeChallenge:    "challenge123",
		OriginalState:    "state456",
		ExpiresAt:        time.Now().Add(10 * time.Minute),
	})

	handler := NewGeminiKeyPromptHandler(store)

	writer := newErrorAfterHeadersWriter()
	request := httptest.NewRequest(http.MethodGet, "/gemini-key?session=prompt-error-session", nil)

	handler.ServeHTTP(writer, request)

	// The handler ran without panicking — the error branch was exercised.
}

// --- NewGeminiKeySubmitHandler: parse form failure ---

// TestGeminiKeySubmitHandler_ParseFormFailure verifies that a malformed POST body
// causes a 400 response.
func TestGeminiKeySubmitHandler_ParseFormFailure(test *testing.T) {
	store := NewStore()
	handler := NewGeminiKeySubmitHandler(store, newMockCredentialsStore())

	body := strings.NewReader("%invalid-percent-encoding")
	request := httptest.NewRequest(http.MethodPost, "/gemini-key-submit", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		test.Errorf("expected status 400, got %d", recorder.Code)
	}
}

// --- NewGeminiKeySubmitHandler: invalid/expired session ---

// TestGeminiKeySubmitHandler_InvalidSession verifies that submitting with a
// nonexistent session token returns 400.
func TestGeminiKeySubmitHandler_InvalidSession(test *testing.T) {
	store := NewStore()
	handler := NewGeminiKeySubmitHandler(store, newMockCredentialsStore())

	formData := url.Values{}
	formData.Set("session_token", "nonexistent-session")
	formData.Set("gemini_api_key", "some-key")

	request := httptest.NewRequest(http.MethodPost, "/gemini-key-submit", bytes.NewBufferString(formData.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		test.Errorf("expected status 400, got %d", recorder.Code)
	}
}

// --- NewGeminiKeySubmitHandler: skip with no existing key ---

// TestGeminiKeySubmitHandler_SkipWithoutExistingKey verifies that the skip action
// redirects back to the prompt with an error when the user has no stored key.
func TestGeminiKeySubmitHandler_SkipWithoutExistingKey(test *testing.T) {
	store := NewStore()
	credStore := newMockCredentialsStore()

	store.StoreGeminiKeySession("skip-no-key-session", &GeminiKeySession{
		ProviderIdentity: "google:newuser@example.com",
		ClientID:         "client-abc",
		RedirectURI:      "https://app.example.com/callback",
		CodeChallenge:    "challenge123",
		OriginalState:    "state456",
		ExpiresAt:        time.Now().Add(10 * time.Minute),
	})

	handler := NewGeminiKeySubmitHandler(store, credStore)

	formData := url.Values{}
	formData.Set("session_token", "skip-no-key-session")
	formData.Set("action", "skip")

	request := httptest.NewRequest(http.MethodPost, "/gemini-key-submit", bytes.NewBufferString(formData.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusFound {
		test.Errorf("expected status 302, got %d: %s", recorder.Code, recorder.Body.String())
	}

	location := recorder.Header().Get("Location")
	if !containsSubstring(location, "/gemini-key") {
		test.Errorf("expected redirect to /gemini-key, got %s", location)
	}
	if !containsSubstring(location, "error=") {
		test.Errorf("expected error param in redirect, got %s", location)
	}
}

// --- NewGeminiKeySubmitHandler: skip with no existing key + token generation failure ---

// TestGeminiKeySubmitHandler_SkipNoKey_TokenFailure verifies that a token generation
// failure during the skip-no-key redirect returns 500.
func TestGeminiKeySubmitHandler_SkipNoKey_TokenFailure(test *testing.T) {
	store := NewStore()
	credStore := newMockCredentialsStore()

	store.StoreGeminiKeySession("skip-token-fail", &GeminiKeySession{
		ProviderIdentity: "google:newuser@example.com",
		ClientID:         "client-abc",
		RedirectURI:      "https://app.example.com/callback",
		CodeChallenge:    "challenge123",
		OriginalState:    "state456",
		ExpiresAt:        time.Now().Add(10 * time.Minute),
	})

	handler := NewGeminiKeySubmitHandler(store, credStore)

	withFailingTokenGenerator(test, func() {
		formData := url.Values{}
		formData.Set("session_token", "skip-token-fail")
		formData.Set("action", "skip")

		request := httptest.NewRequest(http.MethodPost, "/gemini-key-submit", bytes.NewBufferString(formData.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recorder := httptest.NewRecorder()

		handler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusInternalServerError {
			test.Errorf("expected status 500, got %d", recorder.Code)
		}
	})
}

// --- NewGeminiKeySubmitHandler: empty key with returning user ---

// TestGeminiKeySubmitHandler_EmptyKeyReturningUser verifies that an empty key
// submission for a returning user redirects with returning=true.
func TestGeminiKeySubmitHandler_EmptyKeyReturningUser(test *testing.T) {
	store := NewStore()
	credStore := newMockCredentialsStore()
	credStore.keys["google:returning@example.com"] = "existing-key"

	store.StoreGeminiKeySession("empty-key-returning", &GeminiKeySession{
		ProviderIdentity: "google:returning@example.com",
		ClientID:         "client-abc",
		RedirectURI:      "https://app.example.com/callback",
		CodeChallenge:    "challenge123",
		OriginalState:    "state456",
		ExpiresAt:        time.Now().Add(10 * time.Minute),
	})

	handler := NewGeminiKeySubmitHandler(store, credStore)

	formData := url.Values{}
	formData.Set("session_token", "empty-key-returning")
	formData.Set("gemini_api_key", "")

	request := httptest.NewRequest(http.MethodPost, "/gemini-key-submit", bytes.NewBufferString(formData.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusFound {
		test.Errorf("expected status 302, got %d", recorder.Code)
	}

	location := recorder.Header().Get("Location")
	if !containsSubstring(location, "returning=true") {
		test.Errorf("expected returning=true in redirect, got %s", location)
	}
}

// --- NewGeminiKeySubmitHandler: empty key token generation failure ---

// TestGeminiKeySubmitHandler_EmptyKey_TokenFailure verifies that a token generation
// failure during the empty-key redirect returns 500.
func TestGeminiKeySubmitHandler_EmptyKey_TokenFailure(test *testing.T) {
	store := NewStore()
	credStore := newMockCredentialsStore()

	store.StoreGeminiKeySession("empty-key-token-fail", &GeminiKeySession{
		ProviderIdentity: "google:user@example.com",
		ClientID:         "client-abc",
		RedirectURI:      "https://app.example.com/callback",
		CodeChallenge:    "challenge123",
		OriginalState:    "state456",
		ExpiresAt:        time.Now().Add(10 * time.Minute),
	})

	handler := NewGeminiKeySubmitHandler(store, credStore)

	withFailingTokenGenerator(test, func() {
		formData := url.Values{}
		formData.Set("session_token", "empty-key-token-fail")
		formData.Set("gemini_api_key", "")

		request := httptest.NewRequest(http.MethodPost, "/gemini-key-submit", bytes.NewBufferString(formData.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recorder := httptest.NewRecorder()

		handler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusInternalServerError {
			test.Errorf("expected status 500, got %d", recorder.Code)
		}
	})
}

// --- NewGeminiKeySubmitHandler: Gemini key validation failure ---

// TestGeminiKeySubmitHandler_ValidationFailure verifies that an invalid Gemini API key
// redirects back to the prompt with an error message.
func TestGeminiKeySubmitHandler_ValidationFailure(test *testing.T) {
	store := NewStore()
	credStore := newMockCredentialsStore()

	store.StoreGeminiKeySession("validate-fail-session", &GeminiKeySession{
		ProviderIdentity: "google:user@example.com",
		ClientID:         "client-abc",
		RedirectURI:      "https://app.example.com/callback",
		CodeChallenge:    "challenge123",
		OriginalState:    "state456",
		ExpiresAt:        time.Now().Add(10 * time.Minute),
	})

	restoreValidator := credentials.OverrideGeminiKeyValidator(func(ctx context.Context, apiKey string) error {
		return fmt.Errorf("invalid key")
	})
	defer restoreValidator()

	handler := NewGeminiKeySubmitHandler(store, credStore)

	formData := url.Values{}
	formData.Set("session_token", "validate-fail-session")
	formData.Set("gemini_api_key", "bad-key-12345")

	request := httptest.NewRequest(http.MethodPost, "/gemini-key-submit", bytes.NewBufferString(formData.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusFound {
		test.Errorf("expected status 302, got %d: %s", recorder.Code, recorder.Body.String())
	}

	location := recorder.Header().Get("Location")
	if !containsSubstring(location, "error=") {
		test.Errorf("expected error param in redirect, got %s", location)
	}
}

// --- NewGeminiKeySubmitHandler: validation failure + returning user ---

// TestGeminiKeySubmitHandler_ValidationFailure_ReturningUser verifies that validation
// failure for a returning user includes returning=true in the redirect.
func TestGeminiKeySubmitHandler_ValidationFailure_ReturningUser(test *testing.T) {
	store := NewStore()
	credStore := newMockCredentialsStore()
	credStore.keys["google:returning@example.com"] = "existing-key"

	store.StoreGeminiKeySession("validate-fail-returning", &GeminiKeySession{
		ProviderIdentity: "google:returning@example.com",
		ClientID:         "client-abc",
		RedirectURI:      "https://app.example.com/callback",
		CodeChallenge:    "challenge123",
		OriginalState:    "state456",
		ExpiresAt:        time.Now().Add(10 * time.Minute),
	})

	restoreValidator := credentials.OverrideGeminiKeyValidator(func(ctx context.Context, apiKey string) error {
		return fmt.Errorf("invalid key")
	})
	defer restoreValidator()

	handler := NewGeminiKeySubmitHandler(store, credStore)

	formData := url.Values{}
	formData.Set("session_token", "validate-fail-returning")
	formData.Set("gemini_api_key", "bad-key-12345")

	request := httptest.NewRequest(http.MethodPost, "/gemini-key-submit", bytes.NewBufferString(formData.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusFound {
		test.Errorf("expected status 302, got %d", recorder.Code)
	}

	location := recorder.Header().Get("Location")
	if !containsSubstring(location, "returning=true") {
		test.Errorf("expected returning=true in redirect, got %s", location)
	}
}

// --- NewGeminiKeySubmitHandler: validation failure + token generation failure ---

// TestGeminiKeySubmitHandler_ValidationFailure_TokenFailure verifies that a token
// generation failure during validation-failure redirect returns 500.
func TestGeminiKeySubmitHandler_ValidationFailure_TokenFailure(test *testing.T) {
	store := NewStore()
	credStore := newMockCredentialsStore()

	store.StoreGeminiKeySession("validate-token-fail", &GeminiKeySession{
		ProviderIdentity: "google:user@example.com",
		ClientID:         "client-abc",
		RedirectURI:      "https://app.example.com/callback",
		CodeChallenge:    "challenge123",
		OriginalState:    "state456",
		ExpiresAt:        time.Now().Add(10 * time.Minute),
	})

	restoreValidator := credentials.OverrideGeminiKeyValidator(func(ctx context.Context, apiKey string) error {
		return fmt.Errorf("invalid key")
	})
	defer restoreValidator()

	handler := NewGeminiKeySubmitHandler(store, credStore)

	withFailingTokenGenerator(test, func() {
		formData := url.Values{}
		formData.Set("session_token", "validate-token-fail")
		formData.Set("gemini_api_key", "bad-key-12345")

		request := httptest.NewRequest(http.MethodPost, "/gemini-key-submit", bytes.NewBufferString(formData.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recorder := httptest.NewRecorder()

		handler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusInternalServerError {
			test.Errorf("expected status 500, got %d", recorder.Code)
		}
	})
}

// --- failingCredentialsStore: Register always fails ---

// failingCredentialsStore wraps mockCredentialsStore but makes Register return an error.
type failingCredentialsStore struct {
	inner *mockCredentialsStore
}

func (mock *failingCredentialsStore) Lookup(identity string) string {
	return mock.inner.Lookup(identity)
}

func (mock *failingCredentialsStore) Exists(identity string) bool {
	return mock.inner.Exists(identity)
}

func (mock *failingCredentialsStore) Register(_ string, _ string) error {
	return fmt.Errorf("simulated register failure")
}

// --- NewGeminiKeySubmitHandler: register failure ---

// TestGeminiKeySubmitHandler_RegisterFailure verifies that a credStore.Register failure
// returns 500 server_error.
func TestGeminiKeySubmitHandler_RegisterFailure(test *testing.T) {
	store := NewStore()
	credStore := &failingCredentialsStore{inner: newMockCredentialsStore()}

	store.StoreGeminiKeySession("register-fail-session", &GeminiKeySession{
		ProviderIdentity: "google:user@example.com",
		ClientID:         "client-abc",
		RedirectURI:      "https://app.example.com/callback",
		CodeChallenge:    "challenge123",
		OriginalState:    "state456",
		ExpiresAt:        time.Now().Add(10 * time.Minute),
	})

	restoreValidator := credentials.OverrideGeminiKeyValidator(func(ctx context.Context, apiKey string) error {
		return nil
	})
	defer restoreValidator()

	handler := NewGeminiKeySubmitHandler(store, credStore)

	formData := url.Values{}
	formData.Set("session_token", "register-fail-session")
	formData.Set("gemini_api_key", "AIzaValidKey123456")

	request := httptest.NewRequest(http.MethodPost, "/gemini-key-submit", bytes.NewBufferString(formData.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		test.Errorf("expected status 500, got %d", recorder.Code)
	}
}

// --- issueAuthCodeRedirect: GenerateRandomToken failure ---

// TestIssueAuthCodeRedirect_TokenGenerationFailure verifies that a token generation
// failure in issueAuthCodeRedirect returns 500.
func TestIssueAuthCodeRedirect_TokenGenerationFailure(test *testing.T) {
	store := NewStore()
	credStore := newMockCredentialsStore()
	credStore.keys["google:user@example.com"] = "existing-key"

	store.StoreGeminiKeySession("issue-token-fail", &GeminiKeySession{
		ProviderIdentity: "google:user@example.com",
		ClientID:         "client-abc",
		RedirectURI:      "https://app.example.com/callback",
		CodeChallenge:    "challenge123",
		OriginalState:    "state456",
		ExpiresAt:        time.Now().Add(10 * time.Minute),
	})

	restoreValidator := credentials.OverrideGeminiKeyValidator(func(ctx context.Context, apiKey string) error {
		return nil
	})
	defer restoreValidator()

	handler := NewGeminiKeySubmitHandler(store, credStore)

	withFailingTokenGenerator(test, func() {
		formData := url.Values{}
		formData.Set("session_token", "issue-token-fail")
		formData.Set("action", "skip")

		request := httptest.NewRequest(http.MethodPost, "/gemini-key-submit", bytes.NewBufferString(formData.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recorder := httptest.NewRecorder()

		handler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusInternalServerError {
			test.Errorf("expected status 500, got %d", recorder.Code)
		}
	})
}

// --- issueAuthCodeRedirect: malformed redirect URI causes url.Parse error ---

// TestIssueAuthCodeRedirect_URLParseFailure verifies that a malformed redirect URI
// stored in the session causes issueAuthCodeRedirect to return 500.
func TestIssueAuthCodeRedirect_URLParseFailure(test *testing.T) {
	store := NewStore()
	credStore := newMockCredentialsStore()
	credStore.keys["google:user@example.com"] = "existing-key"

	store.StoreGeminiKeySession("parse-fail-session", &GeminiKeySession{
		ProviderIdentity: "google:user@example.com",
		ClientID:         "client-abc",
		RedirectURI:      "://\x00invalid",
		CodeChallenge:    "challenge123",
		OriginalState:    "state456",
		ExpiresAt:        time.Now().Add(10 * time.Minute),
	})

	handler := NewGeminiKeySubmitHandler(store, credStore)

	formData := url.Values{}
	formData.Set("session_token", "parse-fail-session")
	formData.Set("action", "skip")

	request := httptest.NewRequest(http.MethodPost, "/gemini-key-submit", bytes.NewBufferString(formData.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		test.Errorf("expected status 500, got %d", recorder.Code)
	}
}

// --- NewProtectedResourceHandler ---

// TestProtectedResourceHandler_ReturnsMetadata verifies that the protected resource
// metadata endpoint returns the expected JSON document.
func TestProtectedResourceHandler_ReturnsMetadata(test *testing.T) {
	handler := NewProtectedResourceHandler("https://mcp.example.com")

	request := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		test.Errorf("expected status 200, got %d", recorder.Code)
	}

	var metadata ProtectedResourceMetadata
	if decodeError := json.NewDecoder(recorder.Body).Decode(&metadata); decodeError != nil {
		test.Fatalf("failed to decode metadata response: %v", decodeError)
	}
	if metadata.Resource != "https://mcp.example.com" {
		test.Errorf("expected resource https://mcp.example.com, got %s", metadata.Resource)
	}
	if len(metadata.AuthorizationServers) != 1 || metadata.AuthorizationServers[0] != "https://mcp.example.com" {
		test.Errorf("unexpected authorization_servers: %v", metadata.AuthorizationServers)
	}
	if len(metadata.BearerMethodsSupported) != 1 || metadata.BearerMethodsSupported[0] != "header" {
		test.Errorf("unexpected bearer_methods_supported: %v", metadata.BearerMethodsSupported)
	}
}

// TestProtectedResourceHandler_EncodeErrorPath verifies the json.Encode error branch
// in NewProtectedResourceHandler when the writer fails.
func TestProtectedResourceHandler_EncodeErrorPath(test *testing.T) {
	handler := NewProtectedResourceHandler("https://mcp.example.com")
	writer := newErrorAfterHeadersWriter()
	request := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)

	handler.ServeHTTP(writer, request)

	// The handler ran without panicking — the error branch was exercised.
}

// --- CleanupExpired: geminiKeySessions ---

// TestCleanupExpired_IncludesGeminiKeySessions verifies that CleanupExpired removes
// expired geminiKeySessions while preserving valid ones.
func TestCleanupExpired_IncludesGeminiKeySessions(test *testing.T) {
	store := NewStore()

	store.StoreGeminiKeySession("expired-gemini-session", &GeminiKeySession{
		ProviderIdentity: "google:expired@example.com",
		ExpiresAt:        time.Now().Add(-1 * time.Second),
	})
	store.StoreGeminiKeySession("valid-gemini-session", &GeminiKeySession{
		ProviderIdentity: "google:valid@example.com",
		ExpiresAt:        time.Now().Add(10 * time.Minute),
	})

	store.CleanupExpired()

	if store.PeekGeminiKeySession("expired-gemini-session") != nil {
		test.Error("expected expired gemini key session to be cleaned up")
	}
	if store.PeekGeminiKeySession("valid-gemini-session") == nil {
		test.Error("expected valid gemini key session to survive cleanup")
	}
}

// --- providerIdentityFetcher: real implementation tests ---

// TestProviderIdentityFetcher_Success verifies the real providerIdentityFetcher function
// against a local test server returning a valid email.
func TestProviderIdentityFetcher_Success(test *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authHeader := request.Header.Get("Authorization")
		if authHeader != "Bearer test-access-token" {
			test.Errorf("expected Bearer test-access-token, got %s", authHeader)
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		json.NewEncoder(writer).Encode(map[string]string{"email": "user@example.com"}) //nolint:errcheck
	}))
	defer testServer.Close()

	provider := ProviderConfig{
		Name:        "testprovider",
		UserInfoURL: testServer.URL,
	}

	identity, identityError := providerIdentityFetcher(provider, "test-access-token")
	if identityError != nil {
		test.Errorf("expected nil error, got: %v", identityError)
	}
	if identity != "testprovider:user@example.com" {
		test.Errorf("expected testprovider:user@example.com, got %s", identity)
	}
}

// TestProviderIdentityFetcher_NoUserInfoURL verifies that a provider without a
// UserInfoURL returns an error.
func TestProviderIdentityFetcher_NoUserInfoURL(test *testing.T) {
	provider := ProviderConfig{
		Name:        "testprovider",
		UserInfoURL: "",
	}

	_, identityError := providerIdentityFetcher(provider, "test-access-token")
	if identityError == nil {
		test.Error("expected error for missing userinfo URL, got nil")
	}
}

// TestProviderIdentityFetcher_NonOKStatus verifies that a non-200 response from the
// userinfo endpoint returns an error.
func TestProviderIdentityFetcher_NonOKStatus(test *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusForbidden)
	}))
	defer testServer.Close()

	provider := ProviderConfig{
		Name:        "testprovider",
		UserInfoURL: testServer.URL,
	}

	_, identityError := providerIdentityFetcher(provider, "test-access-token")
	if identityError == nil {
		test.Error("expected error for non-200 userinfo response, got nil")
	}
}

// TestProviderIdentityFetcher_InvalidJSON verifies that a non-JSON response from the
// userinfo endpoint returns a decode error.
func TestProviderIdentityFetcher_InvalidJSON(test *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		writer.Write([]byte("not-json")) //nolint:errcheck
	}))
	defer testServer.Close()

	provider := ProviderConfig{
		Name:        "testprovider",
		UserInfoURL: testServer.URL,
	}

	_, identityError := providerIdentityFetcher(provider, "test-access-token")
	if identityError == nil {
		test.Error("expected decode error, got nil")
	}
}

// TestProviderIdentityFetcher_EmptyEmail verifies that a response without an email
// field returns an error.
func TestProviderIdentityFetcher_EmptyEmail(test *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		json.NewEncoder(writer).Encode(map[string]string{"email": ""}) //nolint:errcheck
	}))
	defer testServer.Close()

	provider := ProviderConfig{
		Name:        "testprovider",
		UserInfoURL: testServer.URL,
	}

	_, identityError := providerIdentityFetcher(provider, "test-access-token")
	if identityError == nil {
		test.Error("expected error for empty email, got nil")
	}
}

// TestProviderIdentityFetcher_UnreachableServer verifies that a network failure
// returns an error.
func TestProviderIdentityFetcher_UnreachableServer(test *testing.T) {
	provider := ProviderConfig{
		Name:        "testprovider",
		UserInfoURL: "http://127.0.0.1:1", // port 1 is always refused
	}

	_, identityError := providerIdentityFetcher(provider, "test-access-token")
	if identityError == nil {
		test.Error("expected error for unreachable server, got nil")
	}
}

// TestProviderIdentityFetcher_InvalidURL verifies that a malformed URL that passes
// the empty-string check but fails http.NewRequest returns an error.
func TestProviderIdentityFetcher_InvalidURL(test *testing.T) {
	provider := ProviderConfig{
		Name:        "testprovider",
		UserInfoURL: "http://invalid\x7f-url",
	}

	_, identityError := providerIdentityFetcher(provider, "test-access-token")
	if identityError == nil {
		test.Error("expected error for invalid URL, got nil")
	}
}

// TestProviderIdentityFetcher_GitHubAcceptHeader verifies that the github provider
// sends the correct Accept header.
func TestProviderIdentityFetcher_GitHubAcceptHeader(test *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		acceptHeader := request.Header.Get("Accept")
		if acceptHeader != "application/vnd.github+json" {
			test.Errorf("expected github Accept header, got %s", acceptHeader)
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		json.NewEncoder(writer).Encode(map[string]string{"email": "dev@github.com"}) //nolint:errcheck
	}))
	defer testServer.Close()

	provider := ProviderConfig{
		Name:        "github",
		UserInfoURL: testServer.URL,
	}

	identity, identityError := providerIdentityFetcher(provider, "gh-token")
	if identityError != nil {
		test.Errorf("expected nil error, got: %v", identityError)
	}
	if identity != "github:dev@github.com" {
		test.Errorf("expected github:dev@github.com, got %s", identity)
	}
}
