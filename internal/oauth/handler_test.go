package oauth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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

func TestAuthorizeHandler_InvalidClientID(test *testing.T) {
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

	if recorder.Code != http.StatusBadRequest {
		test.Errorf("expected status 400, got %d", recorder.Code)
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
