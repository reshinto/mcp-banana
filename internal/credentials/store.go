// Package credentials manages the unified credentials file that maps client
// identities (bearer tokens or OAuth provider:email pairs) to Gemini API keys.
// The file is re-read on every lookup for hot-reload support.
package credentials

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
)

// jsonMarshalIndent is the function used to serialize credentials to JSON.
// It is a package-level variable so tests can inject failures for the marshal
// error path that is otherwise unreachable with map[string]string values.
var jsonMarshalIndent = json.MarshalIndent

// osChmod is the function used to set file permissions. It is a package-level
// variable so tests can inject failures for the chmod error path.
var osChmod = os.Chmod

// Store provides thread-safe access to a JSON credentials file that maps
// client identities to Gemini API keys.
type Store struct {
	filePath string
	mutex    sync.Mutex
}

// NewStore opens or creates a credentials file at the given path. If the file
// does not exist, it is created with an empty JSON object and 0600 permissions.
// If the file exists, its contents are validated as valid JSON.
func NewStore(filePath string) (*Store, error) {
	store := &Store{filePath: filePath}

	if _, statError := os.Stat(filePath); os.IsNotExist(statError) {
		// SECURITY: credentials file must be readable only by the owner.
		if writeError := os.WriteFile(filePath, []byte("{}"), 0600); writeError != nil {
			return nil, fmt.Errorf("failed to create credentials file: %w", writeError)
		}
		return store, nil
	}

	data, readError := os.ReadFile(filePath)
	if readError != nil {
		return nil, fmt.Errorf("failed to read credentials file: %w", readError)
	}

	var entries map[string]string
	if parseError := json.Unmarshal(data, &entries); parseError != nil {
		return nil, fmt.Errorf("credentials file contains invalid JSON: %w", parseError)
	}

	// SECURITY: Tighten permissions on pre-existing files. os.WriteFile only
	// sets permissions on creation, not on existing files. An existing file
	// with loose permissions (e.g. 0644) would expose all bearer tokens and
	// Gemini API keys to other users on the system.
	if chmodError := osChmod(filePath, 0600); chmodError != nil {
		return nil, fmt.Errorf("failed to set credentials file permissions: %w", chmodError)
	}

	return store, nil
}

// Lookup returns the Gemini API key associated with the given identity, or an
// empty string if the identity is not found. The file is re-read on every call
// to support hot-reload without restart. The mutex prevents races with
// concurrent Register calls.
func (store *Store) Lookup(identity string) string {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	data, readError := os.ReadFile(store.filePath)
	if readError != nil {
		return ""
	}

	var entries map[string]string
	if parseError := json.Unmarshal(data, &entries); parseError != nil {
		return ""
	}

	return entries[identity]
}

// Register adds or updates the Gemini API key for the given identity.
// The mutex prevents concurrent writes from corrupting the file.
// Writes directly to the file instead of temp+rename because Docker bind
// mounts do not support atomic rename (rename returns "device or resource busy").
func (store *Store) Register(identity string, geminiAPIKey string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	entries := make(map[string]string)
	data, readError := os.ReadFile(store.filePath)
	if readError == nil {
		// A corrupt file is treated as empty to allow recovery via Register.
		if unmarshalError := json.Unmarshal(data, &entries); unmarshalError != nil {
			slog.Warn("credentials file is corrupt, treating as empty", "error", unmarshalError)
		}
	}

	entries[identity] = geminiAPIKey

	serialized, marshalError := jsonMarshalIndent(entries, "", "  ")
	if marshalError != nil {
		return fmt.Errorf("failed to serialize credentials: %w", marshalError)
	}

	// SECURITY: credentials file must be readable only by the owner.
	if writeError := os.WriteFile(store.filePath, serialized, 0600); writeError != nil {
		return fmt.Errorf("failed to write credentials file: %w", writeError)
	}

	return nil
}

// Exists returns true if the given identity has a non-empty Gemini API key
// registered in the credentials file.
func (store *Store) Exists(identity string) bool {
	return store.Lookup(identity) != ""
}
