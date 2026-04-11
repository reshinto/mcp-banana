package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestVerifyCodeChallenge_ValidS256(test *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])

	result := VerifyCodeChallenge(challenge, verifier, "S256")
	if !result {
		test.Errorf("expected valid PKCE challenge to pass, got false")
	}
}

func TestVerifyCodeChallenge_InvalidVerifier(test *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])

	result := VerifyCodeChallenge(challenge, "wrong-verifier", "S256")
	if result {
		test.Errorf("expected invalid verifier to fail, got true")
	}
}

func TestVerifyCodeChallenge_UnsupportedMethod(test *testing.T) {
	result := VerifyCodeChallenge("challenge", "verifier", "plain")
	if result {
		test.Errorf("expected unsupported method to fail, got true")
	}
}

func TestGenerateRandomToken_Length(test *testing.T) {
	token, tokenError := GenerateRandomToken(32)
	if tokenError != nil {
		test.Fatalf("unexpected error: %v", tokenError)
	}
	if len(token) == 0 {
		test.Errorf("expected non-empty token")
	}
}

func TestGenerateRandomToken_Unique(test *testing.T) {
	tokenOne, _ := GenerateRandomToken(32)
	tokenTwo, _ := GenerateRandomToken(32)
	if tokenOne == tokenTwo {
		test.Errorf("expected unique tokens, got identical: %s", tokenOne)
	}
}
