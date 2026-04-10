package oauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegistrationHandler_Success(test *testing.T) {
	store := NewStore()
	handler := NewRegistrationHandler(store)

	body := `{"client_name":"My App","redirect_uris":["https://example.com/callback"],"grant_types":["authorization_code"],"response_types":["code"],"token_endpoint_auth_method":"none"}`
	request := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(body))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		test.Errorf("expected status 201, got %d", recorder.Code)
	}

	var registrationResp RegistrationResponse
	decodeError := json.NewDecoder(recorder.Body).Decode(&registrationResp)
	if decodeError != nil {
		test.Fatalf("failed to decode response: %v", decodeError)
	}

	if registrationResp.ClientID == "" {
		test.Error("expected non-empty client_id in response")
	}

	if registrationResp.ClientName != "My App" {
		test.Errorf("expected client_name %q, got %q", "My App", registrationResp.ClientName)
	}

	storedClient := store.GetClient(registrationResp.ClientID)
	if storedClient == nil {
		test.Fatalf("expected client to be stored, got nil")
	}
	if storedClient.ClientID != registrationResp.ClientID {
		test.Errorf("expected stored ClientID %q, got %q", registrationResp.ClientID, storedClient.ClientID)
	}
}

func TestRegistrationHandler_MissingRedirectURIs(test *testing.T) {
	store := NewStore()
	handler := NewRegistrationHandler(store)

	body := `{"client_name":"Test"}`
	request := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(body))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		test.Errorf("expected status 400, got %d", recorder.Code)
	}
}

func TestRegistrationHandler_InvalidJSON(test *testing.T) {
	store := NewStore()
	handler := NewRegistrationHandler(store)

	request := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader("not json"))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		test.Errorf("expected status 400, got %d", recorder.Code)
	}
}
