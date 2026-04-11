package oauth

import (
	"encoding/json"
	"net/http"
)

// RegistrationRequest holds the client metadata submitted by a dynamic registration
// request as defined by RFC 7591.
type RegistrationRequest struct {
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

// RegistrationResponse is the server's confirmation of a successful dynamic client
// registration, including the assigned client_id.
type RegistrationResponse struct {
	ClientID                string   `json:"client_id"`
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

// NewRegistrationHandler returns an http.Handler implementing RFC 7591 dynamic client
// registration. It validates the request, generates a client_id, stores the client,
// and responds with 201 Created and the registration details.
func NewRegistrationHandler(store *Store) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var registrationReq RegistrationRequest
		decodeError := json.NewDecoder(request.Body).Decode(&registrationReq)
		if decodeError != nil {
			writeJSONError(writer, "invalid_client_metadata", http.StatusBadRequest)
			return
		}

		if len(registrationReq.RedirectURIs) == 0 {
			writeJSONError(writer, "invalid_client_metadata", http.StatusBadRequest)
			return
		}

		clientID, generateError := GenerateRandomToken(16)
		if generateError != nil {
			writeJSONError(writer, "server_error", http.StatusInternalServerError)
			return
		}

		client := &Client{
			ClientID:     clientID,
			ClientName:   registrationReq.ClientName,
			RedirectURIs: registrationReq.RedirectURIs,
		}
		store.RegisterClient(client)

		registrationResp := RegistrationResponse{
			ClientID:                clientID,
			ClientName:              registrationReq.ClientName,
			RedirectURIs:            registrationReq.RedirectURIs,
			GrantTypes:              registrationReq.GrantTypes,
			ResponseTypes:           registrationReq.ResponseTypes,
			TokenEndpointAuthMethod: registrationReq.TokenEndpointAuthMethod,
		}

		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		json.NewEncoder(writer).Encode(registrationResp) //nolint:errcheck
	})
}

// writeJSONError writes a JSON error response with the given error code and HTTP status.
func writeJSONError(writer http.ResponseWriter, errorCode string, statusCode int) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(statusCode)
	json.NewEncoder(writer).Encode(map[string]string{"error": errorCode}) //nolint:errcheck
}
