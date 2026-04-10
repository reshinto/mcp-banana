package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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

// GenerateRandomToken produces a cryptographically random hex-encoded token
// of the specified byte length.
func GenerateRandomToken(byteLength int) (string, error) {
	randomBytes := make([]byte, byteLength)
	_, readError := rand.Read(randomBytes)
	if readError != nil {
		return "", readError
	}
	return hex.EncodeToString(randomBytes), nil
}
