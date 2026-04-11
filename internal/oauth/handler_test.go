package oauth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/reshinto/mcp-banana/internal/credentials"
)

// mockCredentialsStore is a test double for the CredentialsStore interface.
type mockCredentialsStore struct {
	keys map[string]string
}

func newMockCredentialsStore() *mockCredentialsStore {
	return &mockCredentialsStore{keys: make(map[string]string)}
}

func (mock *mockCredentialsStore) Lookup(identity string) string {
	return mock.keys[identity]
}

func (mock *mockCredentialsStore) Exists(identity string) bool {
	return mock.keys[identity] != ""
}

func (mock *mockCredentialsStore) Register(identity string, geminiAPIKey string) error {
	mock.keys[identity] = geminiAPIKey
	return nil
}

// containsSubstring reports whether needle appears within haystack.
func containsSubstring(haystack, needle string) bool {
	haystackLen := len(haystack)
	needleLen := len(needle)
	if needleLen == 0 {
		return true
	}
	if needleLen > haystackLen {
		return false
	}
	for startIndex := 0; startIndex <= haystackLen-needleLen; startIndex++ {
		if haystack[startIndex:startIndex+needleLen] == needle {
			return true
		}
	}
	return false
}

func TestAuthorizeHandler_RendersLoginPage(test *testing.T) {
	store := NewStore()
	store.RegisterClient(&Client{
		ClientID:     "client-abc",
		ClientName:   "Test Client",
		RedirectURIs: []string{"https://app.example.com/callback"},
	})

	providers := []ProviderConfig{
		NewGoogleProvider("google-client-id", "google-client-secret"),
	}
	handler := NewAuthorizeHandler(store, providers, "https://mcp.example.com")

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

	if recorder.Code != http.StatusOK {
		test.Errorf("expected status 200, got %d", recorder.Code)
	}

	responseBody := recorder.Body.String()
	if !containsSubstring(responseBody, "Sign in with Google") {
		test.Error("expected response body to contain 'Sign in with Google'")
	}
	if !containsSubstring(responseBody, "mcp-banana") {
		test.Error("expected response body to contain 'mcp-banana'")
	}
}

func TestAuthorizeHandler_AutoRegistersUnknownClient(test *testing.T) {
	store := NewStore()
	handler := NewAuthorizeHandler(store, nil, "https://mcp.example.com")

	request := httptest.NewRequest(http.MethodGet, "/authorize?"+
		"response_type=code"+
		"&client_id=nonexistent"+
		"&redirect_uri=https%3A%2F%2Fapp.example.com%2Fcallback"+
		"&state=random-state"+
		"&code_challenge=abc123"+
		"&code_challenge_method=S256",
		nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	// Auto-registration should succeed — renders login page (200), not 400
	if recorder.Code != http.StatusOK {
		test.Errorf("expected status 200 (auto-registered), got %d", recorder.Code)
	}
	// Verify the client was registered
	client := store.GetClient("nonexistent")
	if client == nil {
		test.Fatal("expected client to be auto-registered")
	}
}

func TestAuthorizeHandler_RejectsEmptyClientID(test *testing.T) {
	store := NewStore()
	handler := NewAuthorizeHandler(store, nil, "https://mcp.example.com")

	request := httptest.NewRequest(http.MethodGet, "/authorize?"+
		"response_type=code"+
		"&client_id="+
		"&redirect_uri=https%3A%2F%2Fapp.example.com%2Fcallback"+
		"&state=random-state"+
		"&code_challenge=abc123"+
		"&code_challenge_method=S256",
		nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		test.Errorf("expected status 400 for empty client_id, got %d", recorder.Code)
	}
}

func TestAuthorizeHandler_MissingCodeChallenge(test *testing.T) {
	store := NewStore()
	store.RegisterClient(&Client{
		ClientID:     "client-abc",
		ClientName:   "Test Client",
		RedirectURIs: []string{"https://app.example.com/callback"},
	})
	handler := NewAuthorizeHandler(store, nil, "https://mcp.example.com")

	request := httptest.NewRequest(http.MethodGet, "/authorize?"+
		"response_type=code"+
		"&client_id=client-abc"+
		"&redirect_uri=https%3A%2F%2Fapp.example.com%2Fcallback"+
		"&state=random-state"+
		"&code_challenge_method=S256",
		nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		test.Errorf("expected status 400, got %d", recorder.Code)
	}
}

func TestAuthorizeHandler_RedirectURIMismatch(test *testing.T) {
	store := NewStore()
	store.RegisterClient(&Client{
		ClientID:     "client-abc",
		ClientName:   "Test Client",
		RedirectURIs: []string{"https://app.example.com/callback"},
	})
	handler := NewAuthorizeHandler(store, nil, "https://mcp.example.com")

	request := httptest.NewRequest(http.MethodGet, "/authorize?"+
		"response_type=code"+
		"&client_id=client-abc"+
		"&redirect_uri=https%3A%2F%2Fevil.example.com%2Fcallback"+
		"&state=random-state"+
		"&code_challenge=abc123"+
		"&code_challenge_method=S256",
		nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		test.Errorf("expected status 400, got %d", recorder.Code)
	}
}

// computeS256Challenge derives the PKCE S256 code_challenge from a plaintext verifier.
func computeS256Challenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

// TestTokenHandler_AuthorizationCodeExchange verifies that a valid authorization code
// with a correct PKCE verifier is exchanged for access and refresh tokens.
func TestTokenHandler_AuthorizationCodeExchange(test *testing.T) {
	store := NewStore()
	store.RegisterClient(&Client{
		ClientID:     "client-abc",
		ClientName:   "Test Client",
		RedirectURIs: []string{"https://app.example.com/callback"},
	})

	verifier := "my-code-verifier-long-enough-to-be-valid"
	challenge := computeS256Challenge(verifier)

	store.StoreAuthCode(&AuthCode{
		Code:          "test-auth-code",
		ClientID:      "client-abc",
		RedirectURI:   "https://app.example.com/callback",
		CodeChallenge: challenge,
		ExpiresAt:     time.Now().Add(10 * time.Minute),
	})

	handler := NewTokenHandler(store)

	formData := url.Values{}
	formData.Set("grant_type", "authorization_code")
	formData.Set("code", "test-auth-code")
	formData.Set("client_id", "client-abc")
	formData.Set("redirect_uri", "https://app.example.com/callback")
	formData.Set("code_verifier", verifier)

	request := httptest.NewRequest(http.MethodPost, "/token", bytes.NewBufferString(formData.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		test.Errorf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var tokenResp map[string]interface{}
	if decodeError := json.NewDecoder(recorder.Body).Decode(&tokenResp); decodeError != nil {
		test.Fatalf("failed to decode response: %v", decodeError)
	}
	if tokenResp["access_token"] == "" || tokenResp["access_token"] == nil {
		test.Error("expected non-empty access_token")
	}
	if tokenResp["refresh_token"] == "" || tokenResp["refresh_token"] == nil {
		test.Error("expected non-empty refresh_token")
	}
	if tokenResp["token_type"] != "Bearer" {
		test.Errorf("expected token_type Bearer, got %v", tokenResp["token_type"])
	}
}

// TestTokenHandler_InvalidCode verifies that a nonexistent authorization code returns 400.
func TestTokenHandler_InvalidCode(test *testing.T) {
	store := NewStore()
	handler := NewTokenHandler(store)

	formData := url.Values{}
	formData.Set("grant_type", "authorization_code")
	formData.Set("code", "nonexistent-code")
	formData.Set("client_id", "client-abc")
	formData.Set("redirect_uri", "https://app.example.com/callback")
	formData.Set("code_verifier", "some-verifier")

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

// TestTokenHandler_InvalidPKCE verifies that a valid code paired with a wrong verifier returns 400.
func TestTokenHandler_InvalidPKCE(test *testing.T) {
	store := NewStore()
	store.RegisterClient(&Client{
		ClientID:     "client-abc",
		ClientName:   "Test Client",
		RedirectURIs: []string{"https://app.example.com/callback"},
	})

	verifier := "correct-verifier-long-enough-to-be-valid"
	challenge := computeS256Challenge(verifier)

	store.StoreAuthCode(&AuthCode{
		Code:          "test-auth-code-pkce",
		ClientID:      "client-abc",
		RedirectURI:   "https://app.example.com/callback",
		CodeChallenge: challenge,
		ExpiresAt:     time.Now().Add(10 * time.Minute),
	})

	handler := NewTokenHandler(store)

	formData := url.Values{}
	formData.Set("grant_type", "authorization_code")
	formData.Set("code", "test-auth-code-pkce")
	formData.Set("client_id", "client-abc")
	formData.Set("redirect_uri", "https://app.example.com/callback")
	formData.Set("code_verifier", "wrong-verifier")

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

// TestTokenHandler_RefreshToken verifies that a valid refresh token is exchanged for new tokens.
func TestTokenHandler_RefreshToken(test *testing.T) {
	store := NewStore()
	store.RegisterClient(&Client{
		ClientID:     "client-abc",
		ClientName:   "Test Client",
		RedirectURIs: []string{"https://app.example.com/callback"},
	})

	store.StoreRefreshToken(&RefreshData{
		Token:     "valid-refresh-token",
		ClientID:  "client-abc",
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
	})

	handler := NewTokenHandler(store)

	formData := url.Values{}
	formData.Set("grant_type", "refresh_token")
	formData.Set("refresh_token", "valid-refresh-token")
	formData.Set("client_id", "client-abc")

	request := httptest.NewRequest(http.MethodPost, "/token", bytes.NewBufferString(formData.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		test.Errorf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var tokenResp map[string]interface{}
	if decodeError := json.NewDecoder(recorder.Body).Decode(&tokenResp); decodeError != nil {
		test.Fatalf("failed to decode response: %v", decodeError)
	}
	if tokenResp["access_token"] == "" || tokenResp["access_token"] == nil {
		test.Error("expected non-empty access_token")
	}
	if tokenResp["refresh_token"] == "" || tokenResp["refresh_token"] == nil {
		test.Error("expected non-empty refresh_token")
	}
}

// TestTokenHandler_UnsupportedGrantType verifies that unknown grant types return 400.
func TestTokenHandler_UnsupportedGrantType(test *testing.T) {
	store := NewStore()
	handler := NewTokenHandler(store)

	formData := url.Values{}
	formData.Set("grant_type", "implicit")

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
	if errResp["error"] != "unsupported_grant_type" {
		test.Errorf("expected error unsupported_grant_type, got %s", errResp["error"])
	}
}

// TestTokenHandler_RefreshToken_ClientMismatch verifies that a refresh token for a different
// client_id is rejected with 400.
func TestTokenHandler_RefreshToken_ClientMismatch(test *testing.T) {
	store := NewStore()
	store.StoreRefreshToken(&RefreshData{
		Token:     "mismatch-refresh-token",
		ClientID:  "actual-client",
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
	})

	handler := NewTokenHandler(store)

	formData := url.Values{}
	formData.Set("grant_type", "refresh_token")
	formData.Set("refresh_token", "mismatch-refresh-token")
	formData.Set("client_id", "different-client")

	request := httptest.NewRequest(http.MethodPost, "/token", bytes.NewBufferString(formData.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		test.Errorf("expected status 400, got %d", recorder.Code)
	}
}

// TestCallbackHandler_InvalidState verifies that a callback with an unknown state returns 400.
func TestCallbackHandler_InvalidState(test *testing.T) {
	store := NewStore()
	providers := []ProviderConfig{NewGoogleProvider("gid", "gsecret")}
	handler := NewCallbackHandler(store, providers, "https://mcp.example.com", newMockCredentialsStore())

	request := httptest.NewRequest(http.MethodGet, "/callback?code=somecode&state=unknown-state", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		test.Errorf("expected status 400, got %d", recorder.Code)
	}

	var errResp map[string]string
	if decodeError := json.NewDecoder(recorder.Body).Decode(&errResp); decodeError != nil {
		test.Fatalf("failed to decode error response: %v", decodeError)
	}
	if errResp["error"] != "invalid_request" {
		test.Errorf("expected error invalid_request, got %s", errResp["error"])
	}
}

// TestCallbackHandler_Success verifies that a valid callback results in a redirect
// to the client redirect URI with code and state query parameters.
func TestCallbackHandler_Success(test *testing.T) {
	store := NewStore()
	store.RegisterClient(&Client{
		ClientID:     "client-abc",
		ClientName:   "Test Client",
		RedirectURIs: []string{"https://app.example.com/callback"},
	})

	store.StoreProviderSession(&ProviderSession{
		State:         "provider-state-xyz",
		ClientID:      "client-abc",
		RedirectURI:   "https://app.example.com/callback",
		CodeChallenge: "challenge-abc",
		OriginalState: "original-client-state",
		Provider:      "google",
		ExpiresAt:     time.Now().Add(10 * time.Minute),
	})

	providers := []ProviderConfig{NewGoogleProvider("gid", "gsecret")}

	// Replace the real HTTP calls with no-op mocks.
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

	request := httptest.NewRequest(http.MethodGet, "/callback?code=provider-code&state=provider-state-xyz", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusFound {
		test.Errorf("expected status 302, got %d", recorder.Code)
	}

	location := recorder.Header().Get("Location")
	if location == "" {
		test.Fatal("expected Location header to be set")
	}
	parsed, parseError := url.Parse(location)
	if parseError != nil {
		test.Fatalf("failed to parse Location header: %v", parseError)
	}
	if parsed.Path != "/gemini-key" {
		test.Errorf("expected redirect to /gemini-key, got %s", parsed.Path)
	}
	if parsed.Query().Get("session") == "" {
		test.Error("expected session query param to be set")
	}
}

// TestCallbackHandler_ProviderExchangeFailure verifies that a provider token exchange
// error returns 502.
func TestCallbackHandler_ProviderExchangeFailure(test *testing.T) {
	store := NewStore()
	store.StoreProviderSession(&ProviderSession{
		State:         "state-fail",
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
		return "", fmt.Errorf("upstream failure")
	}
	defer func() { exchangeProviderCode = originalExchange }()

	handler := NewCallbackHandler(store, providers, "https://mcp.example.com", newMockCredentialsStore())

	request := httptest.NewRequest(http.MethodGet, "/callback?code=bad-code&state=state-fail", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadGateway {
		test.Errorf("expected status 502, got %d", recorder.Code)
	}
}

// TestCallbackHandler_PostMethod verifies that Apple-style POST callbacks are handled
// using form body parameters.
func TestCallbackHandler_PostMethod(test *testing.T) {
	store := NewStore()
	store.StoreProviderSession(&ProviderSession{
		State:         "apple-state",
		ClientID:      "client-abc",
		RedirectURI:   "https://app.example.com/callback",
		CodeChallenge: "challenge-abc",
		OriginalState: "orig-state",
		Provider:      "apple",
		ExpiresAt:     time.Now().Add(10 * time.Minute),
	})

	providers := []ProviderConfig{NewAppleProvider("aid", "asecret")}

	originalExchange := exchangeProviderCode
	exchangeProviderCode = func(_ ProviderConfig, _ string, _ string) (string, error) {
		return "mock-access-token", nil
	}
	defer func() { exchangeProviderCode = originalExchange }()

	originalFetcher := providerIdentityFetcher
	providerIdentityFetcher = func(_ ProviderConfig, _ string) (string, error) {
		return "apple:user@example.com", nil
	}
	defer func() { providerIdentityFetcher = originalFetcher }()

	handler := NewCallbackHandler(store, providers, "https://mcp.example.com", newMockCredentialsStore())

	formData := url.Values{}
	formData.Set("code", "apple-auth-code")
	formData.Set("state", "apple-state")

	request := httptest.NewRequest(http.MethodPost, "/callback", bytes.NewBufferString(formData.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusFound {
		test.Errorf("expected status 302, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

// TestGeminiKeyPromptHandler_RendersPage verifies that a valid session token renders
// the Gemini key prompt page with HTTP 200.
func TestGeminiKeyPromptHandler_RendersPage(test *testing.T) {
	store := NewStore()
	store.StoreGeminiKeySession("valid-session", &GeminiKeySession{
		ProviderIdentity: "google:user@example.com",
		ClientID:         "client-abc",
		RedirectURI:      "https://app.example.com/callback",
		CodeChallenge:    "challenge123",
		OriginalState:    "state456",
		ExpiresAt:        time.Now().Add(10 * time.Minute),
	})

	handler := NewGeminiKeyPromptHandler(store)

	request := httptest.NewRequest(http.MethodGet, "/gemini-key?session=valid-session", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		test.Errorf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	responseBody := recorder.Body.String()
	if !containsSubstring(responseBody, "google:user@example.com") {
		test.Error("expected response body to contain user identity")
	}
}

// TestGeminiKeyPromptHandler_InvalidSession verifies that a missing or invalid session
// token returns HTTP 400.
func TestGeminiKeyPromptHandler_InvalidSession(test *testing.T) {
	store := NewStore()
	handler := NewGeminiKeyPromptHandler(store)

	request := httptest.NewRequest(http.MethodGet, "/gemini-key?session=nonexistent", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		test.Errorf("expected status 400, got %d", recorder.Code)
	}
}

// TestGeminiKeySubmitHandler_SkipWithExistingKey verifies that the skip action with an
// existing key in the credentials store redirects to the client redirect URI with a code.
func TestGeminiKeySubmitHandler_SkipWithExistingKey(test *testing.T) {
	store := NewStore()
	credStore := newMockCredentialsStore()
	credStore.keys["google:user@example.com"] = "existing-key"

	store.StoreGeminiKeySession("skip-session", &GeminiKeySession{
		ProviderIdentity: "google:user@example.com",
		ClientID:         "client-abc",
		RedirectURI:      "https://app.example.com/callback",
		CodeChallenge:    "challenge123",
		OriginalState:    "state456",
		ExpiresAt:        time.Now().Add(10 * time.Minute),
	})

	handler := NewGeminiKeySubmitHandler(store, credStore)

	formData := url.Values{}
	formData.Set("session_token", "skip-session")
	formData.Set("action", "skip")

	request := httptest.NewRequest(http.MethodPost, "/gemini-key-submit", bytes.NewBufferString(formData.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusFound {
		test.Errorf("expected status 302, got %d: %s", recorder.Code, recorder.Body.String())
	}

	location := recorder.Header().Get("Location")
	parsed, parseError := url.Parse(location)
	if parseError != nil {
		test.Fatalf("failed to parse Location: %v", parseError)
	}
	if !containsSubstring(parsed.Host, "app.example.com") {
		test.Errorf("expected redirect to client redirect URI, got %s", location)
	}
	if parsed.Query().Get("code") == "" {
		test.Error("expected code query param in redirect")
	}
	if parsed.Query().Get("state") != "state456" {
		test.Errorf("expected state=state456, got %s", parsed.Query().Get("state"))
	}
}

// TestGeminiKeySubmitHandler_RejectsEmptyKey verifies that submitting an empty API key
// redirects back to the prompt form with an error message.
func TestGeminiKeySubmitHandler_RejectsEmptyKey(test *testing.T) {
	store := NewStore()
	credStore := newMockCredentialsStore()

	store.StoreGeminiKeySession("empty-key-session", &GeminiKeySession{
		ProviderIdentity: "google:user@example.com",
		ClientID:         "client-abc",
		RedirectURI:      "https://app.example.com/callback",
		CodeChallenge:    "challenge123",
		OriginalState:    "state456",
		ExpiresAt:        time.Now().Add(10 * time.Minute),
	})

	handler := NewGeminiKeySubmitHandler(store, credStore)

	formData := url.Values{}
	formData.Set("session_token", "empty-key-session")
	formData.Set("gemini_api_key", "")

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
		test.Errorf("expected error query param in redirect, got %s", location)
	}
}

// TestGeminiKeySubmitHandler_SuccessfulRegistration verifies that a valid Gemini API key
// is registered and the user is redirected to the client redirect URI.
func TestGeminiKeySubmitHandler_SuccessfulRegistration(test *testing.T) {
	store := NewStore()
	credStore := newMockCredentialsStore()

	store.StoreGeminiKeySession("submit-session", &GeminiKeySession{
		ProviderIdentity: "google:user@example.com",
		ClientID:         "client-abc",
		RedirectURI:      "https://app.example.com/callback",
		CodeChallenge:    "challenge123",
		OriginalState:    "state456",
		ExpiresAt:        time.Now().Add(10 * time.Minute),
	})

	// Replace the Gemini key validator to avoid real API calls
	restoreValidator := credentials.OverrideGeminiKeyValidator(func(ctx context.Context, apiKey string) error {
		return nil
	})
	defer restoreValidator()

	handler := NewGeminiKeySubmitHandler(store, credStore)

	formData := url.Values{}
	formData.Set("session_token", "submit-session")
	formData.Set("gemini_api_key", "AIzaTestKey123456")

	request := httptest.NewRequest(http.MethodPost, "/gemini-key-submit", bytes.NewBufferString(formData.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusFound {
		test.Errorf("expected status 302, got %d: %s", recorder.Code, recorder.Body.String())
	}

	location := recorder.Header().Get("Location")
	parsed, parseError := url.Parse(location)
	if parseError != nil {
		test.Fatalf("failed to parse Location: %v", parseError)
	}
	if !containsSubstring(parsed.Host, "app.example.com") {
		test.Errorf("expected redirect to client redirect URI, got %s", location)
	}
	if parsed.Query().Get("code") == "" {
		test.Error("expected code query param in redirect")
	}

	// Verify the key was registered
	if credStore.Lookup("google:user@example.com") != "AIzaTestKey123456" {
		test.Error("expected Gemini key to be registered in credentials store")
	}
}
