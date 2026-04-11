package oauth

import (
	"sync"
	"time"
)

// Client represents a registered OAuth 2.1 client application.
type Client struct {
	ClientID     string   `json:"client_id"`
	ClientName   string   `json:"client_name"`
	RedirectURIs []string `json:"redirect_uris"`
}

// AuthCode represents a single-use OAuth 2.1 authorization code with PKCE support.
type AuthCode struct {
	Code             string
	ClientID         string
	RedirectURI      string
	CodeChallenge    string
	ProviderIdentity string // "provider:email" identity from upstream OAuth
	ExpiresAt        time.Time
}

// TokenData holds an issued access token and its associated metadata.
type TokenData struct {
	Token            string
	ClientID         string
	ProviderIdentity string // "provider:email" identity from upstream OAuth
	ExpiresAt        time.Time
}

// RefreshData holds a single-use refresh token and its associated metadata.
type RefreshData struct {
	Token            string
	ClientID         string
	ProviderIdentity string // "provider:email" identity from upstream OAuth
	ExpiresAt        time.Time
}

// GeminiKeySession stores the pending state between the OAuth callback and the
// Gemini key submission form.
type GeminiKeySession struct {
	ProviderIdentity string
	ClientID         string
	RedirectURI      string
	CodeChallenge    string
	OriginalState    string
	ExpiresAt        time.Time
}

// ProviderSession tracks an in-flight upstream OAuth provider authorization,
// keyed by the state parameter sent to the provider.
type ProviderSession struct {
	State         string
	ClientID      string
	RedirectURI   string
	CodeChallenge string
	OriginalState string
	Provider      string
	ExpiresAt     time.Time
}

// Store is a thread-safe in-memory store for OAuth 2.1 state including clients,
// authorization codes, access tokens, refresh tokens, and provider sessions.
// Auth codes, refresh tokens, and provider sessions are single-use and expire by TTL.
type Store struct {
	mutex             sync.RWMutex
	clients           map[string]*Client
	authCodes         map[string]*AuthCode
	accessTokens      map[string]*TokenData
	refreshTokens     map[string]*RefreshData
	providerSessions  map[string]*ProviderSession
	geminiKeySessions map[string]*GeminiKeySession
}

// NewStore creates and initializes an empty OAuth store.
func NewStore() *Store {
	return &Store{
		clients:           make(map[string]*Client),
		authCodes:         make(map[string]*AuthCode),
		accessTokens:      make(map[string]*TokenData),
		refreshTokens:     make(map[string]*RefreshData),
		providerSessions:  make(map[string]*ProviderSession),
		geminiKeySessions: make(map[string]*GeminiKeySession),
	}
}

// RegisterClient adds a client to the store, keyed by its ClientID.
func (store *Store) RegisterClient(client *Client) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.clients[client.ClientID] = client
}

// GetClient returns the client with the given clientID, or nil if not found.
func (store *Store) GetClient(clientID string) *Client {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	return store.clients[clientID]
}

// StoreAuthCode persists an authorization code for later single-use consumption.
func (store *Store) StoreAuthCode(authCode *AuthCode) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.authCodes[authCode.Code] = authCode
}

// ConsumeAuthCode retrieves and removes an authorization code by value.
// Returns nil if the code does not exist or has expired (single-use semantics).
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

// StoreAccessToken persists an access token for later validation.
func (store *Store) StoreAccessToken(tokenData *TokenData) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.accessTokens[tokenData.Token] = tokenData
}

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

// ValidateAccessToken returns true if the token exists and has not expired.
func (store *Store) ValidateAccessToken(token string) bool {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	tokenData, exists := store.accessTokens[token]
	if !exists {
		return false
	}
	return time.Now().Before(tokenData.ExpiresAt)
}

// StoreRefreshToken persists a refresh token for later single-use consumption.
func (store *Store) StoreRefreshToken(refreshData *RefreshData) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.refreshTokens[refreshData.Token] = refreshData
}

// ConsumeRefreshToken retrieves and removes a refresh token by value.
// Returns nil if the token does not exist or has expired (single-use semantics).
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

// StoreProviderSession persists an upstream provider session keyed by its state parameter.
func (store *Store) StoreProviderSession(session *ProviderSession) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.providerSessions[session.State] = session
}

// GetProviderSession retrieves and removes a provider session by state (consumed on read).
// Returns nil if the session does not exist or has expired.
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

// StoreGeminiKeySession persists a Gemini key session keyed by its token.
func (store *Store) StoreGeminiKeySession(token string, session *GeminiKeySession) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.geminiKeySessions[token] = session
}

// ConsumeGeminiKeySession retrieves and removes a Gemini key session by token.
// Returns nil if the session does not exist or has expired (single-use semantics).
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

// PeekGeminiKeySession reads a session without consuming it.
func (store *Store) PeekGeminiKeySession(token string) *GeminiKeySession {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	session, exists := store.geminiKeySessions[token]
	if !exists {
		return nil
	}
	if time.Now().After(session.ExpiresAt) {
		return nil
	}
	return session
}

// CleanupExpired removes all expired entries from every map in the store.
func (store *Store) CleanupExpired() {
	now := time.Now()
	store.mutex.Lock()
	defer store.mutex.Unlock()
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
	for token, session := range store.geminiKeySessions {
		if now.After(session.ExpiresAt) {
			delete(store.geminiKeySessions, token)
		}
	}
}
