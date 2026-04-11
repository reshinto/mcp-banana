// Package oauth provides OAuth 2.1 authorization support for Claude Desktop integration,
// including PKCE verification, token management, provider delegation, and dynamic client registration.
package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
)

// VerifyCodeChallenge validates a PKCE code challenge against a code verifier
// using the S256 method as required by the MCP OAuth spec.
func VerifyCodeChallenge(challenge string, verifier string, method string) bool {
	if method != "S256" {
		return false
	}
	hash := sha256.Sum256([]byte(verifier))
	expected := base64.RawURLEncoding.EncodeToString(hash[:])
	return challenge == expected
}

// randomSource is the reader used by generateToken to produce random bytes.
// It defaults to crypto/rand.Reader but can be replaced in tests to simulate failures.
var randomSource io.Reader = rand.Reader

// generateToken reads random bytes from randomSource and returns a hex-encoded string.
// Extracted so that both the happy and error paths can be covered by tests that swap
// randomSource instead of replacing the entire GenerateRandomToken var.
func generateToken(byteLength int) (string, error) {
	randomBytes := make([]byte, byteLength)
	_, readError := randomSource.Read(randomBytes)
	if readError != nil {
		return "", readError
	}
	return hex.EncodeToString(randomBytes), nil
}

// GenerateRandomToken is a package-level variable holding the function used to produce
// a cryptographically random hex-encoded token of the specified byte length.
// It is a var so tests can inject a failing implementation to exercise error paths.
var GenerateRandomToken = generateToken
