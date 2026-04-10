package oauth

import (
	"testing"
)

// TestNewProviderConfig_Google verifies the Google provider has the correct name and auth URL.
func TestNewProviderConfig_Google(test *testing.T) {
	provider := NewGoogleProvider("google-client-id", "google-client-secret")

	if provider.Name != "google" {
		test.Errorf("expected name %q, got %q", "google", provider.Name)
	}
	if provider.AuthURL != "https://accounts.google.com/o/oauth2/v2/auth" {
		test.Errorf("unexpected AuthURL: %s", provider.AuthURL)
	}
	if provider.ClientID != "google-client-id" {
		test.Errorf("expected ClientID %q, got %q", "google-client-id", provider.ClientID)
	}
}

// TestNewProviderConfig_GitHub verifies the GitHub provider has the correct name and auth URL.
func TestNewProviderConfig_GitHub(test *testing.T) {
	provider := NewGitHubProvider("github-client-id", "github-client-secret")

	if provider.Name != "github" {
		test.Errorf("expected name %q, got %q", "github", provider.Name)
	}
	if provider.AuthURL != "https://github.com/login/oauth/authorize" {
		test.Errorf("unexpected AuthURL: %s", provider.AuthURL)
	}
}

// TestNewProviderConfig_Apple verifies the Apple provider has the correct name and auth URL.
func TestNewProviderConfig_Apple(test *testing.T) {
	provider := NewAppleProvider("apple-client-id", "apple-client-secret")

	if provider.Name != "apple" {
		test.Errorf("expected name %q, got %q", "apple", provider.Name)
	}
	if provider.AuthURL != "https://appleid.apple.com/auth/authorize" {
		test.Errorf("unexpected AuthURL: %s", provider.AuthURL)
	}
}

// TestBuildActiveProviders_OnlyConfigured verifies that only providers with both
// ID and secret configured are returned.
func TestBuildActiveProviders_OnlyConfigured(test *testing.T) {
	providers := BuildActiveProviders(
		"google-id", "google-secret",
		"", "",
		"apple-id", "apple-secret",
	)

	if len(providers) != 2 {
		test.Errorf("expected 2 providers, got %d", len(providers))
	}
}

// TestBuildActiveProviders_NoneConfigured verifies that an empty slice is returned
// when no providers are configured.
func TestBuildActiveProviders_NoneConfigured(test *testing.T) {
	providers := BuildActiveProviders("", "", "", "", "", "")

	if len(providers) != 0 {
		test.Errorf("expected 0 providers, got %d", len(providers))
	}
}
