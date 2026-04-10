package oauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestMetadataHandler_ReturnsValidJSON verifies the metadata handler returns HTTP 200,
// a correct Content-Type header, and all required OAuth 2.1 server metadata fields.
func TestMetadataHandler_ReturnsValidJSON(test *testing.T) {
	const baseURL = "https://example.com"

	handler := NewMetadataHandler(baseURL)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		test.Fatalf("expected status 200, got %d", recorder.Code)
	}

	contentType := recorder.Header().Get("Content-Type")
	if contentType != "application/json" {
		test.Errorf("expected Content-Type %q, got %q", "application/json", contentType)
	}

	var metadata ServerMetadata
	decodeError := json.NewDecoder(recorder.Body).Decode(&metadata)
	if decodeError != nil {
		test.Fatalf("failed to decode response body: %v", decodeError)
	}

	if metadata.Issuer != baseURL {
		test.Errorf("expected issuer %q, got %q", baseURL, metadata.Issuer)
	}
	if metadata.AuthorizationEndpoint != baseURL+"/authorize" {
		test.Errorf("expected authorization_endpoint %q, got %q", baseURL+"/authorize", metadata.AuthorizationEndpoint)
	}
	if metadata.TokenEndpoint != baseURL+"/token" {
		test.Errorf("expected token_endpoint %q, got %q", baseURL+"/token", metadata.TokenEndpoint)
	}
	if metadata.RegistrationEndpoint != baseURL+"/register" {
		test.Errorf("expected registration_endpoint %q, got %q", baseURL+"/register", metadata.RegistrationEndpoint)
	}
}
