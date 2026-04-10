package oauth

import (
	"encoding/json"
	"net/http"
)

// ServerMetadata represents the OAuth 2.1 authorization server metadata document
// as defined by RFC 8414 and the MCP OAuth specification.
type ServerMetadata struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	RegistrationEndpoint              string   `json:"registration_endpoint"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
}

// NewMetadataHandler returns an http.Handler that serves the OAuth 2.1 authorization
// server metadata document for the given baseURL. Claude Desktop uses this endpoint
// for automatic server discovery via the /.well-known/oauth-authorization-server path.
func NewMetadataHandler(baseURL string) http.Handler {
	metadata := ServerMetadata{
		Issuer:                            baseURL,
		AuthorizationEndpoint:             baseURL + "/authorize",
		TokenEndpoint:                     baseURL + "/token",
		RegistrationEndpoint:              baseURL + "/register",
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code", "refresh_token"},
		CodeChallengeMethodsSupported:     []string{"S256"},
		TokenEndpointAuthMethodsSupported: []string{"none"},
	}

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		encodeError := json.NewEncoder(writer).Encode(metadata)
		if encodeError != nil {
			http.Error(writer, "internal server error", http.StatusInternalServerError)
		}
	})
}
