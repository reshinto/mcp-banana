package oauth

// ProviderConfig holds the configuration for an upstream OAuth 2.0 identity provider,
// including endpoints, credentials, and requested scopes.
type ProviderConfig struct {
	Name         string
	DisplayName  string
	AuthURL      string
	TokenURL     string
	UserInfoURL  string
	ClientID     string
	ClientSecret string
	Scopes       []string
}

// NewGoogleProvider returns a ProviderConfig pre-configured for Google OAuth 2.0.
func NewGoogleProvider(clientID, clientSecret string) ProviderConfig {
	return ProviderConfig{
		Name:         "google",
		DisplayName:  "Google",
		AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:     "https://oauth2.googleapis.com/token",
		UserInfoURL:  "https://www.googleapis.com/oauth2/v2/userinfo",
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       []string{"openid", "email"},
	}
}

// NewGitHubProvider returns a ProviderConfig pre-configured for GitHub OAuth 2.0.
func NewGitHubProvider(clientID, clientSecret string) ProviderConfig {
	return ProviderConfig{
		Name:         "github",
		DisplayName:  "GitHub",
		AuthURL:      "https://github.com/login/oauth/authorize",
		TokenURL:     "https://github.com/login/oauth/access_token",
		UserInfoURL:  "https://api.github.com/user",
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       []string{"read:user", "user:email"},
	}
}

// NewAppleProvider returns a ProviderConfig pre-configured for Sign in with Apple.
// Apple does not expose a userinfo endpoint; the user claims are returned in the ID token.
func NewAppleProvider(clientID, clientSecret string) ProviderConfig {
	return ProviderConfig{
		Name:         "apple",
		DisplayName:  "Apple",
		AuthURL:      "https://appleid.apple.com/auth/authorize",
		TokenURL:     "https://appleid.apple.com/auth/token",
		UserInfoURL:  "",
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       []string{"name", "email"},
	}
}

// BuildActiveProviders constructs the list of providers for which both a client ID
// and a client secret have been supplied. Providers with either value empty are omitted.
func BuildActiveProviders(
	googleClientID, googleClientSecret,
	githubClientID, githubClientSecret,
	appleClientID, appleClientSecret string,
) []ProviderConfig {
	candidates := []ProviderConfig{
		NewGoogleProvider(googleClientID, googleClientSecret),
		NewGitHubProvider(githubClientID, githubClientSecret),
		NewAppleProvider(appleClientID, appleClientSecret),
	}

	active := make([]ProviderConfig, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.ClientID != "" && candidate.ClientSecret != "" {
			active = append(active, candidate)
		}
	}
	return active
}
