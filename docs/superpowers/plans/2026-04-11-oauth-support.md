# OAuth Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add OAuth 2.1 support to mcp-banana so Claude Desktop can connect via custom connectors, while preserving existing bearer token auth for Claude Code CLI.

**Architecture:** New `internal/oauth/` package handles all OAuth logic (PKCE, token store, provider delegation, dynamic client registration). OAuth HTTP endpoints are registered alongside the existing `/mcp` route. The existing middleware is extended to validate OAuth-issued tokens as a fallback when static bearer tokens don't match. TLS support is added to `cmd/mcp-banana/main.go` via cert/key env vars.

**Tech Stack:** Go stdlib (`crypto/rand`, `crypto/sha256`, `embed`, `encoding/json`, `net/http`, `sync`), existing `internal/config/` and `internal/security/` packages.

---

### Task 1: PKCE Verification Logic

**Files:**
- Create: `internal/oauth/pkce.go`
- Test: `internal/oauth/pkce_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/oauth/pkce_test.go
package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestVerifyCodeChallenge_ValidS256(test *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])

	result := VerifyCodeChallenge(challenge, verifier, "S256")
	if !result {
		test.Errorf("expected valid PKCE challenge to pass, got false")
	}
}

func TestVerifyCodeChallenge_InvalidVerifier(test *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])

	result := VerifyCodeChallenge(challenge, "wrong-verifier", "S256")
	if result {
		test.Errorf("expected invalid verifier to fail, got true")
	}
}

func TestVerifyCodeChallenge_UnsupportedMethod(test *testing.T) {
	result := VerifyCodeChallenge("challenge", "verifier", "plain")
	if result {
		test.Errorf("expected unsupported method to fail, got true")
	}
}

func TestGenerateRandomToken_Length(test *testing.T) {
	token, tokenError := GenerateRandomToken(32)
	if tokenError != nil {
		test.Fatalf("unexpected error: %v", tokenError)
	}
	if len(token) == 0 {
		test.Errorf("expected non-empty token")
	}
}

func TestGenerateRandomToken_Unique(test *testing.T) {
	tokenOne, _ := GenerateRandomToken(32)
	tokenTwo, _ := GenerateRandomToken(32)
	if tokenOne == tokenTwo {
		test.Errorf("expected unique tokens, got identical: %s", tokenOne)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/oauth/ -run TestVerifyCodeChallenge -v`
Expected: FAIL — package does not exist yet

- [ ] **Step 3: Write minimal implementation**

```go
// internal/oauth/pkce.go
package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// VerifyCodeChallenge validates a PKCE code challenge against a code verifier
// using the S256 method as required by the MCP OAuth spec.
func VerifyCodeChallenge(challenge string, verifier string, method string) bool {
	if method != "S256" {
		return false
	}
	hash := sha256.Sum256([]byte(verifier))
	expected := base64.RawURLEncoding.EncodeToString(hash[:])
	return challenge == expected
}

// GenerateRandomToken produces a cryptographically random hex-encoded token
// of the specified byte length.
func GenerateRandomToken(byteLength int) (string, error) {
	randomBytes := make([]byte, byteLength)
	_, readError := rand.Read(randomBytes)
	if readError != nil {
		return "", readError
	}
	return hex.EncodeToString(randomBytes), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/oauth/ -run "TestVerifyCodeChallenge|TestGenerateRandomToken" -v`
Expected: PASS (all 5 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/oauth/pkce.go internal/oauth/pkce_test.go
git commit -m "Add PKCE verification and random token generation"
```

---

### Task 2: In-Memory OAuth Store

**Files:**
- Create: `internal/oauth/store.go`
- Test: `internal/oauth/store_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/oauth/store_test.go
package oauth

import (
	"testing"
	"time"
)

func TestStore_RegisterAndLookupClient(test *testing.T) {
	store := NewStore()

	client := &Client{
		ClientID:     "test-client-id",
		ClientName:   "Test Client",
		RedirectURIs: []string{"https://example.com/callback"},
	}
	store.RegisterClient(client)

	found := store.GetClient("test-client-id")
	if found == nil {
		test.Fatal("expected client to be found")
	}
	if found.ClientName != "Test Client" {
		test.Errorf("expected client name 'Test Client', got '%s'", found.ClientName)
	}
}

func TestStore_GetClient_NotFound(test *testing.T) {
	store := NewStore()
	found := store.GetClient("nonexistent")
	if found != nil {
		test.Errorf("expected nil for nonexistent client")
	}
}

func TestStore_StoreAndConsumeAuthCode(test *testing.T) {
	store := NewStore()

	authCode := &AuthCode{
		Code:          "test-code",
		ClientID:      "client-1",
		RedirectURI:   "https://example.com/callback",
		CodeChallenge: "challenge-value",
		ExpiresAt:     time.Now().Add(10 * time.Minute),
	}
	store.StoreAuthCode(authCode)

	found := store.ConsumeAuthCode("test-code")
	if found == nil {
		test.Fatal("expected auth code to be found")
	}

	consumedAgain := store.ConsumeAuthCode("test-code")
	if consumedAgain != nil {
		test.Errorf("expected auth code to be single-use, got non-nil on second consume")
	}
}

func TestStore_ConsumeAuthCode_Expired(test *testing.T) {
	store := NewStore()

	authCode := &AuthCode{
		Code:      "expired-code",
		ExpiresAt: time.Now().Add(-1 * time.Minute),
	}
	store.StoreAuthCode(authCode)

	found := store.ConsumeAuthCode("expired-code")
	if found != nil {
		test.Errorf("expected expired auth code to return nil")
	}
}

func TestStore_StoreAndValidateAccessToken(test *testing.T) {
	store := NewStore()

	tokenData := &TokenData{
		Token:     "access-token-123",
		ClientID:  "client-1",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	store.StoreAccessToken(tokenData)

	valid := store.ValidateAccessToken("access-token-123")
	if !valid {
		test.Errorf("expected valid access token")
	}
}

func TestStore_ValidateAccessToken_Expired(test *testing.T) {
	store := NewStore()

	tokenData := &TokenData{
		Token:     "expired-token",
		ExpiresAt: time.Now().Add(-1 * time.Minute),
	}
	store.StoreAccessToken(tokenData)

	valid := store.ValidateAccessToken("expired-token")
	if valid {
		test.Errorf("expected expired token to be invalid")
	}
}

func TestStore_StoreAndConsumeRefreshToken(test *testing.T) {
	store := NewStore()

	refreshData := &RefreshData{
		Token:     "refresh-token-123",
		ClientID:  "client-1",
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
	}
	store.StoreRefreshToken(refreshData)

	found := store.ConsumeRefreshToken("refresh-token-123")
	if found == nil {
		test.Fatal("expected refresh token to be found")
	}

	consumedAgain := store.ConsumeRefreshToken("refresh-token-123")
	if consumedAgain != nil {
		test.Errorf("expected refresh token to be single-use after rotation")
	}
}

func TestStore_StoreAndGetProviderSession(test *testing.T) {
	store := NewStore()

	session := &ProviderSession{
		State:         "state-abc",
		ClientID:      "client-1",
		RedirectURI:   "https://example.com/callback",
		CodeChallenge: "challenge",
		OriginalState: "original-state",
		ExpiresAt:     time.Now().Add(10 * time.Minute),
	}
	store.StoreProviderSession(session)

	found := store.GetProviderSession("state-abc")
	if found == nil {
		test.Fatal("expected provider session to be found")
	}
	if found.ClientID != "client-1" {
		test.Errorf("expected client ID 'client-1', got '%s'", found.ClientID)
	}
}

func TestStore_CleanupExpired(test *testing.T) {
	store := NewStore()

	store.StoreAccessToken(&TokenData{
		Token:     "expired-access",
		ExpiresAt: time.Now().Add(-1 * time.Minute),
	})
	store.StoreAccessToken(&TokenData{
		Token:     "valid-access",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	})

	store.CleanupExpired()

	if store.ValidateAccessToken("expired-access") {
		test.Errorf("expected expired token to be cleaned up")
	}
	if !store.ValidateAccessToken("valid-access") {
		test.Errorf("expected valid token to survive cleanup")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/oauth/ -run TestStore -v`
Expected: FAIL — types and functions not defined

- [ ] **Step 3: Write minimal implementation**

```go
// internal/oauth/store.go
package oauth

import (
	"sync"
	"time"
)

// Client represents a dynamically registered OAuth client.
type Client struct {
	ClientID     string   `json:"client_id"`
	ClientName   string   `json:"client_name"`
	RedirectURIs []string `json:"redirect_uris"`
}

// AuthCode represents an authorization code issued after provider authentication.
type AuthCode struct {
	Code          string
	ClientID      string
	RedirectURI   string
	CodeChallenge string
	ExpiresAt     time.Time
}

// TokenData represents an issued access token.
type TokenData struct {
	Token     string
	ClientID  string
	ExpiresAt time.Time
}

// RefreshData represents an issued refresh token.
type RefreshData struct {
	Token     string
	ClientID  string
	ExpiresAt time.Time
}

// ProviderSession tracks state during the OAuth provider redirect flow.
type ProviderSession struct {
	State         string
	ClientID      string
	RedirectURI   string
	CodeChallenge string
	OriginalState string
	Provider      string
	ExpiresAt     time.Time
}

// Store holds all OAuth state in memory. All methods are thread-safe.
type Store struct {
	mutex            sync.RWMutex
	clients          map[string]*Client
	authCodes        map[string]*AuthCode
	accessTokens     map[string]*TokenData
	refreshTokens    map[string]*RefreshData
	providerSessions map[string]*ProviderSession
}

// NewStore creates an empty OAuth store.
func NewStore() *Store {
	return &Store{
		clients:          make(map[string]*Client),
		authCodes:        make(map[string]*AuthCode),
		accessTokens:     make(map[string]*TokenData),
		refreshTokens:    make(map[string]*RefreshData),
		providerSessions: make(map[string]*ProviderSession),
	}
}

// RegisterClient adds a client to the store.
func (store *Store) RegisterClient(client *Client) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.clients[client.ClientID] = client
}

// GetClient retrieves a client by ID, or nil if not found.
func (store *Store) GetClient(clientID string) *Client {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	return store.clients[clientID]
}

// StoreAuthCode saves an authorization code.
func (store *Store) StoreAuthCode(authCode *AuthCode) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.authCodes[authCode.Code] = authCode
}

// ConsumeAuthCode retrieves and deletes an auth code (single-use).
// Returns nil if not found or expired.
func (store *Store) ConsumeAuthCode(code string) *AuthCode {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	authCode, exists := store.authCodes[code]
	if !exists {
		return nil
	}
	delete(store.authCodes, code)
	if time.Now().After(authCode.ExpiresAt) {
		return nil
	}
	return authCode
}

// StoreAccessToken saves an access token.
func (store *Store) StoreAccessToken(tokenData *TokenData) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.accessTokens[tokenData.Token] = tokenData
}

// ValidateAccessToken checks if a token exists and is not expired.
func (store *Store) ValidateAccessToken(token string) bool {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	tokenData, exists := store.accessTokens[token]
	if !exists {
		return false
	}
	return time.Now().Before(tokenData.ExpiresAt)
}

// StoreRefreshToken saves a refresh token.
func (store *Store) StoreRefreshToken(refreshData *RefreshData) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.refreshTokens[refreshData.Token] = refreshData
}

// ConsumeRefreshToken retrieves and deletes a refresh token (rotated on use).
// Returns nil if not found or expired.
func (store *Store) ConsumeRefreshToken(token string) *RefreshData {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	refreshData, exists := store.refreshTokens[token]
	if !exists {
		return nil
	}
	delete(store.refreshTokens, token)
	if time.Now().After(refreshData.ExpiresAt) {
		return nil
	}
	return refreshData
}

// StoreProviderSession saves a provider session keyed by state.
func (store *Store) StoreProviderSession(session *ProviderSession) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.providerSessions[session.State] = session
}

// GetProviderSession retrieves and deletes a provider session by state.
// Returns nil if not found or expired.
func (store *Store) GetProviderSession(state string) *ProviderSession {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	session, exists := store.providerSessions[state]
	if !exists {
		return nil
	}
	delete(store.providerSessions, state)
	if time.Now().After(session.ExpiresAt) {
		return nil
	}
	return session
}

// CleanupExpired removes all expired entries from the store.
func (store *Store) CleanupExpired() {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	now := time.Now()

	for code, authCode := range store.authCodes {
		if now.After(authCode.ExpiresAt) {
			delete(store.authCodes, code)
		}
	}
	for token, tokenData := range store.accessTokens {
		if now.After(tokenData.ExpiresAt) {
			delete(store.accessTokens, token)
		}
	}
	for token, refreshData := range store.refreshTokens {
		if now.After(refreshData.ExpiresAt) {
			delete(store.refreshTokens, token)
		}
	}
	for state, session := range store.providerSessions {
		if now.After(session.ExpiresAt) {
			delete(store.providerSessions, state)
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/oauth/ -run TestStore -v`
Expected: PASS (all 9 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/oauth/store.go internal/oauth/store_test.go
git commit -m "Add in-memory OAuth store with TTL and single-use semantics"
```

---

### Task 3: Provider Configuration

**Files:**
- Create: `internal/oauth/provider.go`
- Test: `internal/oauth/provider_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/oauth/provider_test.go
package oauth

import (
	"testing"
)

func TestNewProviderConfig_Google(test *testing.T) {
	provider := NewGoogleProvider("google-client-id", "google-secret")
	if provider.Name != "google" {
		test.Errorf("expected name 'google', got '%s'", provider.Name)
	}
	if provider.AuthURL != "https://accounts.google.com/o/oauth2/v2/auth" {
		test.Errorf("unexpected auth URL: %s", provider.AuthURL)
	}
	if provider.ClientID != "google-client-id" {
		test.Errorf("unexpected client ID: %s", provider.ClientID)
	}
}

func TestNewProviderConfig_GitHub(test *testing.T) {
	provider := NewGitHubProvider("github-client-id", "github-secret")
	if provider.Name != "github" {
		test.Errorf("expected name 'github', got '%s'", provider.Name)
	}
	if provider.AuthURL != "https://github.com/login/oauth/authorize" {
		test.Errorf("unexpected auth URL: %s", provider.AuthURL)
	}
}

func TestNewProviderConfig_Apple(test *testing.T) {
	provider := NewAppleProvider("apple-client-id", "apple-secret")
	if provider.Name != "apple" {
		test.Errorf("expected name 'apple', got '%s'", provider.Name)
	}
	if provider.AuthURL != "https://appleid.apple.com/auth/authorize" {
		test.Errorf("unexpected auth URL: %s", provider.AuthURL)
	}
}

func TestBuildActiveProviders_OnlyConfigured(test *testing.T) {
	providers := BuildActiveProviders("gid", "gsecret", "", "", "aid", "asecret")
	if len(providers) != 2 {
		test.Fatalf("expected 2 providers, got %d", len(providers))
	}
	if providers[0].Name != "google" {
		test.Errorf("expected first provider 'google', got '%s'", providers[0].Name)
	}
	if providers[1].Name != "apple" {
		test.Errorf("expected second provider 'apple', got '%s'", providers[1].Name)
	}
}

func TestBuildActiveProviders_NoneConfigured(test *testing.T) {
	providers := BuildActiveProviders("", "", "", "", "", "")
	if len(providers) != 0 {
		test.Errorf("expected 0 providers, got %d", len(providers))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/oauth/ -run TestNewProviderConfig -v`
Expected: FAIL — functions not defined

- [ ] **Step 3: Write minimal implementation**

```go
// internal/oauth/provider.go
package oauth

// ProviderConfig holds the OAuth configuration for a third-party identity provider.
type ProviderConfig struct {
	Name         string
	DisplayName  string
	AuthURL      string
	TokenURL     string
	UserInfoURL  string
	ClientID     string
	ClientSecret string
	Scopes       []string
}

// NewGoogleProvider creates a Google OAuth provider configuration.
func NewGoogleProvider(clientID string, clientSecret string) ProviderConfig {
	return ProviderConfig{
		Name:         "google",
		DisplayName:  "Google",
		AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:     "https://oauth2.googleapis.com/token",
		UserInfoURL:  "https://www.googleapis.com/oauth2/v2/userinfo",
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       []string{"openid", "email"},
	}
}

// NewGitHubProvider creates a GitHub OAuth provider configuration.
func NewGitHubProvider(clientID string, clientSecret string) ProviderConfig {
	return ProviderConfig{
		Name:         "github",
		DisplayName:  "GitHub",
		AuthURL:      "https://github.com/login/oauth/authorize",
		TokenURL:     "https://github.com/login/oauth/access_token",
		UserInfoURL:  "https://api.github.com/user",
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       []string{"read:user", "user:email"},
	}
}

// NewAppleProvider creates an Apple Sign In provider configuration.
func NewAppleProvider(clientID string, clientSecret string) ProviderConfig {
	return ProviderConfig{
		Name:         "apple",
		DisplayName:  "Apple",
		AuthURL:      "https://appleid.apple.com/auth/authorize",
		TokenURL:     "https://appleid.apple.com/auth/token",
		UserInfoURL:  "",
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       []string{"name", "email"},
	}
}

// BuildActiveProviders returns only providers where both client ID and secret are set.
func BuildActiveProviders(
	googleClientID string, googleClientSecret string,
	githubClientID string, githubClientSecret string,
	appleClientID string, appleClientSecret string,
) []ProviderConfig {
	var activeProviders []ProviderConfig

	if googleClientID != "" && googleClientSecret != "" {
		activeProviders = append(activeProviders, NewGoogleProvider(googleClientID, googleClientSecret))
	}
	if githubClientID != "" && githubClientSecret != "" {
		activeProviders = append(activeProviders, NewGitHubProvider(githubClientID, githubClientSecret))
	}
	if appleClientID != "" && appleClientSecret != "" {
		activeProviders = append(activeProviders, NewAppleProvider(appleClientID, appleClientSecret))
	}

	return activeProviders
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/oauth/ -run "TestNewProviderConfig|TestBuildActiveProviders" -v`
Expected: PASS (all 5 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/oauth/provider.go internal/oauth/provider_test.go
git commit -m "Add OAuth provider configurations for Google, GitHub, and Apple"
```

---

### Task 4: Authorization Server Metadata Endpoint

**Files:**
- Create: `internal/oauth/metadata.go`
- Test: `internal/oauth/metadata_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/oauth/metadata_test.go
package oauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMetadataHandler_ReturnsValidJSON(test *testing.T) {
	handler := NewMetadataHandler("https://banana.example.com:8847")

	request := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		test.Errorf("expected 200, got %d", recorder.Code)
	}

	contentType := recorder.Header().Get("Content-Type")
	if contentType != "application/json" {
		test.Errorf("expected Content-Type 'application/json', got '%s'", contentType)
	}

	var metadata map[string]interface{}
	decodeError := json.NewDecoder(recorder.Body).Decode(&metadata)
	if decodeError != nil {
		test.Fatalf("failed to decode JSON: %v", decodeError)
	}

	if metadata["issuer"] != "https://banana.example.com:8847" {
		test.Errorf("unexpected issuer: %v", metadata["issuer"])
	}
	if metadata["authorization_endpoint"] != "https://banana.example.com:8847/authorize" {
		test.Errorf("unexpected authorization_endpoint: %v", metadata["authorization_endpoint"])
	}
	if metadata["token_endpoint"] != "https://banana.example.com:8847/token" {
		test.Errorf("unexpected token_endpoint: %v", metadata["token_endpoint"])
	}
	if metadata["registration_endpoint"] != "https://banana.example.com:8847/register" {
		test.Errorf("unexpected registration_endpoint: %v", metadata["registration_endpoint"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/oauth/ -run TestMetadataHandler -v`
Expected: FAIL — NewMetadataHandler not defined

- [ ] **Step 3: Write minimal implementation**

```go
// internal/oauth/metadata.go
package oauth

import (
	"encoding/json"
	"net/http"
)

// ServerMetadata represents the OAuth 2.0 Authorization Server Metadata (RFC 8414).
type ServerMetadata struct {
	Issuer                           string   `json:"issuer"`
	AuthorizationEndpoint            string   `json:"authorization_endpoint"`
	TokenEndpoint                    string   `json:"token_endpoint"`
	RegistrationEndpoint             string   `json:"registration_endpoint"`
	ResponseTypesSupported           []string `json:"response_types_supported"`
	GrantTypesSupported              []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported    []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
}

// NewMetadataHandler returns an HTTP handler that serves the OAuth server metadata.
func NewMetadataHandler(baseURL string) http.Handler {
	metadata := ServerMetadata{
		Issuer:                           baseURL,
		AuthorizationEndpoint:            baseURL + "/authorize",
		TokenEndpoint:                    baseURL + "/token",
		RegistrationEndpoint:             baseURL + "/register",
		ResponseTypesSupported:           []string{"code"},
		GrantTypesSupported:              []string{"authorization_code", "refresh_token"},
		CodeChallengeMethodsSupported:    []string{"S256"},
		TokenEndpointAuthMethodsSupported: []string{"none"},
	}

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		json.NewEncoder(writer).Encode(metadata)
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/oauth/ -run TestMetadataHandler -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/oauth/metadata.go internal/oauth/metadata_test.go
git commit -m "Add OAuth authorization server metadata endpoint"
```

---

### Task 5: Dynamic Client Registration Endpoint

**Files:**
- Create: `internal/oauth/registration.go`
- Test: `internal/oauth/registration_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/oauth/registration_test.go
package oauth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegistrationHandler_Success(test *testing.T) {
	store := NewStore()
	handler := NewRegistrationHandler(store)

	body := `{
		"client_name": "Claude Desktop",
		"redirect_uris": ["https://claude.ai/api/mcp/auth_callback"],
		"grant_types": ["authorization_code", "refresh_token"],
		"response_types": ["code"],
		"token_endpoint_auth_method": "none"
	}`

	request := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		test.Errorf("expected 201, got %d", recorder.Code)
	}

	var response map[string]interface{}
	json.NewDecoder(recorder.Body).Decode(&response)

	clientID, hasClientID := response["client_id"]
	if !hasClientID || clientID == "" {
		test.Errorf("expected client_id in response")
	}
	if response["client_name"] != "Claude Desktop" {
		test.Errorf("expected client_name 'Claude Desktop', got '%v'", response["client_name"])
	}

	storedClient := store.GetClient(clientID.(string))
	if storedClient == nil {
		test.Errorf("expected client to be stored")
	}
}

func TestRegistrationHandler_MissingRedirectURIs(test *testing.T) {
	store := NewStore()
	handler := NewRegistrationHandler(store)

	body := `{"client_name": "Test"}`
	request := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		test.Errorf("expected 400, got %d", recorder.Code)
	}
}

func TestRegistrationHandler_InvalidJSON(test *testing.T) {
	store := NewStore()
	handler := NewRegistrationHandler(store)

	request := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBufferString("not json"))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		test.Errorf("expected 400, got %d", recorder.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/oauth/ -run TestRegistrationHandler -v`
Expected: FAIL — NewRegistrationHandler not defined

- [ ] **Step 3: Write minimal implementation**

```go
// internal/oauth/registration.go
package oauth

import (
	"encoding/json"
	"net/http"
)

// RegistrationRequest represents an OAuth dynamic client registration request (RFC 7591).
type RegistrationRequest struct {
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

// RegistrationResponse represents the response to a successful client registration.
type RegistrationResponse struct {
	ClientID                string   `json:"client_id"`
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

// NewRegistrationHandler returns an HTTP handler for dynamic client registration.
func NewRegistrationHandler(store *Store) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var registrationRequest RegistrationRequest
		decodeError := json.NewDecoder(request.Body).Decode(&registrationRequest)
		if decodeError != nil {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(writer).Encode(map[string]string{"error": "invalid_client_metadata"})
			return
		}

		if len(registrationRequest.RedirectURIs) == 0 {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(writer).Encode(map[string]string{"error": "invalid_client_metadata", "error_description": "redirect_uris required"})
			return
		}

		clientID, generateError := GenerateRandomToken(16)
		if generateError != nil {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(writer).Encode(map[string]string{"error": "server_error"})
			return
		}

		client := &Client{
			ClientID:     clientID,
			ClientName:   registrationRequest.ClientName,
			RedirectURIs: registrationRequest.RedirectURIs,
		}
		store.RegisterClient(client)

		response := RegistrationResponse{
			ClientID:                clientID,
			ClientName:              registrationRequest.ClientName,
			RedirectURIs:            registrationRequest.RedirectURIs,
			GrantTypes:              registrationRequest.GrantTypes,
			ResponseTypes:           registrationRequest.ResponseTypes,
			TokenEndpointAuthMethod: registrationRequest.TokenEndpointAuthMethod,
		}

		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		json.NewEncoder(writer).Encode(response)
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/oauth/ -run TestRegistrationHandler -v`
Expected: PASS (all 3 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/oauth/registration.go internal/oauth/registration_test.go
git commit -m "Add OAuth dynamic client registration endpoint"
```

---

### Task 6: Login Page and Authorization Endpoint

**Files:**
- Create: `internal/oauth/login.html`
- Create: `internal/oauth/handler.go`
- Test: `internal/oauth/handler_test.go`

- [ ] **Step 1: Create the embedded login page**

```html
<!-- internal/oauth/login.html -->
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Sign In — mcp-banana</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #1a1a2e; color: #e0e0e0; display: flex; justify-content: center; align-items: center; min-height: 100vh; }
  .container { background: #16213e; border-radius: 12px; padding: 40px; max-width: 400px; width: 90%; box-shadow: 0 8px 32px rgba(0,0,0,0.3); }
  h1 { font-size: 24px; margin-bottom: 8px; text-align: center; }
  .subtitle { color: #888; text-align: center; margin-bottom: 32px; font-size: 14px; }
  .btn { display: block; width: 100%; padding: 14px; border: 1px solid #333; border-radius: 8px; background: #0f3460; color: #e0e0e0; font-size: 16px; cursor: pointer; text-align: center; text-decoration: none; margin-bottom: 12px; transition: background 0.2s; }
  .btn:hover { background: #1a4a8a; }
  .btn-google { border-color: #4285f4; }
  .btn-github { border-color: #6e5494; }
  .btn-apple { border-color: #555; }
</style>
</head>
<body>
<div class="container">
  <h1>🍌 mcp-banana</h1>
  <p class="subtitle">Sign in to connect Claude Desktop</p>
  {{range .Providers}}
  <a class="btn btn-{{.Name}}" href="{{.AuthRedirectURL}}">Sign in with {{.DisplayName}}</a>
  {{end}}
</div>
</body>
</html>
```

- [ ] **Step 2: Write the failing test for the authorize handler**

```go
// internal/oauth/handler_test.go
package oauth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthorizeHandler_RendersLoginPage(test *testing.T) {
	store := NewStore()
	store.RegisterClient(&Client{
		ClientID:     "test-client",
		RedirectURIs: []string{"https://claude.ai/api/mcp/auth_callback"},
	})

	providers := []ProviderConfig{
		NewGoogleProvider("gid", "gsecret"),
	}
	handler := NewAuthorizeHandler(store, providers, "https://banana.example.com:8847")

	request := httptest.NewRequest(http.MethodGet, "/authorize?response_type=code&client_id=test-client&redirect_uri=https://claude.ai/api/mcp/auth_callback&state=abc&code_challenge=challenge123&code_challenge_method=S256", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		test.Errorf("expected 200, got %d", recorder.Code)
	}

	body := recorder.Body.String()
	if !containsSubstring(body, "Sign in with Google") {
		test.Errorf("expected login page to contain 'Sign in with Google'")
	}
	if !containsSubstring(body, "mcp-banana") {
		test.Errorf("expected login page to contain 'mcp-banana'")
	}
}

func TestAuthorizeHandler_InvalidClientID(test *testing.T) {
	store := NewStore()
	providers := []ProviderConfig{NewGoogleProvider("gid", "gsecret")}
	handler := NewAuthorizeHandler(store, providers, "https://banana.example.com:8847")

	request := httptest.NewRequest(http.MethodGet, "/authorize?response_type=code&client_id=nonexistent&redirect_uri=https://example.com/cb&state=abc&code_challenge=ch&code_challenge_method=S256", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		test.Errorf("expected 400 for invalid client_id, got %d", recorder.Code)
	}
}

func TestAuthorizeHandler_MissingCodeChallenge(test *testing.T) {
	store := NewStore()
	store.RegisterClient(&Client{
		ClientID:     "test-client",
		RedirectURIs: []string{"https://example.com/cb"},
	})
	providers := []ProviderConfig{NewGoogleProvider("gid", "gsecret")}
	handler := NewAuthorizeHandler(store, providers, "https://banana.example.com:8847")

	request := httptest.NewRequest(http.MethodGet, "/authorize?response_type=code&client_id=test-client&redirect_uri=https://example.com/cb&state=abc", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		test.Errorf("expected 400 for missing code_challenge, got %d", recorder.Code)
	}
}

func TestAuthorizeHandler_RedirectURIMismatch(test *testing.T) {
	store := NewStore()
	store.RegisterClient(&Client{
		ClientID:     "test-client",
		RedirectURIs: []string{"https://example.com/cb"},
	})
	providers := []ProviderConfig{NewGoogleProvider("gid", "gsecret")}
	handler := NewAuthorizeHandler(store, providers, "https://banana.example.com:8847")

	request := httptest.NewRequest(http.MethodGet, "/authorize?response_type=code&client_id=test-client&redirect_uri=https://evil.com/cb&state=abc&code_challenge=ch&code_challenge_method=S256", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		test.Errorf("expected 400 for redirect URI mismatch, got %d", recorder.Code)
	}
}

func containsSubstring(haystack string, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || len(needle) == 0 || findSubstring(haystack, needle))
}

func findSubstring(haystack string, needle string) bool {
	for position := 0; position <= len(haystack)-len(needle); position++ {
		if haystack[position:position+len(needle)] == needle {
			return true
		}
	}
	return false
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/oauth/ -run TestAuthorizeHandler -v`
Expected: FAIL — NewAuthorizeHandler not defined

- [ ] **Step 4: Write the implementation**

```go
// internal/oauth/handler.go
package oauth

import (
	"embed"
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"time"
)

//go:embed login.html
var loginPageFS embed.FS

// ProviderLink holds the data needed to render a provider button on the login page.
type ProviderLink struct {
	Name            string
	DisplayName     string
	AuthRedirectURL string
}

// NewAuthorizeHandler returns an HTTP handler for the OAuth authorization endpoint.
// It validates the request, stores a provider session, and renders the login page.
func NewAuthorizeHandler(store *Store, providers []ProviderConfig, baseURL string) http.Handler {
	loginTemplate, parseError := template.ParseFS(loginPageFS, "login.html")
	if parseError != nil {
		panic("failed to parse login template: " + parseError.Error())
	}

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		clientID := query.Get("client_id")
		redirectURI := query.Get("redirect_uri")
		state := query.Get("state")
		codeChallenge := query.Get("code_challenge")
		codeChallengeMethod := query.Get("code_challenge_method")

		if codeChallengeMethod != "S256" || codeChallenge == "" {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(writer).Encode(map[string]string{"error": "invalid_request", "error_description": "PKCE S256 code_challenge required"})
			return
		}

		client := store.GetClient(clientID)
		if client == nil {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(writer).Encode(map[string]string{"error": "invalid_request", "error_description": "unknown client_id"})
			return
		}

		redirectAllowed := false
		for _, allowedURI := range client.RedirectURIs {
			if allowedURI == redirectURI {
				redirectAllowed = true
				break
			}
		}
		if !redirectAllowed {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(writer).Encode(map[string]string{"error": "invalid_request", "error_description": "redirect_uri not registered"})
			return
		}

		var providerLinks []ProviderLink
		for _, provider := range providers {
			providerState, generateError := GenerateRandomToken(16)
			if generateError != nil {
				writer.WriteHeader(http.StatusInternalServerError)
				return
			}

			session := &ProviderSession{
				State:         providerState,
				ClientID:      clientID,
				RedirectURI:   redirectURI,
				CodeChallenge: codeChallenge,
				OriginalState: state,
				Provider:      provider.Name,
				ExpiresAt:     time.Now().Add(10 * time.Minute),
			}
			store.StoreProviderSession(session)

			params := url.Values{}
			params.Set("client_id", provider.ClientID)
			params.Set("redirect_uri", baseURL+"/callback")
			params.Set("response_type", "code")
			params.Set("state", providerState)
			for _, scope := range provider.Scopes {
				params.Add("scope", scope)
			}

			providerLinks = append(providerLinks, ProviderLink{
				Name:            provider.Name,
				DisplayName:     provider.DisplayName,
				AuthRedirectURL: provider.AuthURL + "?" + params.Encode(),
			})
		}

		data := struct {
			Providers []ProviderLink
		}{
			Providers: providerLinks,
		}

		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		loginTemplate.Execute(writer, data)
	})
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/oauth/ -run TestAuthorizeHandler -v`
Expected: PASS (all 4 tests)

- [ ] **Step 6: Commit**

```bash
git add internal/oauth/handler.go internal/oauth/handler_test.go internal/oauth/login.html
git commit -m "Add OAuth authorization endpoint with embedded login page"
```

---

### Task 7: Provider Callback and Token Endpoint

**Files:**
- Modify: `internal/oauth/handler.go`
- Test: `internal/oauth/handler_test.go` (append)

- [ ] **Step 1: Write failing tests for callback and token exchange**

Append to `internal/oauth/handler_test.go`:

```go
func TestTokenHandler_AuthorizationCodeExchange(test *testing.T) {
	store := NewStore()
	store.RegisterClient(&Client{
		ClientID:     "test-client",
		RedirectURIs: []string{"https://claude.ai/api/mcp/auth_callback"},
	})

	// Simulate a stored auth code with known PKCE challenge
	verifier := "test-verifier-string-that-is-long-enough"
	challenge := computeS256Challenge(verifier)

	store.StoreAuthCode(&AuthCode{
		Code:          "auth-code-123",
		ClientID:      "test-client",
		RedirectURI:   "https://claude.ai/api/mcp/auth_callback",
		CodeChallenge: challenge,
		ExpiresAt:     time.Now().Add(10 * time.Minute),
	})

	body := url.Values{}
	body.Set("grant_type", "authorization_code")
	body.Set("code", "auth-code-123")
	body.Set("client_id", "test-client")
	body.Set("redirect_uri", "https://claude.ai/api/mcp/auth_callback")
	body.Set("code_verifier", verifier)

	handler := NewTokenHandler(store)
	request := httptest.NewRequest(http.MethodPost, "/token", bytes.NewBufferString(body.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		test.Errorf("expected 200, got %d; body: %s", recorder.Code, recorder.Body.String())
	}

	var response map[string]interface{}
	json.NewDecoder(recorder.Body).Decode(&response)

	if response["access_token"] == nil || response["access_token"] == "" {
		test.Errorf("expected access_token in response")
	}
	if response["refresh_token"] == nil || response["refresh_token"] == "" {
		test.Errorf("expected refresh_token in response")
	}
	if response["token_type"] != "Bearer" {
		test.Errorf("expected token_type 'Bearer', got '%v'", response["token_type"])
	}
}

func TestTokenHandler_InvalidCode(test *testing.T) {
	store := NewStore()
	handler := NewTokenHandler(store)

	body := url.Values{}
	body.Set("grant_type", "authorization_code")
	body.Set("code", "nonexistent")
	body.Set("client_id", "test-client")
	body.Set("redirect_uri", "https://example.com/cb")
	body.Set("code_verifier", "verifier")

	request := httptest.NewRequest(http.MethodPost, "/token", bytes.NewBufferString(body.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		test.Errorf("expected 400, got %d", recorder.Code)
	}
}

func TestTokenHandler_InvalidPKCE(test *testing.T) {
	store := NewStore()
	store.StoreAuthCode(&AuthCode{
		Code:          "code-456",
		ClientID:      "test-client",
		RedirectURI:   "https://example.com/cb",
		CodeChallenge: "correct-challenge",
		ExpiresAt:     time.Now().Add(10 * time.Minute),
	})

	handler := NewTokenHandler(store)

	body := url.Values{}
	body.Set("grant_type", "authorization_code")
	body.Set("code", "code-456")
	body.Set("client_id", "test-client")
	body.Set("redirect_uri", "https://example.com/cb")
	body.Set("code_verifier", "wrong-verifier")

	request := httptest.NewRequest(http.MethodPost, "/token", bytes.NewBufferString(body.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		test.Errorf("expected 400 for invalid PKCE, got %d", recorder.Code)
	}
}

func TestTokenHandler_RefreshToken(test *testing.T) {
	store := NewStore()
	store.RegisterClient(&Client{
		ClientID:     "test-client",
		RedirectURIs: []string{"https://example.com/cb"},
	})
	store.StoreRefreshToken(&RefreshData{
		Token:     "refresh-abc",
		ClientID:  "test-client",
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
	})

	handler := NewTokenHandler(store)

	body := url.Values{}
	body.Set("grant_type", "refresh_token")
	body.Set("refresh_token", "refresh-abc")
	body.Set("client_id", "test-client")

	request := httptest.NewRequest(http.MethodPost, "/token", bytes.NewBufferString(body.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		test.Errorf("expected 200, got %d; body: %s", recorder.Code, recorder.Body.String())
	}

	var response map[string]interface{}
	json.NewDecoder(recorder.Body).Decode(&response)

	if response["access_token"] == nil || response["access_token"] == "" {
		test.Errorf("expected new access_token")
	}
}

// computeS256Challenge is a test helper for generating PKCE challenges.
func computeS256Challenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}
```

Add these imports to the top of handler_test.go:

```go
import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/oauth/ -run "TestTokenHandler" -v`
Expected: FAIL — NewTokenHandler not defined

- [ ] **Step 3: Write the token handler and callback handler**

Append to `internal/oauth/handler.go`:

```go
// NewCallbackHandler returns an HTTP handler for the OAuth provider callback.
// It receives the provider's auth code, exchanges it for a provider token,
// then issues an MCP auth code and redirects back to the MCP client.
func NewCallbackHandler(store *Store, providers []ProviderConfig, baseURL string) http.Handler {
	providersByName := make(map[string]ProviderConfig)
	for _, provider := range providers {
		providersByName[provider.Name] = provider
	}

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// Support both GET (Google/GitHub) and POST (Apple)
		var providerCode, providerState string
		if request.Method == http.MethodPost {
			request.ParseForm()
			providerCode = request.FormValue("code")
			providerState = request.FormValue("state")
		} else {
			providerCode = request.URL.Query().Get("code")
			providerState = request.URL.Query().Get("state")
		}

		session := store.GetProviderSession(providerState)
		if session == nil {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(writer).Encode(map[string]string{"error": "invalid_request", "error_description": "invalid or expired state"})
			return
		}

		provider, providerExists := providersByName[session.Provider]
		if !providerExists {
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Exchange provider code for provider token
		exchangeError := exchangeProviderCode(provider, providerCode, baseURL+"/callback")
		if exchangeError != nil {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(writer).Encode(map[string]string{"error": "server_error", "error_description": "provider token exchange failed"})
			return
		}

		// Generate MCP auth code
		mcpCode, generateError := GenerateRandomToken(32)
		if generateError != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}

		store.StoreAuthCode(&AuthCode{
			Code:          mcpCode,
			ClientID:      session.ClientID,
			RedirectURI:   session.RedirectURI,
			CodeChallenge: session.CodeChallenge,
			ExpiresAt:     time.Now().Add(10 * time.Minute),
		})

		// Redirect back to MCP client
		redirectURL, _ := url.Parse(session.RedirectURI)
		redirectParams := redirectURL.Query()
		redirectParams.Set("code", mcpCode)
		redirectParams.Set("state", session.OriginalState)
		redirectURL.RawQuery = redirectParams.Encode()

		http.Redirect(writer, request, redirectURL.String(), http.StatusFound)
	})
}

// exchangeProviderCode exchanges an authorization code with the provider's token endpoint.
// This is a package-level variable to allow test injection.
var exchangeProviderCode = func(provider ProviderConfig, code string, callbackURL string) error {
	tokenParams := url.Values{}
	tokenParams.Set("grant_type", "authorization_code")
	tokenParams.Set("code", code)
	tokenParams.Set("redirect_uri", callbackURL)
	tokenParams.Set("client_id", provider.ClientID)
	tokenParams.Set("client_secret", provider.ClientSecret)

	response, postError := http.PostForm(provider.TokenURL, tokenParams)
	if postError != nil {
		return postError
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("provider returned status %d", response.StatusCode)
	}
	return nil
}

// NewTokenHandler returns an HTTP handler for the OAuth token endpoint.
// It handles authorization_code and refresh_token grant types.
func NewTokenHandler(store *Store) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		request.ParseForm()
		grantType := request.FormValue("grant_type")

		switch grantType {
		case "authorization_code":
			handleAuthorizationCodeGrant(writer, request, store)
		case "refresh_token":
			handleRefreshTokenGrant(writer, request, store)
		default:
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(writer).Encode(map[string]string{"error": "unsupported_grant_type"})
		}
	})
}

func handleAuthorizationCodeGrant(writer http.ResponseWriter, request *http.Request, store *Store) {
	code := request.FormValue("code")
	clientID := request.FormValue("client_id")
	redirectURI := request.FormValue("redirect_uri")
	codeVerifier := request.FormValue("code_verifier")

	authCode := store.ConsumeAuthCode(code)
	if authCode == nil {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(writer).Encode(map[string]string{"error": "invalid_grant", "error_description": "invalid or expired code"})
		return
	}

	if authCode.ClientID != clientID || authCode.RedirectURI != redirectURI {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(writer).Encode(map[string]string{"error": "invalid_grant", "error_description": "client_id or redirect_uri mismatch"})
		return
	}

	if !VerifyCodeChallenge(authCode.CodeChallenge, codeVerifier, "S256") {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(writer).Encode(map[string]string{"error": "invalid_grant", "error_description": "PKCE verification failed"})
		return
	}

	issueTokenResponse(writer, store, clientID)
}

func handleRefreshTokenGrant(writer http.ResponseWriter, request *http.Request, store *Store) {
	refreshToken := request.FormValue("refresh_token")
	clientID := request.FormValue("client_id")

	refreshData := store.ConsumeRefreshToken(refreshToken)
	if refreshData == nil {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(writer).Encode(map[string]string{"error": "invalid_grant", "error_description": "invalid or expired refresh token"})
		return
	}

	if refreshData.ClientID != clientID {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(writer).Encode(map[string]string{"error": "invalid_grant", "error_description": "client_id mismatch"})
		return
	}

	issueTokenResponse(writer, store, clientID)
}

func issueTokenResponse(writer http.ResponseWriter, store *Store, clientID string) {
	accessToken, accessError := GenerateRandomToken(32)
	if accessError != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	refreshToken, refreshError := GenerateRandomToken(32)
	if refreshError != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	accessExpiresIn := 3600
	store.StoreAccessToken(&TokenData{
		Token:     accessToken,
		ClientID:  clientID,
		ExpiresAt: time.Now().Add(time.Duration(accessExpiresIn) * time.Second),
	})
	store.StoreRefreshToken(&RefreshData{
		Token:     refreshToken,
		ClientID:  clientID,
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
	})

	tokenResponse := map[string]interface{}{
		"access_token":  accessToken,
		"token_type":    "Bearer",
		"expires_in":    accessExpiresIn,
		"refresh_token": refreshToken,
	}

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(tokenResponse)
}
```

Add `"fmt"` to the imports in handler.go.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/oauth/ -run "TestTokenHandler" -v`
Expected: PASS (all 4 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/oauth/handler.go internal/oauth/handler_test.go
git commit -m "Add OAuth callback and token exchange endpoints"
```

---

### Task 8: Config Changes for OAuth and TLS

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

Add to the existing config test file:

```go
func TestLoad_OAuthConfigFields(test *testing.T) {
	test.Setenv("GEMINI_API_KEY", "AIzaTestKeyForOAuthConfigTest12345678901")
	test.Setenv("OAUTH_GOOGLE_CLIENT_ID", "google-id")
	test.Setenv("OAUTH_GOOGLE_CLIENT_SECRET", "google-secret")
	test.Setenv("OAUTH_BASE_URL", "https://banana.example.com:8847")

	cfg, loadError := Load()
	if loadError != nil {
		test.Fatalf("unexpected error: %v", loadError)
	}
	if cfg.OAuthGoogleClientID != "google-id" {
		test.Errorf("expected OAuthGoogleClientID 'google-id', got '%s'", cfg.OAuthGoogleClientID)
	}
	if cfg.OAuthBaseURL != "https://banana.example.com:8847" {
		test.Errorf("expected OAuthBaseURL, got '%s'", cfg.OAuthBaseURL)
	}
}

func TestLoad_TLSConfigFields(test *testing.T) {
	test.Setenv("GEMINI_API_KEY", "AIzaTestKeyForTLSConfigTest123456789012")
	test.Setenv("MCP_TLS_CERT_FILE", "/certs/cert.pem")
	test.Setenv("MCP_TLS_KEY_FILE", "/certs/key.pem")

	cfg, loadError := Load()
	if loadError != nil {
		test.Fatalf("unexpected error: %v", loadError)
	}
	if cfg.TLSCertFile != "/certs/cert.pem" {
		test.Errorf("expected TLSCertFile '/certs/cert.pem', got '%s'", cfg.TLSCertFile)
	}
	if cfg.TLSKeyFile != "/certs/key.pem" {
		test.Errorf("expected TLSKeyFile '/certs/key.pem', got '%s'", cfg.TLSKeyFile)
	}
}

func TestLoad_TLSPartialConfig_ReturnsError(test *testing.T) {
	test.Setenv("GEMINI_API_KEY", "AIzaTestKeyForTLSPartialTest12345678901")
	test.Setenv("MCP_TLS_CERT_FILE", "/certs/cert.pem")
	// MCP_TLS_KEY_FILE intentionally missing

	_, loadError := Load()
	if loadError == nil {
		test.Errorf("expected error when only one TLS file is set")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run "TestLoad_OAuth|TestLoad_TLS" -v`
Expected: FAIL — fields don't exist on Config struct

- [ ] **Step 3: Add new fields to Config struct and Load function**

Add these fields to the `Config` struct in `internal/config/config.go`:

```go
// OAuth provider credentials
OAuthGoogleClientID     string
OAuthGoogleClientSecret string
OAuthGitHubClientID     string
OAuthGitHubClientSecret string
OAuthAppleClientID      string
OAuthAppleClientSecret  string
OAuthBaseURL            string

// TLS configuration
TLSCertFile string
TLSKeyFile  string
```

Add to the `Load()` function before the return statement:

```go
// OAuth provider configuration (all optional)
cfg.OAuthGoogleClientID = os.Getenv("OAUTH_GOOGLE_CLIENT_ID")
cfg.OAuthGoogleClientSecret = os.Getenv("OAUTH_GOOGLE_CLIENT_SECRET")
cfg.OAuthGitHubClientID = os.Getenv("OAUTH_GITHUB_CLIENT_ID")
cfg.OAuthGitHubClientSecret = os.Getenv("OAUTH_GITHUB_CLIENT_SECRET")
cfg.OAuthAppleClientID = os.Getenv("OAUTH_APPLE_CLIENT_ID")
cfg.OAuthAppleClientSecret = os.Getenv("OAUTH_APPLE_CLIENT_SECRET")
cfg.OAuthBaseURL = os.Getenv("OAUTH_BASE_URL")

// TLS configuration (both required together, or neither)
cfg.TLSCertFile = os.Getenv("MCP_TLS_CERT_FILE")
cfg.TLSKeyFile = os.Getenv("MCP_TLS_KEY_FILE")
if (cfg.TLSCertFile != "" && cfg.TLSKeyFile == "") || (cfg.TLSCertFile == "" && cfg.TLSKeyFile != "") {
    return nil, fmt.Errorf("both MCP_TLS_CERT_FILE and MCP_TLS_KEY_FILE must be set together")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run "TestLoad_OAuth|TestLoad_TLS" -v`
Expected: PASS (all 3 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "Add OAuth and TLS configuration to config package"
```

---

### Task 9: Register OAuth Routes and Integrate with Server

**Files:**
- Modify: `internal/server/server.go`
- Modify: `internal/server/middleware.go`
- Modify: `internal/security/sanitize.go`
- Modify: `cmd/mcp-banana/main.go`

- [ ] **Step 1: Modify server.go to register OAuth routes**

In `internal/server/server.go`, update `NewHTTPHandler` to accept an optional `*oauth.Store` and providers, then register OAuth routes:

```go
// Add import: "github.com/reshinto/mcp-banana/internal/oauth"

// Update NewHTTPHandler signature:
func NewHTTPHandler(mcpSrv *mcpserver.MCPServer, serverConfig *config.Config, logger *slog.Logger, oauthStore *oauth.Store, providers []oauth.ProviderConfig) http.Handler {

// Add OAuth route registration inside NewHTTPHandler, before the middleware wrap:
if oauthStore != nil && serverConfig.OAuthBaseURL != "" {
    mux.Handle("/.well-known/oauth-authorization-server", oauth.NewMetadataHandler(serverConfig.OAuthBaseURL))
    mux.Handle("/register", oauth.NewRegistrationHandler(oauthStore))
    mux.Handle("/authorize", oauth.NewAuthorizeHandler(oauthStore, providers, serverConfig.OAuthBaseURL))
    mux.Handle("/callback", oauth.NewCallbackHandler(oauthStore, providers, serverConfig.OAuthBaseURL))
    mux.Handle("/token", oauth.NewTokenHandler(oauthStore))
}
```

- [ ] **Step 2: Modify middleware.go to validate OAuth tokens**

In `internal/server/middleware.go`, update `authenticateRequest` to accept the OAuth store and check it as a fallback:

```go
// Update the middleware struct to include oauthStore:
type middleware struct {
    // ... existing fields ...
    oauthStore *oauth.Store
}

// Update newMiddleware to accept oauthStore:
func newMiddleware(cfg *config.Config, logger *slog.Logger, oauthStore *oauth.Store) *middleware {
    // ... existing code ...
    return &middleware{
        // ... existing fields ...
        oauthStore: oauthStore,
    }
}

// Update authenticateRequest to check OAuth tokens:
func (mw *middleware) authenticateRequest(request *http.Request) bool {
    // ... existing bearer token check ...

    // If static token check fails and OAuth store exists, check OAuth tokens
    if mw.oauthStore != nil && mw.oauthStore.ValidateAccessToken(token) {
        return true
    }

    return false
}
```

Update `WrapWithMiddleware` and `WrapHTTP` signatures to pass through the OAuth store.

- [ ] **Step 3: Register provider secrets in main.go**

In `cmd/mcp-banana/main.go`, after existing `security.RegisterSecret` calls:

```go
// Register OAuth provider secrets
security.RegisterSecret(serverConfig.OAuthGoogleClientSecret)
security.RegisterSecret(serverConfig.OAuthGitHubClientSecret)
security.RegisterSecret(serverConfig.OAuthAppleClientSecret)

// Build OAuth store and providers
var oauthStore *oauth.Store
providers := oauth.BuildActiveProviders(
    serverConfig.OAuthGoogleClientID, serverConfig.OAuthGoogleClientSecret,
    serverConfig.OAuthGitHubClientID, serverConfig.OAuthGitHubClientSecret,
    serverConfig.OAuthAppleClientID, serverConfig.OAuthAppleClientSecret,
)
if len(providers) > 0 {
    oauthStore = oauth.NewStore()
    // Start background cleanup goroutine
    go func() {
        ticker := time.NewTicker(5 * time.Minute)
        defer ticker.Stop()
        for range ticker.C {
            oauthStore.CleanupExpired()
        }
    }()
}
```

- [ ] **Step 4: Add TLS support in main.go**

In `runHTTPServer`, update the server start logic:

```go
// Replace httpServer.Serve(listener) with:
if serverConfig.TLSCertFile != "" && serverConfig.TLSKeyFile != "" {
    logger.Info("starting HTTPS server", "address", address)
    serveError = httpServer.ServeTLS(listener, serverConfig.TLSCertFile, serverConfig.TLSKeyFile)
} else {
    logger.Info("starting HTTP server", "address", address)
    serveError = httpServer.Serve(listener)
}
```

- [ ] **Step 5: Run all tests**

Run: `go test ./... -v`
Expected: PASS (update any broken test signatures for the new parameters)

- [ ] **Step 6: Commit**

```bash
git add internal/server/server.go internal/server/middleware.go internal/security/sanitize.go cmd/mcp-banana/main.go
git commit -m "Integrate OAuth routes, token validation, and TLS into server"
```

---

### Task 10: Update .env.example and docker-compose.yml

**Files:**
- Modify: `.env.example`
- Modify: `docker-compose.yml`

- [ ] **Step 1: Add OAuth and TLS vars to .env.example**

Append to `.env.example`:

```bash
# =============================================================================
# OAuth Configuration (Optional — enables Claude Desktop integration)
# =============================================================================
# To enable OAuth, configure at least one provider below and set OAUTH_BASE_URL.
# Only providers with both client ID and secret are shown on the login page.

# Base URL for OAuth endpoints (must be HTTPS in production).
# Example: https://banana.yourdomain.com:8847
OAUTH_BASE_URL=

# Google OAuth credentials (https://console.cloud.google.com/apis/credentials)
OAUTH_GOOGLE_CLIENT_ID=
OAUTH_GOOGLE_CLIENT_SECRET=

# GitHub OAuth credentials (https://github.com/settings/developers)
OAUTH_GITHUB_CLIENT_ID=
OAUTH_GITHUB_CLIENT_SECRET=

# Apple Sign In credentials (https://developer.apple.com/account/resources/identifiers)
OAUTH_APPLE_CLIENT_ID=
OAUTH_APPLE_CLIENT_SECRET=

# =============================================================================
# TLS Configuration (Optional — required for OAuth in production)
# =============================================================================
# Both must be set together. When set, the server uses HTTPS.
# When not set, the server uses plain HTTP (for local development or behind a proxy).

# Path to TLS certificate file (PEM format)
MCP_TLS_CERT_FILE=

# Path to TLS private key file (PEM format)
MCP_TLS_KEY_FILE=
```

- [ ] **Step 2: Add TLS cert volume mount to docker-compose.yml**

Add a commented volume mount to `docker-compose.yml`:

```yaml
    # Uncomment to mount TLS certificates for HTTPS/OAuth support:
    # volumes:
    #   - /etc/letsencrypt/live/banana.yourdomain.com:/certs:ro
```

- [ ] **Step 3: Commit**

```bash
git add .env.example docker-compose.yml
git commit -m "Add OAuth and TLS configuration to .env.example and docker-compose"
```

---

### Task 11: Update All Documentation

**Files:**
- Modify: `README.md`
- Modify: `docs/claude-code-integration.md`
- Modify: `docs/architecture.md`
- Modify: `docs/authentication.md`
- Modify: `docs/setup-and-operations.md`
- Modify: `docs/security.md`
- Modify: `docs/troubleshooting.md`
- Modify: `docs/go-guide.md`
- Modify: `docs/root-files.md`
- Modify: `CONTRIBUTING.md`

- [ ] **Step 1: Update README.md**

Add a "Claude Desktop" section after the existing Quick Start:

```markdown
## Claude Desktop Integration

mcp-banana supports OAuth 2.1 for connecting via Claude Desktop GUI.

**Prerequisites:** A subdomain (e.g., `banana.yourdomain.com`) with a TLS certificate, and OAuth credentials from at least one provider (Google, GitHub, or Apple).

1. Configure OAuth provider credentials and TLS in `.env`
2. Start the server: `docker compose up -d`
3. In Claude Desktop, go to **Customize > Connectors**
4. Add your server URL: `https://banana.yourdomain.com:8847/mcp`
5. Sign in with your chosen provider when prompted

See [Claude Code Integration](docs/claude-code-integration.md) for detailed setup.
```

Update the Overview paragraph to mention OAuth:

```markdown
mcp-banana implements the Model Context Protocol (MCP) to expose four image generation tools to Claude Code and Claude Desktop. It runs locally as a stdio subprocess or remotely as an HTTP server with bearer token or OAuth 2.1 authentication.
```

- [ ] **Step 2: Update docs/claude-code-integration.md**

Add a new section "Option C: Claude Desktop GUI (OAuth)" with:
- Prerequisites (subdomain, TLS cert, OAuth provider credentials)
- Step-by-step setup (DNS, cert, .env config, docker restart)
- How to add the connector in Claude Desktop

- [ ] **Step 3: Update docs/architecture.md**

Add `internal/oauth/` to the package layout diagram and describe its responsibility.

- [ ] **Step 4: Update docs/authentication.md**

Add OAuth as a third authentication option alongside bearer token and SSH tunnel. Document the OAuth flow, token TTLs, and provider configuration.

- [ ] **Step 5: Update docs/setup-and-operations.md**

Add sections for:
- Subdomain DNS setup
- TLS certificate generation (certbot with DNS-01 challenge)
- OAuth provider credential setup (Google Console, GitHub Developer Settings, Apple Developer)
- Docker volume mount for certificates

- [ ] **Step 6: Update docs/security.md**

Add:
- OAuth threat model (PKCE, state validation, token TTLs)
- Provider secret sanitization
- HTTPS requirement rationale

- [ ] **Step 7: Update docs/troubleshooting.md**

Add entries for:
- "OAuth login page shows no providers" → check provider env vars
- "TLS handshake failed" → verify cert/key paths and domain
- "401 after OAuth login" → token may have expired, re-authenticate
- "Claude Desktop can't connect" → verify HTTPS and OAuth base URL

- [ ] **Step 8: Update docs/go-guide.md**

Add sections explaining:
- `embed` package for embedding static files (login.html)
- `crypto/rand` for secure random token generation
- `sync.RWMutex` for concurrent map access in the store
- `net/http` ListenAndServeTLS for built-in TLS
- `html/template` for server-side HTML rendering

- [ ] **Step 9: Update docs/root-files.md and CONTRIBUTING.md**

- `root-files.md`: no new root files added (OAuth code is in `internal/oauth/`)
- `CONTRIBUTING.md`: add note about setting up OAuth test credentials for local development

- [ ] **Step 10: Commit**

```bash
git add README.md docs/ CONTRIBUTING.md
git commit -m "Update all documentation for OAuth and TLS support"
```

---

### Task 12: Run Full CI Quality Gate

**Files:** None (validation only)

- [ ] **Step 1: Run linter**

Run: `golangci-lint run`
Expected: No errors

- [ ] **Step 2: Run formatter**

Run: `gofmt -w .`
Expected: No changes (code already formatted)

- [ ] **Step 3: Run vet**

Run: `go vet ./...`
Expected: No errors

- [ ] **Step 4: Run tests with race detector**

Run: `go test -race -coverprofile=coverage.out ./...`
Expected: All tests pass, coverage meets 80% threshold

- [ ] **Step 5: Fix any issues and re-run until green**

Iterate on any failures until all 4 steps pass cleanly.

- [ ] **Step 6: Final commit if any fixes were needed**

```bash
git add -A
git commit -m "Fix CI quality gate issues"
```
