# Unified Credentials File Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace separate auth token and Gemini API key configuration with a single hot-reloadable JSON credentials file (`MCP_CREDENTIALS_FILE`) that maps client identities to Gemini API keys.

**Architecture:** A new `internal/credentials/` package manages the credentials file (read, write, validate). The middleware resolves auth + Gemini key in one pass using the credentials file. The OAuth flow adds a Gemini key prompt step. Tool handlers receive the resolved Gemini key via context instead of resolving it themselves.

**Tech Stack:** Go 1.24, encoding/json, os, sync, google.golang.org/genai (for key validation)

---

### Task 1: Create the Credentials Store Package

**Files:**
- Create: `internal/credentials/store.go`
- Create: `internal/credentials/store_test.go`

This package handles all credentials file I/O: reading, writing, atomic saves, and auto-creation.

- [ ] **Step 1: Write the failing test for NewStore and Load**

```go
// Package credentials manages the unified credentials file that maps client
// identities (bearer tokens or OAuth provider:email pairs) to Gemini API keys.
package credentials

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewStore_CreatesFileIfMissing(test *testing.T) {
	filePath := filepath.Join(test.TempDir(), "creds.json")
	store, storeError := NewStore(filePath)
	if storeError != nil {
		test.Fatalf("unexpected error: %s", storeError)
	}
	if store == nil {
		test.Fatal("expected non-nil store")
	}
	// File should exist with empty JSON object
	data, readError := os.ReadFile(filePath)
	if readError != nil {
		test.Fatalf("file should exist: %s", readError)
	}
	if string(data) != "{}" {
		test.Errorf("expected empty JSON object, got: %s", string(data))
	}
}

func TestNewStore_LoadsExistingFile(test *testing.T) {
	filePath := filepath.Join(test.TempDir(), "creds.json")
	writeError := os.WriteFile(filePath, []byte(`{"mytoken":"AIzaFakeKey123"}`), 0600)
	if writeError != nil {
		test.Fatalf("setup failed: %s", writeError)
	}
	store, storeError := NewStore(filePath)
	if storeError != nil {
		test.Fatalf("unexpected error: %s", storeError)
	}
	geminiKey := store.Lookup("mytoken")
	if geminiKey != "AIzaFakeKey123" {
		test.Errorf("expected AIzaFakeKey123, got: %s", geminiKey)
	}
}

func TestNewStore_HandlesEmptyFile(test *testing.T) {
	filePath := filepath.Join(test.TempDir(), "creds.json")
	writeError := os.WriteFile(filePath, []byte(`{}`), 0600)
	if writeError != nil {
		test.Fatalf("setup failed: %s", writeError)
	}
	store, storeError := NewStore(filePath)
	if storeError != nil {
		test.Fatalf("unexpected error: %s", storeError)
	}
	geminiKey := store.Lookup("nonexistent")
	if geminiKey != "" {
		test.Errorf("expected empty string for missing key, got: %s", geminiKey)
	}
}

func TestNewStore_RejectsMalformedJSON(test *testing.T) {
	filePath := filepath.Join(test.TempDir(), "creds.json")
	writeError := os.WriteFile(filePath, []byte(`not json`), 0600)
	if writeError != nil {
		test.Fatalf("setup failed: %s", writeError)
	}
	_, storeError := NewStore(filePath)
	if storeError == nil {
		test.Fatal("expected error for malformed JSON")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/credentials/ -run TestNewStore -v`
Expected: FAIL — package does not exist

- [ ] **Step 3: Write Store struct with NewStore and Lookup**

```go
// Package credentials manages the unified credentials file that maps client
// identities (bearer tokens or OAuth provider:email pairs) to Gemini API keys.
// The file is re-read on every lookup for hot-reload support.
package credentials

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// Store manages the credentials file at the given path.
// It provides thread-safe read and write access to the identity-to-key mapping.
type Store struct {
	filePath string
	mutex    sync.Mutex
}

// NewStore creates a Store for the given file path. If the file does not exist,
// it is created with an empty JSON object. If the file exists, it is validated
// as parseable JSON.
func NewStore(filePath string) (*Store, error) {
	store := &Store{filePath: filePath}

	if _, statError := os.Stat(filePath); os.IsNotExist(statError) {
		if writeError := os.WriteFile(filePath, []byte("{}"), 0600); writeError != nil {
			return nil, fmt.Errorf("failed to create credentials file: %w", writeError)
		}
		return store, nil
	}

	// Validate existing file is parseable JSON
	data, readError := os.ReadFile(filePath)
	if readError != nil {
		return nil, fmt.Errorf("failed to read credentials file: %w", readError)
	}
	var entries map[string]string
	if parseError := json.Unmarshal(data, &entries); parseError != nil {
		return nil, fmt.Errorf("credentials file contains invalid JSON: %w", parseError)
	}

	return store, nil
}

// Lookup reads the credentials file and returns the Gemini API key for the
// given identity. Returns an empty string if the identity is not found or the
// file cannot be read. The file is re-read on every call for hot-reload.
func (store *Store) Lookup(identity string) string {
	data, readError := os.ReadFile(store.filePath)
	if readError != nil {
		return ""
	}
	var entries map[string]string
	if parseError := json.Unmarshal(data, &entries); parseError != nil {
		return ""
	}
	return entries[identity]
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/credentials/ -run TestNewStore -v`
Expected: PASS (all 4 tests)

- [ ] **Step 5: Write failing tests for Register (write/overwrite)**

```go
func TestRegister_AddsNewEntry(test *testing.T) {
	filePath := filepath.Join(test.TempDir(), "creds.json")
	store, _ := NewStore(filePath)

	registerError := store.Register("newtoken", "AIzaNewKey456")
	if registerError != nil {
		test.Fatalf("unexpected error: %s", registerError)
	}

	geminiKey := store.Lookup("newtoken")
	if geminiKey != "AIzaNewKey456" {
		test.Errorf("expected AIzaNewKey456, got: %s", geminiKey)
	}
}

func TestRegister_OverwritesExistingEntry(test *testing.T) {
	filePath := filepath.Join(test.TempDir(), "creds.json")
	writeError := os.WriteFile(filePath, []byte(`{"existingtoken":"AIzaOldKey"}`), 0600)
	if writeError != nil {
		test.Fatalf("setup failed: %s", writeError)
	}
	store, _ := NewStore(filePath)

	registerError := store.Register("existingtoken", "AIzaUpdatedKey")
	if registerError != nil {
		test.Fatalf("unexpected error: %s", registerError)
	}

	geminiKey := store.Lookup("existingtoken")
	if geminiKey != "AIzaUpdatedKey" {
		test.Errorf("expected AIzaUpdatedKey, got: %s", geminiKey)
	}
}

func TestRegister_PreservesOtherEntries(test *testing.T) {
	filePath := filepath.Join(test.TempDir(), "creds.json")
	writeError := os.WriteFile(filePath, []byte(`{"alice":"AIzaAlice","bob":"AIzaBob"}`), 0600)
	if writeError != nil {
		test.Fatalf("setup failed: %s", writeError)
	}
	store, _ := NewStore(filePath)

	registerError := store.Register("alice", "AIzaAliceNew")
	if registerError != nil {
		test.Fatalf("unexpected error: %s", registerError)
	}

	aliceKey := store.Lookup("alice")
	bobKey := store.Lookup("bob")
	if aliceKey != "AIzaAliceNew" {
		test.Errorf("expected AIzaAliceNew, got: %s", aliceKey)
	}
	if bobKey != "AIzaBob" {
		test.Errorf("expected AIzaBob, got: %s", bobKey)
	}
}

func TestRegister_FilePermissions(test *testing.T) {
	filePath := filepath.Join(test.TempDir(), "creds.json")
	store, _ := NewStore(filePath)

	store.Register("token", "AIzaKey") //nolint:errcheck

	fileInfo, statError := os.Stat(filePath)
	if statError != nil {
		test.Fatalf("stat failed: %s", statError)
	}
	// File should have 0600 permissions (owner read/write only)
	if fileInfo.Mode().Perm() != 0600 {
		test.Errorf("expected 0600 permissions, got: %o", fileInfo.Mode().Perm())
	}
}
```

- [ ] **Step 6: Run tests to verify they fail**

Run: `go test ./internal/credentials/ -run TestRegister -v`
Expected: FAIL — Register method does not exist

- [ ] **Step 7: Write the Register method with atomic writes**

Add to `internal/credentials/store.go`:

```go
// Register writes or overwrites an identity-to-key mapping in the credentials file.
// The write is atomic (write to temp file, rename) to prevent corruption from
// concurrent requests. The file is written with 0600 permissions.
func (store *Store) Register(identity string, geminiAPIKey string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	// Read current state
	entries := make(map[string]string)
	data, readError := os.ReadFile(store.filePath)
	if readError == nil {
		json.Unmarshal(data, &entries) //nolint:errcheck
	}

	entries[identity] = geminiAPIKey

	// Atomic write: temp file + rename
	serialized, marshalError := json.MarshalIndent(entries, "", "  ")
	if marshalError != nil {
		return fmt.Errorf("failed to serialize credentials: %w", marshalError)
	}

	tempPath := store.filePath + ".tmp"
	if writeError := os.WriteFile(tempPath, serialized, 0600); writeError != nil {
		return fmt.Errorf("failed to write temp credentials file: %w", writeError)
	}
	if renameError := os.Rename(tempPath, store.filePath); renameError != nil {
		os.Remove(tempPath) //nolint:errcheck
		return fmt.Errorf("failed to rename credentials file: %w", renameError)
	}

	return nil
}
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test ./internal/credentials/ -run TestRegister -v`
Expected: PASS (all 4 tests)

- [ ] **Step 9: Write failing test for Exists**

```go
func TestExists_ReturnsTrueForKnownIdentity(test *testing.T) {
	filePath := filepath.Join(test.TempDir(), "creds.json")
	os.WriteFile(filePath, []byte(`{"mytoken":"AIzaKey"}`), 0600) //nolint:errcheck
	store, _ := NewStore(filePath)

	if !store.Exists("mytoken") {
		test.Error("expected true for existing identity")
	}
}

func TestExists_ReturnsFalseForUnknownIdentity(test *testing.T) {
	filePath := filepath.Join(test.TempDir(), "creds.json")
	store, _ := NewStore(filePath)

	if store.Exists("nonexistent") {
		test.Error("expected false for missing identity")
	}
}
```

- [ ] **Step 10: Run tests to verify they fail**

Run: `go test ./internal/credentials/ -run TestExists -v`
Expected: FAIL — Exists method does not exist

- [ ] **Step 11: Write the Exists method**

Add to `internal/credentials/store.go`:

```go
// Exists returns true if the given identity has an entry in the credentials file.
func (store *Store) Exists(identity string) bool {
	return store.Lookup(identity) != ""
}
```

- [ ] **Step 12: Run all credentials tests**

Run: `go test ./internal/credentials/ -v`
Expected: PASS (all tests)

- [ ] **Step 13: Commit**

```bash
git add internal/credentials/store.go internal/credentials/store_test.go
git commit -m "Add credentials store package with read, write, and lookup"
```

---

### Task 2: Add Gemini API Key Validation

**Files:**
- Create: `internal/credentials/validate.go`
- Create: `internal/credentials/validate_test.go`

A lightweight Gemini API call to verify a key is valid before writing it to the credentials file.

- [ ] **Step 1: Write the failing test for ValidateGeminiKey**

```go
package credentials

import (
	"context"
	"errors"
	"testing"
)

func TestValidateGeminiKey_AcceptsValidKey(test *testing.T) {
	original := geminiKeyValidator
	defer func() { geminiKeyValidator = original }()

	geminiKeyValidator = func(ctx context.Context, apiKey string) error {
		return nil
	}

	validateError := ValidateGeminiKey(context.Background(), "AIzaValidKey123")
	if validateError != nil {
		test.Errorf("expected nil error, got: %s", validateError)
	}
}

func TestValidateGeminiKey_RejectsInvalidKey(test *testing.T) {
	original := geminiKeyValidator
	defer func() { geminiKeyValidator = original }()

	geminiKeyValidator = func(ctx context.Context, apiKey string) error {
		return errors.New("invalid API key")
	}

	validateError := ValidateGeminiKey(context.Background(), "AIzaBadKey")
	if validateError == nil {
		test.Error("expected error for invalid key")
	}
}

func TestValidateGeminiKey_RejectsEmptyKey(test *testing.T) {
	validateError := ValidateGeminiKey(context.Background(), "")
	if validateError == nil {
		test.Error("expected error for empty key")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/credentials/ -run TestValidateGeminiKey -v`
Expected: FAIL — ValidateGeminiKey does not exist

- [ ] **Step 3: Write ValidateGeminiKey**

```go
// Package credentials manages the unified credentials file that maps client
// identities (bearer tokens or OAuth provider:email pairs) to Gemini API keys.
package credentials

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/genai"
)

// geminiKeyValidator is the function used to validate a Gemini API key.
// It is a package-level variable so tests can replace it without making real API calls.
var geminiKeyValidator = func(ctx context.Context, apiKey string) error {
	client, clientError := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if clientError != nil {
		return fmt.Errorf("invalid Gemini API key: %w", clientError)
	}

	// List models as a lightweight validation call
	iterator := client.Models.List(ctx, &genai.ListModelsConfig{PageSize: 1})
	page, pageError := iterator.Next()
	if pageError != nil {
		return fmt.Errorf("Gemini API key validation failed: %w", pageError)
	}
	if page == nil {
		return errors.New("Gemini API key validation returned no results")
	}
	return nil
}

// ValidateGeminiKey checks whether the given API key is a valid Gemini API key
// by making a lightweight API call. Returns nil if valid, an error otherwise.
func ValidateGeminiKey(ctx context.Context, apiKey string) error {
	if apiKey == "" {
		return errors.New("Gemini API key must not be empty")
	}
	return geminiKeyValidator(ctx, apiKey)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/credentials/ -run TestValidateGeminiKey -v`
Expected: PASS (all 3 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/credentials/validate.go internal/credentials/validate_test.go
git commit -m "Add Gemini API key validation for credentials registration"
```

---

### Task 3: Update Config Package

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

Remove `GeminiAPIKey`, `AuthToken`, `AuthTokensFile`. Add `CredentialsFile`.

- [ ] **Step 1: Write the failing test for CredentialsFile**

Add to `internal/config/config_test.go`:

```go
func TestLoad_CredentialsFilePopulated(test *testing.T) {
	test.Setenv("MCP_CREDENTIALS_FILE", "/tmp/test-creds.json")

	serverConfig, loadError := Load()
	if loadError != nil {
		test.Fatalf("unexpected error: %s", loadError)
	}
	if serverConfig.CredentialsFile != "/tmp/test-creds.json" {
		test.Errorf("expected CredentialsFile to be '/tmp/test-creds.json', got: %s", serverConfig.CredentialsFile)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestLoad_CredentialsFilePopulated -v`
Expected: FAIL — CredentialsFile field does not exist

- [ ] **Step 3: Update Config struct and Load function**

In `internal/config/config.go`, replace the `Config` struct and update `Load()`:

Remove these fields from the struct:
```go
GeminiAPIKey       string
AuthToken          string
AuthTokensFile     string
```

Add this field:
```go
CredentialsFile    string // Path to unified credentials JSON file (optional, hot-reloaded)
```

In `Load()`, remove:
```go
apiKey := os.Getenv("GEMINI_API_KEY")
authToken := os.Getenv("MCP_AUTH_TOKEN")
authTokensFile := os.Getenv("MCP_AUTH_TOKENS_FILE")
```

Add:
```go
credentialsFile := os.Getenv("MCP_CREDENTIALS_FILE")
```

Update the config struct initialization to use `CredentialsFile: credentialsFile` instead of the three removed fields.

Remove the comment about `GEMINI_API_KEY` being optional (lines 51-53).

- [ ] **Step 4: Update existing config tests**

All existing tests that reference `GeminiAPIKey`, `AuthToken`, or `AuthTokensFile` must be updated. Tests that set `GEMINI_API_KEY` env var should be modified:
- Tests that only set `GEMINI_API_KEY` to satisfy a "required" check no longer need it — remove those `test.Setenv("GEMINI_API_KEY", ...)` lines since the field no longer exists.
- Remove `TestLoad_AuthTokensFilePopulated` — replaced by `TestLoad_CredentialsFilePopulated`.
- Update any test that checks `serverConfig.GeminiAPIKey`, `serverConfig.AuthToken`, or `serverConfig.AuthTokensFile` — remove those assertions.

- [ ] **Step 5: Run all config tests**

Run: `go test ./internal/config/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "Replace GeminiAPIKey, AuthToken, AuthTokensFile with CredentialsFile in config"
```

---

### Task 4: Update OAuth Store to Track User Identity

**Files:**
- Modify: `internal/oauth/store.go`
- Modify: `internal/oauth/store_test.go`

Add `ProviderIdentity` field to `AuthCode` and `TokenData` so access tokens can be resolved back to `provider:email`.

- [ ] **Step 1: Write failing test for identity tracking**

Add to `internal/oauth/store_test.go`:

```go
func TestGetAccessTokenData_ReturnsIdentity(test *testing.T) {
	store := NewStore()
	store.StoreAccessToken(&TokenData{
		Token:            "access-token-123",
		ClientID:         "client-1",
		ProviderIdentity: "google:user@example.com",
		ExpiresAt:        time.Now().Add(time.Hour),
	})

	tokenData := store.GetAccessTokenData("access-token-123")
	if tokenData == nil {
		test.Fatal("expected non-nil token data")
	}
	if tokenData.ProviderIdentity != "google:user@example.com" {
		test.Errorf("expected google:user@example.com, got: %s", tokenData.ProviderIdentity)
	}
}

func TestGetAccessTokenData_ReturnsNilForExpired(test *testing.T) {
	store := NewStore()
	store.StoreAccessToken(&TokenData{
		Token:            "expired-token",
		ClientID:         "client-1",
		ProviderIdentity: "google:user@example.com",
		ExpiresAt:        time.Now().Add(-time.Hour),
	})

	tokenData := store.GetAccessTokenData("expired-token")
	if tokenData != nil {
		test.Error("expected nil for expired token")
	}
}

func TestGetAccessTokenData_ReturnsNilForMissing(test *testing.T) {
	store := NewStore()

	tokenData := store.GetAccessTokenData("nonexistent")
	if tokenData != nil {
		test.Error("expected nil for missing token")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/oauth/ -run TestGetAccessTokenData -v`
Expected: FAIL — ProviderIdentity field and GetAccessTokenData method do not exist

- [ ] **Step 3: Add ProviderIdentity field and GetAccessTokenData method**

In `internal/oauth/store.go`, add `ProviderIdentity` to `AuthCode`, `TokenData`, and `RefreshData`:

```go
type AuthCode struct {
	Code              string
	ClientID          string
	RedirectURI       string
	CodeChallenge     string
	ProviderIdentity  string // "provider:email" identity from upstream OAuth
	ExpiresAt         time.Time
}

type TokenData struct {
	Token             string
	ClientID          string
	ProviderIdentity  string // "provider:email" identity from upstream OAuth
	ExpiresAt         time.Time
}

type RefreshData struct {
	Token             string
	ClientID          string
	ProviderIdentity  string // "provider:email" identity from upstream OAuth
	ExpiresAt         time.Time
}
```

Add `GetAccessTokenData` method:

```go
// GetAccessTokenData returns the full token data for the given access token,
// or nil if the token does not exist or has expired. Unlike ValidateAccessToken,
// this method returns the associated metadata including the provider identity.
func (store *Store) GetAccessTokenData(token string) *TokenData {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	tokenData, exists := store.accessTokens[token]
	if !exists {
		return nil
	}
	if time.Now().After(tokenData.ExpiresAt) {
		return nil
	}
	return tokenData
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/oauth/ -run TestGetAccessTokenData -v`
Expected: PASS (all 3 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/oauth/store.go internal/oauth/store_test.go
git commit -m "Add ProviderIdentity tracking to OAuth token data"
```

---

### Task 5: Fetch User Identity in OAuth Callback

**Files:**
- Modify: `internal/oauth/handler.go`
- Modify: `internal/oauth/handler_test.go` (if it exists, otherwise test via integration)

Update the callback handler to call the provider's UserInfoURL after code exchange, extract the email, and store `provider:email` as the identity in the auth code.

- [ ] **Step 1: Write failing test for fetchProviderIdentity**

Create or add to test file:

```go
func TestFetchProviderIdentity_Google(test *testing.T) {
	original := providerIdentityFetcher
	defer func() { providerIdentityFetcher = original }()

	providerIdentityFetcher = func(provider ProviderConfig, providerAccessToken string) (string, error) {
		return "google:user@example.com", nil
	}

	identity, fetchError := fetchProviderIdentity(
		ProviderConfig{Name: "google", UserInfoURL: "https://example.com/userinfo"},
		"fake-access-token",
	)
	if fetchError != nil {
		test.Fatalf("unexpected error: %s", fetchError)
	}
	if identity != "google:user@example.com" {
		test.Errorf("expected google:user@example.com, got: %s", identity)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/oauth/ -run TestFetchProviderIdentity -v`
Expected: FAIL — function does not exist

- [ ] **Step 3: Update exchangeProviderCode to return the provider access token**

Change the signature and implementation of `exchangeProviderCode` in `internal/oauth/handler.go`:

```go
// exchangeProviderCode exchanges an upstream provider authorization code for
// a provider access token. Returns the access token string for subsequent
// userinfo calls.
var exchangeProviderCode = func(provider ProviderConfig, code string, callbackURL string) (string, error) {
	formData := url.Values{}
	formData.Set("grant_type", "authorization_code")
	formData.Set("code", code)
	formData.Set("redirect_uri", callbackURL)
	formData.Set("client_id", provider.ClientID)
	formData.Set("client_secret", provider.ClientSecret)

	resp, postError := http.PostForm(provider.TokenURL, formData)
	if postError != nil {
		return "", fmt.Errorf("provider token request failed: %w", postError)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
		return "", fmt.Errorf("provider returned status %d", resp.StatusCode)
	}

	var tokenResponse struct {
		AccessToken string `json:"access_token"`
	}
	if decodeError := json.NewDecoder(resp.Body).Decode(&tokenResponse); decodeError != nil {
		return "", fmt.Errorf("failed to decode provider token response: %w", decodeError)
	}

	return tokenResponse.AccessToken, nil
}
```

- [ ] **Step 4: Write fetchProviderIdentity function**

Add to `internal/oauth/handler.go`:

```go
// providerIdentityFetcher calls the provider's UserInfo endpoint to retrieve
// the user's identity. It is a variable so tests can replace it.
var providerIdentityFetcher = func(provider ProviderConfig, providerAccessToken string) (string, error) {
	if provider.UserInfoURL == "" {
		return "", errors.New("provider does not have a userinfo endpoint")
	}

	userInfoRequest, requestError := http.NewRequest("GET", provider.UserInfoURL, nil)
	if requestError != nil {
		return "", fmt.Errorf("failed to create userinfo request: %w", requestError)
	}
	userInfoRequest.Header.Set("Authorization", "Bearer "+providerAccessToken)

	if provider.Name == "github" {
		userInfoRequest.Header.Set("Accept", "application/vnd.github+json")
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, fetchError := httpClient.Do(userInfoRequest)
	if fetchError != nil {
		return "", fmt.Errorf("userinfo request failed: %w", fetchError)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
		return "", fmt.Errorf("userinfo endpoint returned status %d", resp.StatusCode)
	}

	var userInfo struct {
		Email string `json:"email"`
	}
	if decodeError := json.NewDecoder(resp.Body).Decode(&userInfo); decodeError != nil {
		return "", fmt.Errorf("failed to decode userinfo response: %w", decodeError)
	}
	if userInfo.Email == "" {
		return "", errors.New("userinfo response did not contain an email")
	}

	return provider.Name + ":" + userInfo.Email, nil
}

// fetchProviderIdentity is the exported wrapper for providerIdentityFetcher.
func fetchProviderIdentity(provider ProviderConfig, providerAccessToken string) (string, error) {
	return providerIdentityFetcher(provider, providerAccessToken)
}
```

- [ ] **Step 5: Update the callback handler to store identity in AuthCode**

In `NewCallbackHandler`, after the successful code exchange, add identity fetching:

```go
// After exchangeProviderCode returns successfully:
providerAccessToken, exchangeError := exchangeProviderCode(*matchedProvider, code, callbackURL)
if exchangeError != nil {
	// ... existing error handling
}

providerIdentity, identityError := fetchProviderIdentity(*matchedProvider, providerAccessToken)
if identityError != nil {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusBadGateway)
	json.NewEncoder(writer).Encode(map[string]string{ //nolint:errcheck
		"error":             "server_error",
		"error_description": "failed to retrieve user identity from provider",
	})
	return
}

// Store identity in auth code
store.StoreAuthCode(&AuthCode{
	Code:             mcpCode,
	ClientID:         session.ClientID,
	RedirectURI:      session.RedirectURI,
	CodeChallenge:    session.CodeChallenge,
	ProviderIdentity: providerIdentity,
	ExpiresAt:        time.Now().Add(10 * time.Minute),
})
```

- [ ] **Step 6: Update issueTokenResponse to propagate identity**

Change `issueTokenResponse` to accept and propagate `providerIdentity`:

```go
func issueTokenResponse(writer http.ResponseWriter, store *Store, clientID string, providerIdentity string) {
	// ... existing token generation ...
	store.StoreAccessToken(&TokenData{
		Token:            accessToken,
		ClientID:         clientID,
		ProviderIdentity: providerIdentity,
		ExpiresAt:        time.Now().Add(time.Hour),
	})
	store.StoreRefreshToken(&RefreshData{
		Token:            newRefreshToken,
		ClientID:         clientID,
		ProviderIdentity: providerIdentity,
		ExpiresAt:        time.Now().Add(30 * 24 * time.Hour),
	})
	// ... existing response writing ...
}
```

Update callers:
- `handleAuthorizationCodeGrant`: pass `authCode.ProviderIdentity` to `issueTokenResponse`
- `handleRefreshTokenGrant`: pass `refreshData.ProviderIdentity` to `issueTokenResponse`

- [ ] **Step 7: Update existing handler tests**

Update any existing tests that call `exchangeProviderCode` to match the new signature returning `(string, error)` instead of just `error`. Update mocks accordingly.

- [ ] **Step 8: Run all OAuth tests**

Run: `go test ./internal/oauth/ -v`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add internal/oauth/handler.go internal/oauth/store.go
git commit -m "Fetch provider identity in OAuth callback and propagate to tokens"
```

---

### Task 6: Add OAuth Gemini Key Prompt Page

**Files:**
- Create: `internal/oauth/gemini_prompt.html`
- Modify: `internal/oauth/handler.go`
- Modify: `internal/server/server.go`

Add a page that prompts the user to enter their Gemini API key after OAuth login, before issuing the final auth code redirect.

- [ ] **Step 1: Create the Gemini key prompt HTML template**

Create `internal/oauth/gemini_prompt.html`:

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>mcp-banana - Gemini API Key</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, sans-serif; max-width: 480px; margin: 60px auto; padding: 0 20px; }
        h1 { font-size: 1.5em; }
        .info { color: #555; margin-bottom: 20px; }
        input[type="text"] { width: 100%; padding: 10px; font-size: 1em; border: 1px solid #ccc; border-radius: 4px; box-sizing: border-box; }
        button { padding: 10px 24px; font-size: 1em; cursor: pointer; border: none; border-radius: 4px; margin-top: 12px; }
        .submit { background: #2563eb; color: white; }
        .skip { background: #e5e7eb; color: #333; margin-left: 8px; }
        .error { color: #dc2626; margin-top: 8px; }
        .required { color: #dc2626; font-weight: bold; }
    </style>
</head>
<body>
    <h1>Gemini API Key</h1>
    <p class="info">Signed in as <strong>{{.Identity}}</strong></p>
    {{if .Error}}
    <p class="error">{{.Error}}</p>
    {{end}}
    {{if .IsReturning}}
    <p>You already have a Gemini API key configured. Enter a new key to update it, or skip to keep your current key.</p>
    {{else}}
    <p><span class="required">Required:</span> Enter your Gemini API key to use mcp-banana. Get one at <a href="https://aistudio.google.com/" target="_blank">aistudio.google.com</a>.</p>
    {{end}}
    <form method="POST" action="/gemini-key">
        <input type="hidden" name="session_token" value="{{.SessionToken}}">
        <input type="text" name="gemini_api_key" placeholder="AIza..." autocomplete="off">
        <div>
            <button type="submit" class="submit">Save Key</button>
            {{if .IsReturning}}
            <button type="submit" name="action" value="skip" class="skip">Skip</button>
            {{end}}
        </div>
    </form>
</body>
</html>
```

- [ ] **Step 2: Write the Gemini key prompt handler**

Add to `internal/oauth/handler.go`:

```go
//go:embed gemini_prompt.html
var geminiPromptFS embed.FS

// GeminiKeySession stores the pending state between the OAuth callback and the
// Gemini key submission form. It holds everything needed to issue the final
// MCP auth code after the user provides their Gemini API key.
type GeminiKeySession struct {
	ProviderIdentity string
	ClientID         string
	RedirectURI      string
	CodeChallenge    string
	OriginalState    string
	ExpiresAt        time.Time
}
```

Add `geminiKeySessions` to the `Store` struct in `store.go`:

```go
type Store struct {
	mutex              sync.RWMutex
	clients            map[string]*Client
	authCodes          map[string]*AuthCode
	accessTokens       map[string]*TokenData
	refreshTokens      map[string]*RefreshData
	providerSessions   map[string]*ProviderSession
	geminiKeySessions  map[string]*GeminiKeySession
}
```

Update `NewStore` to initialize `geminiKeySessions`:
```go
geminiKeySessions: make(map[string]*GeminiKeySession),
```

Add store methods for `GeminiKeySession`:
```go
// StoreGeminiKeySession persists a Gemini key prompt session.
func (store *Store) StoreGeminiKeySession(token string, session *GeminiKeySession) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.geminiKeySessions[token] = session
}

// ConsumeGeminiKeySession retrieves and removes a Gemini key session.
func (store *Store) ConsumeGeminiKeySession(token string) *GeminiKeySession {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	session, exists := store.geminiKeySessions[token]
	if !exists {
		return nil
	}
	delete(store.geminiKeySessions, token)
	if time.Now().After(session.ExpiresAt) {
		return nil
	}
	return session
}
```

Update `CleanupExpired` to also clean `geminiKeySessions`.

- [ ] **Step 3: Write NewGeminiKeyPromptHandler**

Add to `internal/oauth/handler.go`:

```go
// NewGeminiKeyPromptHandler renders the Gemini API key input form.
// It is shown after OAuth callback, before issuing the MCP auth code.
func NewGeminiKeyPromptHandler(store *Store) http.Handler {
	promptTemplate := template.Must(template.ParseFS(geminiPromptFS, "gemini_prompt.html"))

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		sessionToken := request.URL.Query().Get("session")
		errorMessage := request.URL.Query().Get("error")

		// Peek at session without consuming it
		store.mutex.RLock()
		session, exists := store.geminiKeySessions[sessionToken]
		store.mutex.RUnlock()

		if !exists || session == nil || time.Now().After(session.ExpiresAt) {
			writeJSONError(writer, "invalid_request", http.StatusBadRequest)
			return
		}

		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		writer.WriteHeader(http.StatusOK)
		isReturning := request.URL.Query().Get("returning") == "true"

		promptTemplate.Execute(writer, map[string]interface{}{ //nolint:errcheck
			"Identity":     session.ProviderIdentity,
			"IsReturning":  isReturning,
			"SessionToken": sessionToken,
			"Error":        errorMessage,
		})
	})
}
```

- [ ] **Step 4: Write NewGeminiKeySubmitHandler**

```go
// NewGeminiKeySubmitHandler processes the Gemini API key form submission.
// It validates the key, writes it to the credentials store, and issues the
// MCP auth code redirect.
func NewGeminiKeySubmitHandler(store *Store, credStore CredentialsStore) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if parseError := request.ParseForm(); parseError != nil {
			writeJSONError(writer, "invalid_request", http.StatusBadRequest)
			return
		}

		sessionToken := request.PostForm.Get("session_token")
		geminiAPIKey := strings.TrimSpace(request.PostForm.Get("gemini_api_key"))
		action := request.PostForm.Get("action")

		session := store.ConsumeGeminiKeySession(sessionToken)
		if session == nil {
			writeJSONError(writer, "invalid_request", http.StatusBadRequest)
			return
		}

		// Handle skip (returning users only)
		if action == "skip" {
			// Re-check that the user actually has a key on file
			if !credStore.Exists(session.ProviderIdentity) {
				// Cannot skip — re-create session and redirect back with error
				newToken, _ := GenerateRandomToken(16)
				session.ExpiresAt = time.Now().Add(10 * time.Minute)
				store.StoreGeminiKeySession(newToken, session)
				http.Redirect(writer, request, "/gemini-key?session="+newToken+"&error=You+must+provide+a+Gemini+API+key", http.StatusFound)
				return
			}
			// Skip — issue auth code with existing key
			issueAuthCodeRedirect(writer, request, store, session)
			return
		}

		// Validate non-empty key
		if geminiAPIKey == "" {
			newToken, _ := GenerateRandomToken(16)
			session.ExpiresAt = time.Now().Add(10 * time.Minute)
			store.StoreGeminiKeySession(newToken, session)
			http.Redirect(writer, request, "/gemini-key?session="+newToken+"&error=Gemini+API+key+must+not+be+empty", http.StatusFound)
			return
		}

		// Validate the Gemini key with a real API call
		validateError := ValidateGeminiKey(request.Context(), geminiAPIKey)
		if validateError != nil {
			newToken, _ := GenerateRandomToken(16)
			session.ExpiresAt = time.Now().Add(10 * time.Minute)
			store.StoreGeminiKeySession(newToken, session)
			http.Redirect(writer, request, "/gemini-key?session="+newToken+"&error=Invalid+Gemini+API+key", http.StatusFound)
			return
		}

		// Write to credentials file
		registerError := credStore.Register(session.ProviderIdentity, geminiAPIKey)
		if registerError != nil {
			writeJSONError(writer, "server_error", http.StatusInternalServerError)
			return
		}

		issueAuthCodeRedirect(writer, request, store, session)
	})
}

// issueAuthCodeRedirect generates an MCP auth code and redirects the client.
func issueAuthCodeRedirect(writer http.ResponseWriter, request *http.Request, store *Store, session *GeminiKeySession) {
	mcpCode, generateError := GenerateRandomToken(32)
	if generateError != nil {
		writeJSONError(writer, "server_error", http.StatusInternalServerError)
		return
	}

	store.StoreAuthCode(&AuthCode{
		Code:             mcpCode,
		ClientID:         session.ClientID,
		RedirectURI:      session.RedirectURI,
		CodeChallenge:    session.CodeChallenge,
		ProviderIdentity: session.ProviderIdentity,
		ExpiresAt:        time.Now().Add(10 * time.Minute),
	})

	redirectTarget, parseError := url.Parse(session.RedirectURI)
	if parseError != nil {
		writeJSONError(writer, "server_error", http.StatusInternalServerError)
		return
	}
	redirectQuery := redirectTarget.Query()
	redirectQuery.Set("code", mcpCode)
	redirectQuery.Set("state", session.OriginalState)
	redirectTarget.RawQuery = redirectQuery.Encode()

	http.Redirect(writer, request, redirectTarget.String(), http.StatusFound)
}
```

- [ ] **Step 5: Define CredentialsStore interface in oauth package**

Add to `internal/oauth/handler.go`:

```go
// CredentialsStore is the interface that the OAuth handlers use to read and write
// credentials. It is satisfied by *credentials.Store.
type CredentialsStore interface {
	Lookup(identity string) string
	Exists(identity string) bool
	Register(identity string, geminiAPIKey string) error
}
```

- [ ] **Step 6: Update the callback handler to redirect to Gemini key prompt**

In `NewCallbackHandler`, after fetching the provider identity, instead of immediately issuing an auth code, redirect to the Gemini key prompt:

```go
// After identity is fetched successfully:
isReturning := credStore.Exists(providerIdentity)

sessionToken, generateError := GenerateRandomToken(16)
if generateError != nil {
	writeJSONError(writer, "server_error", http.StatusInternalServerError)
	return
}

store.StoreGeminiKeySession(sessionToken, &GeminiKeySession{
	ProviderIdentity: providerIdentity,
	ClientID:         session.ClientID,
	RedirectURI:      session.RedirectURI,
	CodeChallenge:    session.CodeChallenge,
	OriginalState:    session.OriginalState,
	ExpiresAt:        time.Now().Add(10 * time.Minute),
})

returningParam := ""
if isReturning {
	returningParam = "&returning=true"
}
http.Redirect(writer, request, "/gemini-key?session="+sessionToken+returningParam, http.StatusFound)
```

Update `NewCallbackHandler` signature to accept `credStore CredentialsStore`.

- [ ] **Step 7: Run all OAuth tests**

Run: `go test ./internal/oauth/ -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/oauth/handler.go internal/oauth/store.go internal/oauth/gemini_prompt.html
git commit -m "Add Gemini API key prompt page to OAuth onboarding flow"
```

---

### Task 7: Rewrite Middleware Authentication

**Files:**
- Modify: `internal/server/middleware.go`
- Modify: `internal/server/server.go`

Replace the current auth logic with credentials-file-based resolution. Remove `loadTokensFromFile`. Add Gemini key context injection from credentials file.

- [ ] **Step 1: Write failing test for new auth flow**

Add to `internal/server/server_test.go` (or create a new test file):

```go
func TestMiddleware_OAuthTakesPrecedence(test *testing.T) {
	// Setup: OAuth store with a valid access token mapped to provider identity
	oauthStore := oauth.NewStore()
	oauthStore.StoreAccessToken(&oauth.TokenData{
		Token:            "oauth-access-token",
		ClientID:         "test-client",
		ProviderIdentity: "google:user@example.com",
		ExpiresAt:        time.Now().Add(time.Hour),
	})

	// Setup: Credentials file with the provider identity mapped to a Gemini key
	credFilePath := filepath.Join(test.TempDir(), "creds.json")
	os.WriteFile(credFilePath, []byte(`{"google:user@example.com":"AIzaOAuthKey"}`), 0600)
	credStore, _ := credentials.NewStore(credFilePath)

	// ... construct middleware with credStore, send request with Bearer oauth-access-token
	// Assert: request passes auth, context contains Gemini key "AIzaOAuthKey"
}

func TestMiddleware_StaticTokenFromCredentialsFile(test *testing.T) {
	credFilePath := filepath.Join(test.TempDir(), "creds.json")
	os.WriteFile(credFilePath, []byte(`{"my-static-token":"AIzaStaticKey"}`), 0600)
	credStore, _ := credentials.NewStore(credFilePath)

	// ... send request with Bearer my-static-token
	// Assert: passes auth, context contains Gemini key "AIzaStaticKey"
}

func TestMiddleware_SelfRegistration(test *testing.T) {
	credFilePath := filepath.Join(test.TempDir(), "creds.json")
	credStore, _ := credentials.NewStore(credFilePath)

	// ... send request with Bearer new-token + X-Gemini-API-Key: AIzaNewKey
	// Assert: passes, key is registered in credentials file
}

func TestMiddleware_RejectsUnknownTokenWithoutHeader(test *testing.T) {
	credFilePath := filepath.Join(test.TempDir(), "creds.json")
	credStore, _ := credentials.NewStore(credFilePath)

	// ... send request with Bearer unknown-token, no X-Gemini-API-Key
	// Assert: 401
}

func TestMiddleware_RejectsEmptyGeminiKeyHeader(test *testing.T) {
	credFilePath := filepath.Join(test.TempDir(), "creds.json")
	credStore, _ := credentials.NewStore(credFilePath)

	// ... send request with Bearer new-token + X-Gemini-API-Key: ""
	// Assert: 401 (treated as if header not sent)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/server/ -run TestMiddleware_ -v`
Expected: FAIL — new middleware logic does not exist

- [ ] **Step 3: Update middleware struct**

In `internal/server/middleware.go`, update the `middleware` struct:

```go
type middleware struct {
	cfg        *config.Config
	logger     *slog.Logger
	limiter    *rate.Limiter
	semaphore  chan struct{}
	oauthStore *oauth.Store
	credStore  *credentials.Store
}
```

Update `newMiddleware` to accept `credStore`:

```go
func newMiddleware(cfg *config.Config, logger *slog.Logger, oauthStore *oauth.Store, credStore *credentials.Store) *middleware {
	// ... existing limiter and semaphore setup ...
	return &middleware{
		cfg:        cfg,
		logger:     logger,
		limiter:    limiter,
		semaphore:  semaphore,
		oauthStore: oauthStore,
		credStore:  credStore,
	}
}
```

- [ ] **Step 4: Rewrite authenticateRequest**

Replace the entire `authenticateRequest` method:

```go
// authenticateRequest resolves the client's identity and Gemini API key.
// Returns the Gemini API key if authenticated, or empty string if rejected.
//
// Resolution priority:
//  1. OAuth access token → resolve to provider:email → lookup in credentials file
//  2. Bearer token → lookup directly in credentials file
//  3. Unknown token + X-Gemini-API-Key header → self-register and proceed
//  4. Unknown token + no header → reject
//
// Auth is enforced if MCP_CREDENTIALS_FILE is set or OAuth is configured.
func (mw *middleware) authenticateRequest(request *http.Request) (string, bool) {
	hasCredentials := mw.credStore != nil
	hasOAuth := mw.oauthStore != nil

	// No auth configured — SSH tunnel mode
	if !hasCredentials && !hasOAuth {
		return "", true
	}

	// Extract bearer token
	authHeader := request.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, authHeaderPrefix) {
		return "", false
	}
	requestToken := authHeader[len(authHeaderPrefix):]
	if requestToken == "" {
		return "", false
	}

	// Priority 1: OAuth access token
	if hasOAuth {
		tokenData := mw.oauthStore.GetAccessTokenData(requestToken)
		if tokenData != nil && tokenData.ProviderIdentity != "" {
			if hasCredentials {
				geminiKey := mw.credStore.Lookup(tokenData.ProviderIdentity)
				if geminiKey != "" {
					security.RegisterSecret(geminiKey)
					return geminiKey, true
				}
			}
			// OAuth valid but no Gemini key in credentials file
			return "", false
		}
	}

	// Priority 2: Static bearer token in credentials file
	if hasCredentials {
		geminiKey := mw.credStore.Lookup(requestToken)
		if geminiKey != "" {
			security.RegisterSecret(geminiKey)
			return geminiKey, true
		}
	}

	// Priority 3: Self-registration via X-Gemini-API-Key header
	geminiAPIKey := request.Header.Get("X-Gemini-API-Key")
	geminiAPIKey = strings.TrimSpace(geminiAPIKey)
	if geminiAPIKey != "" && hasCredentials {
		// Validate the key before registering
		validateError := credentials.ValidateGeminiKey(request.Context(), geminiAPIKey)
		if validateError != nil {
			mw.logger.Warn("self-registration rejected: invalid Gemini API key")
			return "", false
		}
		registerError := mw.credStore.Register(requestToken, geminiAPIKey)
		if registerError != nil {
			mw.logger.Error("failed to register credentials", "error", registerError)
			return "", false
		}
		security.RegisterSecret(geminiAPIKey)
		return geminiAPIKey, true
	}

	// Priority 4: Reject
	return "", false
}
```

- [ ] **Step 5: Update WrapHTTP to use the new authenticateRequest return value**

In `WrapHTTP`, update the auth check and Gemini key injection:

```go
// 2. Bearer token auth + Gemini key resolution
geminiKey, authenticated := mw.authenticateRequest(request)
if !authenticated {
	if mw.cfg.OAuthBaseURL != "" {
		writer.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+mw.cfg.OAuthBaseURL+`/.well-known/oauth-protected-resource"`)
	}
	writeJSONError(writer, http.StatusUnauthorized, "unauthorized")
	return
}

// Store resolved Gemini key in context for downstream tool handlers
if geminiKey != "" {
	request = request.WithContext(gemini.WithAPIKey(request.Context(), geminiKey))
}
```

Remove the separate `X-Gemini-API-Key` header extraction block (lines 168-176 in the current code) — this is now handled inside `authenticateRequest`.

- [ ] **Step 6: Remove loadTokensFromFile**

Delete the `loadTokensFromFile` function from `middleware.go` — it is no longer used.

- [ ] **Step 7: Update WrapWithMiddleware and NewHTTPHandler**

Update `WrapWithMiddleware` in `server.go` to accept `credStore`:

```go
func WrapWithMiddleware(cfg *config.Config, logger *slog.Logger, inner http.Handler, oauthStore *oauth.Store, credStore *credentials.Store) http.Handler {
	mw := newMiddleware(cfg, logger, oauthStore, credStore)
	return mw.WrapHTTP(inner)
}
```

Update `NewHTTPHandler` signature and internal call:

```go
func NewHTTPHandler(mcpSrv *mcpserver.MCPServer, serverConfig *config.Config, logger *slog.Logger, oauthStore *oauth.Store, providers []oauth.ProviderConfig, credStore *credentials.Store) http.Handler {
	// ...
	mw := newMiddleware(serverConfig, logger, oauthStore, credStore)
	// ...
}
```

Also mount the new Gemini key prompt routes in the public mux:

```go
if oauthStore != nil && serverConfig.OAuthBaseURL != "" {
	// ... existing OAuth routes ...
	publicMux.Handle("/gemini-key", oauth.NewGeminiKeyPromptHandler(oauthStore))
	publicMux.HandleFunc("/gemini-key", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			oauth.NewGeminiKeySubmitHandler(oauthStore, credStore).ServeHTTP(writer, request)
		} else {
			oauth.NewGeminiKeyPromptHandler(oauthStore).ServeHTTP(writer, request)
		}
	})
}
```

- [ ] **Step 8: Update existing server tests**

Update all tests in `internal/server/server_test.go` that reference `AuthToken`, `AuthTokensFile`, or `GeminiAPIKey` in config to use the new `CredentialsFile` field and `credStore` parameter.

- [ ] **Step 9: Run all server tests**

Run: `go test ./internal/server/ -v`
Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add internal/server/middleware.go internal/server/server.go internal/server/server_test.go
git commit -m "Rewrite auth middleware to use unified credentials file"
```

---

### Task 8: Update Tool Handlers

**Files:**
- Modify: `internal/tools/generate.go`
- Modify: `internal/tools/edit.go`
- Modify: `internal/tools/tools_test.go`

Tool handlers no longer resolve the Gemini client themselves — the middleware has already resolved the key and placed it in context. Simplify the handlers.

- [ ] **Step 1: Simplify generate handler**

In `internal/tools/generate.go`, replace the client resolution block:

```go
func NewGenerateImageHandler(clientCache *gemini.ClientCache, maxImageBytes int) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(requestContext context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		perRequestKey := gemini.APIKeyFromContext(requestContext)
		if perRequestKey == "" {
			return mcp.NewToolResultError("no Gemini API key available: authenticate with a valid token"), nil
		}

		resolvedClient, clientError := clientCache.GetClient(requestContext, perRequestKey)
		if clientError != nil {
			return mcp.NewToolResultError("failed to initialize Gemini client"), nil
		}

		// ... rest of validation and generation unchanged, using resolvedClient ...
	}
}
```

Remove the `service gemini.GeminiService` parameter — all clients come from the cache.

- [ ] **Step 2: Simplify edit handler**

Same pattern as generate handler:

```go
func NewEditImageHandler(clientCache *gemini.ClientCache, maxImageBytes int) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(requestContext context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		perRequestKey := gemini.APIKeyFromContext(requestContext)
		if perRequestKey == "" {
			return mcp.NewToolResultError("no Gemini API key available: authenticate with a valid token"), nil
		}

		resolvedClient, clientError := clientCache.GetClient(requestContext, perRequestKey)
		if clientError != nil {
			return mcp.NewToolResultError("failed to initialize Gemini client"), nil
		}

		// ... rest unchanged, using resolvedClient ...
	}
}
```

- [ ] **Step 3: Update NewMCPServer**

In `internal/server/server.go`, update `NewMCPServer` to remove the `service` parameter:

```go
func NewMCPServer(clientCache *gemini.ClientCache, maxImageBytes int) *mcpserver.MCPServer {
	srv := mcpserver.NewMCPServer("mcp-banana", "1.0.0")
	// ...
	srv.AddTool(generateImageTool, tools.NewGenerateImageHandler(clientCache, maxImageBytes))
	srv.AddTool(editImageTool, tools.NewEditImageHandler(clientCache, maxImageBytes))
	// ...
}
```

- [ ] **Step 4: Update tool tests**

Update `internal/tools/tools_test.go` to set the Gemini key in context instead of passing a default service:

```go
// Use gemini.WithAPIKey(ctx, "test-key") instead of passing a service
```

- [ ] **Step 5: Run all tool tests**

Run: `go test ./internal/tools/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/tools/generate.go internal/tools/edit.go internal/tools/tools_test.go internal/server/server.go
git commit -m "Simplify tool handlers to use context-resolved Gemini key"
```

---

### Task 9: Update main.go

**Files:**
- Modify: `cmd/mcp-banana/main.go`

Remove the startup Gemini client creation, add credentials store initialization.

- [ ] **Step 1: Update the run function**

Replace the Gemini client creation block (lines 101-121) with credentials store initialization:

```go
// Initialize credentials store (if configured)
var credStore *credentials.Store
if serverConfig.CredentialsFile != "" {
	var credError error
	credStore, credError = credentials.NewStore(serverConfig.CredentialsFile)
	if credError != nil {
		_, _ = fmt.Fprintf(stderr, "failed to initialize credentials store: %s\n", credError)
		return 1
	}
}

// ClientCache for per-key Gemini clients — no default client
clientCache := gemini.NewClientCache(nil, serverConfig.RequestTimeoutSecs, serverConfig.ProConcurrency)

mcpServer := internalserver.NewMCPServer(clientCache, serverConfig.MaxImageBytes)
```

Remove the `security.RegisterSecret(serverConfig.GeminiAPIKey)` line — secrets are now registered dynamically when keys are read from the credentials file.

Remove `security.RegisterSecret(serverConfig.AuthToken)`.

Update the `runHTTPServer` call to pass `credStore`:

```go
return runHTTPServer(mcpServer, serverConfig, logger, *address, oauthStore, providers, credStore)
```

Update `runHTTPServer` function signature:

```go
func runHTTPServer(mcpServer *server.MCPServer, serverConfig *config.Config, logger *slog.Logger, address string, oauthStore *oauth.Store, providers []oauth.ProviderConfig, credStore *credentials.Store) int {
	handler := internalserver.NewHTTPHandler(mcpServer, serverConfig, logger, oauthStore, providers, credStore)
	// ... rest unchanged
}
```

Remove `clientFactory` variable — no longer needed at startup.

- [ ] **Step 2: Update the stdio transport to also support credentials**

For stdio mode, the middleware doesn't run (stdio uses MCP protocol directly, not HTTP). The Gemini key must come from somewhere. Since stdio is for local Claude Code integration, add a warning if no credentials file is configured:

```go
case "stdio":
	if credStore == nil {
		logger.Warn("stdio mode with no MCP_CREDENTIALS_FILE: tool calls will fail without a Gemini API key in context")
	}
	logger.Info("starting mcp-banana in stdio mode", "version", version)
	if stdioError := stdioServe(mcpServer); stdioError != nil {
		_, _ = fmt.Fprintf(stderr, "stdio server error: %s\n", stdioError)
		return 1
	}
```

- [ ] **Step 3: Update main_test.go**

Update all tests that reference `GeminiAPIKey`, `AuthToken`, or `clientFactory` to use the new credentials file approach. Create temp credential files in test setup.

- [ ] **Step 4: Run main package tests**

Run: `go test ./cmd/mcp-banana/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/mcp-banana/main.go
git commit -m "Wire credentials store into main startup and HTTP handler"
```

---

### Task 10: Update ClientCache (Remove Default Client)

**Files:**
- Modify: `internal/gemini/cache.go`
- Modify: `internal/gemini/cache_test.go`

Remove the `defaultClient` concept — all clients are per-key from the credentials file.

- [ ] **Step 1: Simplify ClientCache**

Update `internal/gemini/cache.go`:

```go
// ClientCache manages a pool of Gemini clients, one per unique API key.
// Clients are created on demand and cached for reuse.
type ClientCache struct {
	mutex          sync.RWMutex
	clients        map[string]*Client
	timeoutSecs    int
	proConcurrency int
}

// NewClientCache creates an empty client cache.
func NewClientCache(timeoutSecs int, proConcurrency int) *ClientCache {
	return &ClientCache{
		clients:        make(map[string]*Client),
		timeoutSecs:    timeoutSecs,
		proConcurrency: proConcurrency,
	}
}

// GetClient returns a Gemini client for the given API key.
// Returns an error if apiKey is empty.
// If a client for apiKey is already cached, it is returned.
// Otherwise a new client is created, cached, and returned.
func (cache *ClientCache) GetClient(ctx context.Context, apiKey string) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	// Fast path: read lock
	cache.mutex.RLock()
	existing, found := cache.clients[apiKey]
	cache.mutex.RUnlock()
	if found {
		return existing, nil
	}

	if afterReadMiss != nil {
		afterReadMiss()
	}

	// Slow path: write lock
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	if existing, found = cache.clients[apiKey]; found {
		return existing, nil
	}

	created, clientError := NewClient(ctx, apiKey, cache.timeoutSecs, cache.proConcurrency)
	if clientError != nil {
		return nil, fmt.Errorf("failed to create gemini client: %w", clientError)
	}

	cache.clients[apiKey] = created
	return created, nil
}
```

- [ ] **Step 2: Update cache tests**

Remove tests that reference `defaultClient`. Update `NewClientCache` calls to use the new 2-parameter signature.

- [ ] **Step 3: Update all callers of NewClientCache**

Search for `NewClientCache(` and update to remove the first `defaultClient` parameter. This includes `cmd/mcp-banana/main.go` (already updated in Task 9).

- [ ] **Step 4: Run all cache tests**

Run: `go test ./internal/gemini/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/gemini/cache.go internal/gemini/cache_test.go
git commit -m "Remove default client from ClientCache, require per-key resolution"
```

---

### Task 11: Update .env.example and README

**Files:**
- Modify: `.env.example`
- Modify: `README.md`

- [ ] **Step 1: Update .env.example**

Remove `GEMINI_API_KEY`, `MCP_AUTH_TOKEN`, and `MCP_AUTH_TOKENS_FILE` entries.

Add the new `MCP_CREDENTIALS_FILE` entry:

```env
# [OPTIONAL] Path to the unified credentials JSON file.
# Maps client identities (bearer tokens or OAuth provider:email pairs) to
# Gemini API keys. The file is re-read on every request for hot-reload.
# Format: {"bearer_token_or_identity": "gemini_api_key", ...}
# Auto-created: If the file does not exist, the server creates it with {}.
# Self-registration: Clients can register by sending both Authorization: Bearer <token>
#   and X-Gemini-API-Key: <key> headers on their first request.
# OAuth: Users are prompted to enter their Gemini key during the OAuth login flow.
# Example: MCP_CREDENTIALS_FILE=/opt/mcp-banana/credentials.json
MCP_CREDENTIALS_FILE=
```

- [ ] **Step 2: Update README.md**

Update the relevant sections:
- Replace references to `GEMINI_API_KEY`, `MCP_AUTH_TOKEN`, `MCP_AUTH_TOKENS_FILE` with `MCP_CREDENTIALS_FILE`.
- Update the authentication section to describe the new priority order.
- Document the self-registration flow.
- Document the OAuth onboarding flow with Gemini key prompt.
- Add migration notes for existing users.

- [ ] **Step 3: Commit**

```bash
git add .env.example README.md
git commit -m "Update docs for unified credentials file"
```

---

### Task 12: Run Full CI Sequence

**Files:** None (validation only)

- [ ] **Step 1: Run golangci-lint**

Run: `golangci-lint run`
Expected: No errors. Fix any that appear.

- [ ] **Step 2: Run gofmt**

Run: `gofmt -w .`
Expected: No changes (code should already be formatted).

- [ ] **Step 3: Run go vet**

Run: `go vet ./...`
Expected: No errors.

- [ ] **Step 4: Run all tests with race detector**

Run: `go test -race -coverprofile=coverage.out ./...`
Expected: All tests pass, coverage above 80%.

- [ ] **Step 5: Fix any issues and re-run until all green**

Iterate on any failures until the full CI sequence passes.

- [ ] **Step 6: Final commit if any fixes were needed**

```bash
git add -A
git commit -m "Fix CI issues from unified credentials file implementation"
```
