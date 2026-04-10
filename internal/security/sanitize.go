package security

import (
	"regexp"
	"strings"
	"sync"
)

const redacted = "[REDACTED]"

// geminiAPIKeyPattern matches Gemini API keys of the form AIza followed by 35 alphanumeric/dash/underscore characters.
var geminiAPIKeyPattern = regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`)

var (
	secretsMutex      sync.RWMutex
	registeredSecrets []string
)

// RegisterSecret registers a secret value that will be redacted from all output produced by SanitizeString.
// It is thread-safe and should be called at startup with sensitive values such as API keys and auth tokens.
// Empty strings are ignored.
func RegisterSecret(secret string) {
	if secret == "" {
		return
	}
	secretsMutex.Lock()
	defer secretsMutex.Unlock()
	registeredSecrets = append(registeredSecrets, secret)
}

// ClearSecrets removes all registered secrets. It is intended for use in tests only, to prevent
// state leakage between test functions.
func ClearSecrets() {
	secretsMutex.Lock()
	defer secretsMutex.Unlock()
	registeredSecrets = nil
}

// SanitizeString sanitizes the input string by:
//  1. Replacing any registered secret values with [REDACTED].
//  2. Replacing any Gemini API key pattern (AIza followed by 35 alphanumeric/dash/underscore chars) with [REDACTED].
//  3. Stripping newline (\n) and carriage return (\r) characters to prevent log injection.
//
// It is thread-safe.
func SanitizeString(input string) string {
	secretsMutex.RLock()
	secrets := make([]string, len(registeredSecrets))
	copy(secrets, registeredSecrets)
	secretsMutex.RUnlock()

	output := input
	for _, secret := range secrets {
		output = strings.ReplaceAll(output, secret, redacted)
	}

	output = geminiAPIKeyPattern.ReplaceAllString(output, redacted)
	output = strings.ReplaceAll(output, "\n", "")
	output = strings.ReplaceAll(output, "\r", "")

	return output
}
