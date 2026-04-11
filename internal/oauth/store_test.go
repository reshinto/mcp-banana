package oauth

import (
	"testing"
	"time"
)

func TestStore_RegisterAndLookupClient(test *testing.T) {
	store := NewStore()
	client := &Client{
		ClientID:     "test-client",
		ClientName:   "Test Client",
		RedirectURIs: []string{"https://example.com/callback"},
	}

	store.RegisterClient(client)
	retrieved := store.GetClient("test-client")

	if retrieved == nil {
		test.Fatal("expected client to be found, got nil")
	}
	if retrieved.ClientID != "test-client" {
		test.Errorf("expected ClientID %q, got %q", "test-client", retrieved.ClientID)
	}
	if retrieved.ClientName != "Test Client" {
		test.Errorf("expected ClientName %q, got %q", "Test Client", retrieved.ClientName)
	}
}

func TestStore_GetClient_NotFound(test *testing.T) {
	store := NewStore()
	result := store.GetClient("nonexistent")
	if result != nil {
		test.Errorf("expected nil for nonexistent client, got %+v", result)
	}
}

func TestStore_StoreAndConsumeAuthCode(test *testing.T) {
	store := NewStore()
	authCode := &AuthCode{
		Code:          "auth-code-123",
		ClientID:      "client-1",
		RedirectURI:   "https://example.com/callback",
		CodeChallenge: "challenge",
		ExpiresAt:     time.Now().Add(5 * time.Minute),
	}

	store.StoreAuthCode(authCode)

	first := store.ConsumeAuthCode("auth-code-123")
	if first == nil {
		test.Fatal("expected auth code on first consume, got nil")
	}
	if first.Code != "auth-code-123" {
		test.Errorf("expected Code %q, got %q", "auth-code-123", first.Code)
	}

	second := store.ConsumeAuthCode("auth-code-123")
	if second != nil {
		test.Errorf("expected nil on second consume (single-use), got %+v", second)
	}
}

func TestStore_ConsumeAuthCode_Expired(test *testing.T) {
	store := NewStore()
	authCode := &AuthCode{
		Code:      "expired-code",
		ClientID:  "client-1",
		ExpiresAt: time.Now().Add(-1 * time.Second),
	}

	store.StoreAuthCode(authCode)

	result := store.ConsumeAuthCode("expired-code")
	if result != nil {
		test.Errorf("expected nil for expired auth code, got %+v", result)
	}
}

func TestStore_StoreAndValidateAccessToken(test *testing.T) {
	store := NewStore()
	tokenData := &TokenData{
		Token:     "access-token-abc",
		ClientID:  "client-1",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	store.StoreAccessToken(tokenData)

	valid := store.ValidateAccessToken("access-token-abc")
	if !valid {
		test.Error("expected valid access token to return true")
	}
}

func TestStore_ValidateAccessToken_Expired(test *testing.T) {
	store := NewStore()
	tokenData := &TokenData{
		Token:     "expired-access-token",
		ClientID:  "client-1",
		ExpiresAt: time.Now().Add(-1 * time.Second),
	}

	store.StoreAccessToken(tokenData)

	valid := store.ValidateAccessToken("expired-access-token")
	if valid {
		test.Error("expected expired access token to return false")
	}
}

func TestStore_StoreAndConsumeRefreshToken(test *testing.T) {
	store := NewStore()
	refreshData := &RefreshData{
		Token:     "refresh-token-xyz",
		ClientID:  "client-1",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	store.StoreRefreshToken(refreshData)

	first := store.ConsumeRefreshToken("refresh-token-xyz")
	if first == nil {
		test.Fatal("expected refresh token on first consume, got nil")
	}
	if first.Token != "refresh-token-xyz" {
		test.Errorf("expected Token %q, got %q", "refresh-token-xyz", first.Token)
	}

	second := store.ConsumeRefreshToken("refresh-token-xyz")
	if second != nil {
		test.Errorf("expected nil on second consume (single-use), got %+v", second)
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
		Provider:      "google",
		ExpiresAt:     time.Now().Add(10 * time.Minute),
	}

	store.StoreProviderSession(session)

	retrieved := store.GetProviderSession("state-abc")
	if retrieved == nil {
		test.Fatal("expected provider session, got nil")
	}
	if retrieved.State != "state-abc" {
		test.Errorf("expected State %q, got %q", "state-abc", retrieved.State)
	}
	if retrieved.Provider != "google" {
		test.Errorf("expected Provider %q, got %q", "google", retrieved.Provider)
	}

	// GetProviderSession is single-use (consumed on read)
	again := store.GetProviderSession("state-abc")
	if again != nil {
		test.Errorf("expected nil on second get (consumed on read), got %+v", again)
	}
}

func TestStore_CleanupExpired(test *testing.T) {
	store := NewStore()

	// Register expired entries
	store.StoreAuthCode(&AuthCode{
		Code:      "expired-code",
		ClientID:  "client-1",
		ExpiresAt: time.Now().Add(-1 * time.Second),
	})
	store.StoreAccessToken(&TokenData{
		Token:     "expired-token",
		ClientID:  "client-1",
		ExpiresAt: time.Now().Add(-1 * time.Second),
	})
	store.StoreRefreshToken(&RefreshData{
		Token:     "expired-refresh",
		ClientID:  "client-1",
		ExpiresAt: time.Now().Add(-1 * time.Second),
	})
	store.StoreProviderSession(&ProviderSession{
		State:     "expired-session",
		ClientID:  "client-1",
		ExpiresAt: time.Now().Add(-1 * time.Second),
	})

	// Register valid entries
	store.StoreAuthCode(&AuthCode{
		Code:      "valid-code",
		ClientID:  "client-2",
		ExpiresAt: time.Now().Add(5 * time.Minute),
	})
	store.StoreAccessToken(&TokenData{
		Token:     "valid-token",
		ClientID:  "client-2",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	})
	store.StoreRefreshToken(&RefreshData{
		Token:     "valid-refresh",
		ClientID:  "client-2",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})
	store.StoreProviderSession(&ProviderSession{
		State:     "valid-session",
		ClientID:  "client-2",
		ExpiresAt: time.Now().Add(10 * time.Minute),
	})

	store.CleanupExpired()

	// Expired entries should be gone
	if store.ConsumeAuthCode("expired-code") != nil {
		test.Error("expected expired auth code to be cleaned up")
	}
	if store.ValidateAccessToken("expired-token") {
		test.Error("expected expired access token to be cleaned up")
	}
	if store.ConsumeRefreshToken("expired-refresh") != nil {
		test.Error("expected expired refresh token to be cleaned up")
	}
	if store.GetProviderSession("expired-session") != nil {
		test.Error("expected expired provider session to be cleaned up")
	}

	// Valid entries should survive
	if store.ConsumeAuthCode("valid-code") == nil {
		test.Error("expected valid auth code to survive cleanup")
	}
	if !store.ValidateAccessToken("valid-token") {
		test.Error("expected valid access token to survive cleanup")
	}
	if store.ConsumeRefreshToken("valid-refresh") == nil {
		test.Error("expected valid refresh token to survive cleanup")
	}
	if store.GetProviderSession("valid-session") == nil {
		test.Error("expected valid provider session to survive cleanup")
	}
}
