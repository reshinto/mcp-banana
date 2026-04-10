package oauth

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"time"
)

//go:embed login.html
var loginPageFS embed.FS

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
		client := store.GetClient(clientID)
		if client == nil {
			writeJSONError(writer, "invalid_client", http.StatusBadRequest)
			return
		}

		redirectURI := queryParams.Get("redirect_uri")
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
