// Package credentials manages the unified credentials file that maps client
// identities (bearer tokens or OAuth provider:email pairs) to Gemini API keys.
// The file is re-read on every lookup for hot-reload support.
package credentials

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

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

// Register adds or updates the Gemini API key for the given identity. The write
// is atomic: data is written to a temporary file and then renamed over the
// original to prevent corruption from partial writes.
func (store *Store) Register(identity string, geminiAPIKey string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	entries := make(map[string]string)
	data, readError := os.ReadFile(store.filePath)
	if readError == nil {
		// A corrupt file is treated as empty to allow recovery via Register.
		json.Unmarshal(data, &entries) //nolint:errcheck
	}

	entries[identity] = geminiAPIKey

	serialized, marshalError := json.MarshalIndent(entries, "", "  ")
	if marshalError != nil {
		return fmt.Errorf("failed to serialize credentials: %w", marshalError)
	}

	// SECURITY: credentials file must be readable only by the owner.
	tempPath := store.filePath + ".tmp"
	if writeError := os.WriteFile(tempPath, serialized, 0600); writeError != nil {
		return fmt.Errorf("failed to write temp credentials file: %w", writeError)
	}

	if renameError := os.Rename(tempPath, store.filePath); renameError != nil {
		os.Remove(tempPath) //nolint:errcheck
		return fmt.Errorf("failed to rename credentials file: %w", renameError)
	}

	return nil
}

// Exists returns true if the given identity has a non-empty Gemini API key
// registered in the credentials file.
func (store *Store) Exists(identity string) bool {
	return store.Lookup(identity) != ""
}
