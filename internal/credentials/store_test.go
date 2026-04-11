package credentials

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewStore_CreatesFileIfMissing(test *testing.T) {
	tempDir := test.TempDir()
	credPath := filepath.Join(tempDir, "credentials.json")

	store, createError := NewStore(credPath)
	if createError != nil {
		test.Fatalf("NewStore returned error: %v", createError)
	}
	if store == nil {
		test.Fatal("NewStore returned nil store")
	}

	data, readError := os.ReadFile(credPath)
	if readError != nil {
		test.Fatalf("failed to read created file: %v", readError)
	}
	if string(data) != "{}" {
		test.Errorf("expected empty JSON object, got %q", string(data))
	}

	info, statError := os.Stat(credPath)
	if statError != nil {
		test.Fatalf("failed to stat created file: %v", statError)
	}
	if info.Mode().Perm() != 0600 {
		test.Errorf("expected file permissions 0600, got %o", info.Mode().Perm())
	}
}

func TestNewStore_LoadsExistingFile(test *testing.T) {
	tempDir := test.TempDir()
	credPath := filepath.Join(tempDir, "credentials.json")

	existing := map[string]string{"token-abc": "gemini-key-123"}
	serialized, _ := json.Marshal(existing)
	writeError := os.WriteFile(credPath, serialized, 0600)
	if writeError != nil {
		test.Fatalf("failed to write test file: %v", writeError)
	}

	store, createError := NewStore(credPath)
	if createError != nil {
		test.Fatalf("NewStore returned error: %v", createError)
	}

	apiKey := store.Lookup("token-abc")
	if apiKey != "gemini-key-123" {
		test.Errorf("expected gemini-key-123, got %q", apiKey)
	}
}

func TestNewStore_HandlesEmptyFile(test *testing.T) {
	tempDir := test.TempDir()
	credPath := filepath.Join(tempDir, "credentials.json")

	writeError := os.WriteFile(credPath, []byte("{}"), 0600)
	if writeError != nil {
		test.Fatalf("failed to write test file: %v", writeError)
	}

	store, createError := NewStore(credPath)
	if createError != nil {
		test.Fatalf("NewStore returned error: %v", createError)
	}
	if store == nil {
		test.Fatal("NewStore returned nil store")
	}
}

func TestNewStore_RejectsMalformedJSON(test *testing.T) {
	tempDir := test.TempDir()
	credPath := filepath.Join(tempDir, "credentials.json")

	writeError := os.WriteFile(credPath, []byte("{not valid json}"), 0600)
	if writeError != nil {
		test.Fatalf("failed to write test file: %v", writeError)
	}

	store, createError := NewStore(credPath)
	if createError == nil {
		test.Fatal("expected error for malformed JSON, got nil")
	}
	if store != nil {
		test.Error("expected nil store for malformed JSON")
	}
}

func TestRegister_AddsNewEntry(test *testing.T) {
	tempDir := test.TempDir()
	credPath := filepath.Join(tempDir, "credentials.json")

	store, createError := NewStore(credPath)
	if createError != nil {
		test.Fatalf("NewStore returned error: %v", createError)
	}

	registerError := store.Register("bearer:token-xyz", "gemini-key-456")
	if registerError != nil {
		test.Fatalf("Register returned error: %v", registerError)
	}

	apiKey := store.Lookup("bearer:token-xyz")
	if apiKey != "gemini-key-456" {
		test.Errorf("expected gemini-key-456, got %q", apiKey)
	}
}

func TestRegister_OverwritesExistingEntry(test *testing.T) {
	tempDir := test.TempDir()
	credPath := filepath.Join(tempDir, "credentials.json")

	store, createError := NewStore(credPath)
	if createError != nil {
		test.Fatalf("NewStore returned error: %v", createError)
	}

	_ = store.Register("identity-1", "old-key")
	registerError := store.Register("identity-1", "new-key")
	if registerError != nil {
		test.Fatalf("Register returned error: %v", registerError)
	}

	apiKey := store.Lookup("identity-1")
	if apiKey != "new-key" {
		test.Errorf("expected new-key, got %q", apiKey)
	}
}

func TestRegister_PreservesOtherEntries(test *testing.T) {
	tempDir := test.TempDir()
	credPath := filepath.Join(tempDir, "credentials.json")

	store, createError := NewStore(credPath)
	if createError != nil {
		test.Fatalf("NewStore returned error: %v", createError)
	}

	_ = store.Register("identity-a", "key-a")
	_ = store.Register("identity-b", "key-b")

	keyA := store.Lookup("identity-a")
	if keyA != "key-a" {
		test.Errorf("expected key-a, got %q", keyA)
	}
	keyB := store.Lookup("identity-b")
	if keyB != "key-b" {
		test.Errorf("expected key-b, got %q", keyB)
	}
}

func TestRegister_FilePermissions(test *testing.T) {
	tempDir := test.TempDir()
	credPath := filepath.Join(tempDir, "credentials.json")

	store, createError := NewStore(credPath)
	if createError != nil {
		test.Fatalf("NewStore returned error: %v", createError)
	}

	_ = store.Register("identity-perm", "key-perm")

	info, statError := os.Stat(credPath)
	if statError != nil {
		test.Fatalf("failed to stat file: %v", statError)
	}
	// SECURITY: credentials file must be readable only by the owner.
	if info.Mode().Perm() != 0600 {
		test.Errorf("expected file permissions 0600, got %o", info.Mode().Perm())
	}
}

func TestExists_ReturnsTrueForKnownIdentity(test *testing.T) {
	tempDir := test.TempDir()
	credPath := filepath.Join(tempDir, "credentials.json")

	store, createError := NewStore(credPath)
	if createError != nil {
		test.Fatalf("NewStore returned error: %v", createError)
	}

	_ = store.Register("known-identity", "some-key")

	if !store.Exists("known-identity") {
		test.Error("expected Exists to return true for known identity")
	}
}

func TestExists_ReturnsFalseForUnknownIdentity(test *testing.T) {
	tempDir := test.TempDir()
	credPath := filepath.Join(tempDir, "credentials.json")

	store, createError := NewStore(credPath)
	if createError != nil {
		test.Fatalf("NewStore returned error: %v", createError)
	}

	if store.Exists("unknown-identity") {
		test.Error("expected Exists to return false for unknown identity")
	}
}

func TestNewStore_FailsOnUnreadableFile(test *testing.T) {
	tempDir := test.TempDir()
	credPath := filepath.Join(tempDir, "credentials.json")

	// Create file then remove read permission.
	writeError := os.WriteFile(credPath, []byte("{}"), 0000)
	if writeError != nil {
		test.Fatalf("failed to write test file: %v", writeError)
	}

	_, createError := NewStore(credPath)
	if createError == nil {
		test.Fatal("expected error for unreadable file, got nil")
	}
}

func TestLookup_ReturnsEmptyOnUnreadableFile(test *testing.T) {
	tempDir := test.TempDir()
	credPath := filepath.Join(tempDir, "credentials.json")

	store, createError := NewStore(credPath)
	if createError != nil {
		test.Fatalf("NewStore returned error: %v", createError)
	}

	// Remove the file to simulate read failure.
	removeError := os.Remove(credPath)
	if removeError != nil {
		test.Fatalf("failed to remove file: %v", removeError)
	}

	apiKey := store.Lookup("any-identity")
	if apiKey != "" {
		test.Errorf("expected empty string, got %q", apiKey)
	}
}

func TestLookup_ReturnsEmptyOnCorruptedFile(test *testing.T) {
	tempDir := test.TempDir()
	credPath := filepath.Join(tempDir, "credentials.json")

	store, createError := NewStore(credPath)
	if createError != nil {
		test.Fatalf("NewStore returned error: %v", createError)
	}

	// Corrupt the file after store creation.
	corruptError := os.WriteFile(credPath, []byte("not json"), 0600)
	if corruptError != nil {
		test.Fatalf("failed to corrupt file: %v", corruptError)
	}

	apiKey := store.Lookup("any-identity")
	if apiKey != "" {
		test.Errorf("expected empty string, got %q", apiKey)
	}
}
