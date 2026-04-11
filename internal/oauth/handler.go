package oauth

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/reshinto/mcp-banana/internal/credentials"
)

//go:embed login.html
var loginPageFS embed.FS

//go:embed gemini_prompt.html
var geminiPromptFS embed.FS

// CredentialsStore is the interface that the OAuth handlers use to read and write
// credentials. It is satisfied by *credentials.Store.
type CredentialsStore interface {
	Lookup(identity string) string
	Exists(identity string) bool
	Register(identity string, geminiAPIKey string) error
}

// ProviderLink holds the display information and redirect URL for a single
// upstream identity provider shown on the login page.
type ProviderLink struct {
	Name            string
	DisplayName     string
	AuthRedirectURL string
}

// NewAuthorizeHandler returns an http.Handler for the /authorize endpoint.
// It validates PKCE parameters and client registration, then renders the login page
// with per-provider state tokens and redirect URLs.
func NewAuthorizeHandler(store *Store, providers []ProviderConfig, baseURL string) http.Handler {
	loginTemplate := template.Must(template.ParseFS(loginPageFS, "login.html"))

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		queryParams := request.URL.Query()

		codeChallenge := queryParams.Get("code_challenge")
		codeChallengeMethod := queryParams.Get("code_challenge_method")

		if codeChallenge == "" || codeChallengeMethod != "S256" {
			writeJSONError(writer, "invalid_request", http.StatusBadRequest)
			return
		}

		clientID := queryParams.Get("client_id")
		redirectURI := queryParams.Get("redirect_uri")

		client := store.GetClient(clientID)
		if client == nil {
			// Auto-register unknown clients. MCP clients like Claude Desktop
			// may cache their client_id across server restarts, but the
			// in-memory store loses registrations on restart. Rather than
			// rejecting with invalid_client, register on the fly.
			if clientID == "" || redirectURI == "" {
				writeJSONError(writer, "invalid_client", http.StatusBadRequest)
				return
			}
			client = &Client{
				ClientID:     clientID,
				ClientName:   "auto-registered",
				RedirectURIs: []string{redirectURI},
			}
			store.RegisterClient(client)
		}

		if !uriInList(redirectURI, client.RedirectURIs) {
			writeJSONError(writer, "invalid_redirect_uri", http.StatusBadRequest)
			return
		}

		originalState := queryParams.Get("state")

		providerLinks := make([]ProviderLink, 0, len(providers))
		for _, provider := range providers {
			providerState, generateError := GenerateRandomToken(16)
			if generateError != nil {
				writeJSONError(writer, "server_error", http.StatusInternalServerError)
				return
			}

			session := &ProviderSession{
				State:         providerState,
				ClientID:      clientID,
				RedirectURI:   redirectURI,
				CodeChallenge: codeChallenge,
				OriginalState: originalState,
				Provider:      provider.Name,
				ExpiresAt:     time.Now().Add(10 * time.Minute),
			}
			store.StoreProviderSession(session)

			authRedirectURL := buildProviderAuthURL(provider, baseURL, providerState)

			providerLinks = append(providerLinks, ProviderLink{
				Name:            provider.Name,
				DisplayName:     provider.DisplayName,
				AuthRedirectURL: authRedirectURL,
			})
		}

		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		writer.WriteHeader(http.StatusOK)
		renderError := loginTemplate.Execute(writer, map[string]interface{}{
			"Providers": providerLinks,
		})
		if renderError != nil {
			http.Error(writer, "internal server error", http.StatusInternalServerError)
		}
	})
}

// uriInList returns true if the target URI is present in the allowed list.
func uriInList(target string, allowed []string) bool {
	for _, candidate := range allowed {
		if candidate == target {
			return true
		}
	}
	return false
}

// buildProviderAuthURL constructs the redirect URL for the given upstream provider,
// embedding the callback URL and PKCE state parameter.
func buildProviderAuthURL(provider ProviderConfig, baseURL string, state string) string {
	params := url.Values{}
	params.Set("client_id", provider.ClientID)
	params.Set("redirect_uri", baseURL+"/callback")
	params.Set("response_type", "code")
	params.Set("state", state)

	scopeString := ""
	for scopeIndex, scope := range provider.Scopes {
		if scopeIndex > 0 {
			scopeString += " "
		}
		scopeString += scope
	}
	if scopeString != "" {
		params.Set("scope", scopeString)
	}

	return fmt.Sprintf("%s?%s", provider.AuthURL, params.Encode())
}

// exchangeProviderCode is the package-level function used to exchange an upstream provider
// authorization code for a provider access token. It is a variable so tests can replace it
// with a mock without making real HTTP calls.
var exchangeProviderCode = func(provider ProviderConfig, code string, callbackURL string) (string, error) {
	formData := url.Values{}
	formData.Set("grant_type", "authorization_code")
	formData.Set("code", code)
	formData.Set("redirect_uri", callbackURL)
	formData.Set("client_id", provider.ClientID)
	formData.Set("client_secret", provider.ClientSecret)

	// GitHub requires Accept: application/json to return JSON instead of
	// form-encoded response from the token endpoint.
	tokenRequest, requestError := http.NewRequest("POST", provider.TokenURL, strings.NewReader(formData.Encode()))
	if requestError != nil {
		return "", fmt.Errorf("failed to create token request: %w", requestError)
	}
	tokenRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenRequest.Header.Set("Accept", "application/json")

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, postError := httpClient.Do(tokenRequest)
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
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if decodeError := json.NewDecoder(resp.Body).Decode(&tokenResponse); decodeError != nil {
		return "", fmt.Errorf("failed to decode provider token response: %w", decodeError)
	}

	// GitHub returns 200 OK with an error field in the JSON body instead of
	// using HTTP status codes for token exchange failures.
	if tokenResponse.Error != "" {
		return "", fmt.Errorf("provider token error: %s — %s", tokenResponse.Error, tokenResponse.ErrorDesc)
	}

	if tokenResponse.AccessToken == "" {
		return "", errors.New("provider returned empty access token")
	}

	return tokenResponse.AccessToken, nil
}

// providerIdentityFetcher is the package-level function used to fetch the user's identity
// from the upstream provider's UserInfo endpoint. It is a variable so tests can replace it.
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

// fetchProviderIdentity retrieves the authenticated user's identity from the upstream
// provider using the provider access token obtained during code exchange.
func fetchProviderIdentity(provider ProviderConfig, providerAccessToken string) (string, error) {
	return providerIdentityFetcher(provider, providerAccessToken)
}

// NewCallbackHandler returns an http.Handler for the OAuth provider callback endpoint.
// It handles GET callbacks (Google, GitHub) and POST callbacks (Apple), validates the
// state parameter, exchanges the provider code for a provider access token, fetches
// the user identity, and redirects to the Gemini key prompt page.
func NewCallbackHandler(store *Store, providers []ProviderConfig, baseURL string, credStore CredentialsStore) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var code, state string

		if request.Method == http.MethodPost {
			if parseError := request.ParseForm(); parseError != nil {
				writeJSONError(writer, "invalid_request", http.StatusBadRequest)
				return
			}
			code = request.PostForm.Get("code")
			state = request.PostForm.Get("state")
		} else {
			queryParams := request.URL.Query()
			code = queryParams.Get("code")
			state = queryParams.Get("state")
		}

		session := store.GetProviderSession(state)
		if session == nil {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(writer).Encode(map[string]string{ //nolint:errcheck
				"error":             "invalid_request",
				"error_description": "invalid or expired state",
			})
			return
		}

		var matchedProvider *ProviderConfig
		for providerIndex := range providers {
			if providers[providerIndex].Name == session.Provider {
				matchedProvider = &providers[providerIndex]
				break
			}
		}
		if matchedProvider == nil {
			writeJSONError(writer, "server_error", http.StatusInternalServerError)
			return
		}

		callbackURL := baseURL + "/callback"
		providerAccessToken, exchangeError := exchangeProviderCode(*matchedProvider, code, callbackURL)
		if exchangeError != nil {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(writer).Encode(map[string]string{ //nolint:errcheck
				"error":             "server_error",
				"error_description": "provider token exchange failed",
			})
			return
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
	})
}

// NewTokenHandler returns an http.Handler for the /token endpoint.
// It supports the authorization_code and refresh_token grant types as defined by OAuth 2.1.
func NewTokenHandler(store *Store) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if parseError := request.ParseForm(); parseError != nil {
			writeJSONError(writer, "invalid_request", http.StatusBadRequest)
			return
		}

		grantType := request.PostForm.Get("grant_type")

		switch grantType {
		case "authorization_code":
			handleAuthorizationCodeGrant(writer, request, store)
		case "refresh_token":
			handleRefreshTokenGrant(writer, request, store)
		default:
			writeJSONError(writer, "unsupported_grant_type", http.StatusBadRequest)
		}
	})
}

// handleAuthorizationCodeGrant processes the authorization_code grant type,
// verifying the code, client, redirect URI, and PKCE challenge before issuing tokens.
func handleAuthorizationCodeGrant(writer http.ResponseWriter, request *http.Request, store *Store) {
	code := request.PostForm.Get("code")
	clientID := request.PostForm.Get("client_id")
	redirectURI := request.PostForm.Get("redirect_uri")
	codeVerifier := request.PostForm.Get("code_verifier")

	authCode := store.ConsumeAuthCode(code)
	if authCode == nil {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(writer).Encode(map[string]string{ //nolint:errcheck
			"error":             "invalid_grant",
			"error_description": "invalid or expired code",
		})
		return
	}

	if authCode.ClientID != clientID || authCode.RedirectURI != redirectURI {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(writer).Encode(map[string]string{ //nolint:errcheck
			"error":             "invalid_grant",
			"error_description": "client_id or redirect_uri mismatch",
		})
		return
	}

	if !VerifyCodeChallenge(authCode.CodeChallenge, codeVerifier, "S256") {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(writer).Encode(map[string]string{ //nolint:errcheck
			"error":             "invalid_grant",
			"error_description": "PKCE verification failed",
		})
		return
	}

	issueTokenResponse(writer, store, clientID, authCode.ProviderIdentity)
}

// handleRefreshTokenGrant processes the refresh_token grant type,
// consuming the existing refresh token and issuing a new token pair.
func handleRefreshTokenGrant(writer http.ResponseWriter, request *http.Request, store *Store) {
	refreshToken := request.PostForm.Get("refresh_token")
	clientID := request.PostForm.Get("client_id")

	refreshData := store.ConsumeRefreshToken(refreshToken)
	if refreshData == nil {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(writer).Encode(map[string]string{ //nolint:errcheck
			"error":             "invalid_grant",
			"error_description": "invalid or expired refresh token",
		})
		return
	}

	if refreshData.ClientID != clientID {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(writer).Encode(map[string]string{ //nolint:errcheck
			"error":             "invalid_grant",
			"error_description": "client_id mismatch",
		})
		return
	}

	issueTokenResponse(writer, store, clientID, refreshData.ProviderIdentity)
}

// issueTokenResponse generates a new access/refresh token pair, persists them,
// and writes the token response JSON to the client.
func issueTokenResponse(writer http.ResponseWriter, store *Store, clientID string, providerIdentity string) {
	accessToken, accessError := GenerateRandomToken(32)
	if accessError != nil {
		writeJSONError(writer, "server_error", http.StatusInternalServerError)
		return
	}

	newRefreshToken, refreshError := GenerateRandomToken(32)
	if refreshError != nil {
		writeJSONError(writer, "server_error", http.StatusInternalServerError)
		return
	}

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

	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	json.NewEncoder(writer).Encode(map[string]interface{}{ //nolint:errcheck
		"access_token":  accessToken,
		"token_type":    "Bearer",
		"expires_in":    3600,
		"refresh_token": newRefreshToken,
	})
}

// geminiPromptData holds the template variables for the Gemini key prompt page.
type geminiPromptData struct {
	Identity     string
	IsReturning  bool
	SessionToken string
	Error        string
}

// NewGeminiKeyPromptHandler returns an http.Handler that renders the Gemini API key
// prompt form. It reads session, error, and returning from query parameters and uses
// PeekGeminiKeySession to retrieve the user identity.
func NewGeminiKeyPromptHandler(store *Store) http.Handler {
	promptTemplate := template.Must(template.ParseFS(geminiPromptFS, "gemini_prompt.html"))

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		queryParams := request.URL.Query()
		sessionToken := queryParams.Get("session")
		errorMessage := queryParams.Get("error")
		isReturning := queryParams.Get("returning") == "true"

		session := store.PeekGeminiKeySession(sessionToken)
		if session == nil {
			writeJSONError(writer, "invalid_request", http.StatusBadRequest)
			return
		}

		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		writer.WriteHeader(http.StatusOK)
		renderError := promptTemplate.Execute(writer, geminiPromptData{
			Identity:     session.ProviderIdentity,
			IsReturning:  isReturning,
			SessionToken: sessionToken,
			Error:        errorMessage,
		})
		if renderError != nil {
			http.Error(writer, "internal server error", http.StatusInternalServerError)
		}
	})
}

// NewGeminiKeySubmitHandler returns an http.Handler that processes the Gemini API key
// form submission. It validates the key, registers it in the credentials store, and
// redirects the client to complete the OAuth flow with an authorization code.
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

		if action == "skip" {
			if !credStore.Exists(session.ProviderIdentity) {
				newToken, generateError := GenerateRandomToken(16)
				if generateError != nil {
					writeJSONError(writer, "server_error", http.StatusInternalServerError)
					return
				}
				session.ExpiresAt = time.Now().Add(10 * time.Minute)
				store.StoreGeminiKeySession(newToken, session)
				http.Redirect(writer, request, "/gemini-key?session="+newToken+"&returning=true&error=No+existing+key+found.+Please+provide+a+Gemini+API+key.", http.StatusFound)
				return
			}
			issueAuthCodeRedirect(writer, request, store, session)
			return
		}

		if geminiAPIKey == "" {
			newToken, generateError := GenerateRandomToken(16)
			if generateError != nil {
				writeJSONError(writer, "server_error", http.StatusInternalServerError)
				return
			}
			returningParam := ""
			if credStore.Exists(session.ProviderIdentity) {
				returningParam = "&returning=true"
			}
			session.ExpiresAt = time.Now().Add(10 * time.Minute)
			store.StoreGeminiKeySession(newToken, session)
			http.Redirect(writer, request, "/gemini-key?session="+newToken+returningParam+"&error=Gemini+API+key+must+not+be+empty", http.StatusFound)
			return
		}

		ctx := context.Background()
		validationError := credentials.ValidateGeminiKey(ctx, geminiAPIKey)
		if validationError != nil {
			newToken, generateError := GenerateRandomToken(16)
			if generateError != nil {
				writeJSONError(writer, "server_error", http.StatusInternalServerError)
				return
			}
			returningParam := ""
			if credStore.Exists(session.ProviderIdentity) {
				returningParam = "&returning=true"
			}
			session.ExpiresAt = time.Now().Add(10 * time.Minute)
			store.StoreGeminiKeySession(newToken, session)
			http.Redirect(writer, request, "/gemini-key?session="+newToken+returningParam+"&error=Invalid+Gemini+API+key", http.StatusFound)
			return
		}

		registerError := credStore.Register(session.ProviderIdentity, geminiAPIKey)
		if registerError != nil {
			writeJSONError(writer, "server_error", http.StatusInternalServerError)
			return
		}

		issueAuthCodeRedirect(writer, request, store, session)
	})
}

// issueAuthCodeRedirect generates an MCP authorization code, stores it, and redirects
// the user-agent back to the client's redirect URI with the code and state parameters.
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
