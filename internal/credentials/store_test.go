package credentials

import (
	"encoding/json"
	"fmt"
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

func TestNewStore_FailsWhenDirectoryNotWritable(test *testing.T) {
	tempDir := test.TempDir()
	readOnlyDir := filepath.Join(tempDir, "readonly")
	mkdirError := os.Mkdir(readOnlyDir, 0500)
	if mkdirError != nil {
		test.Fatalf("failed to create read-only directory: %v", mkdirError)
	}

	credPath := filepath.Join(readOnlyDir, "credentials.json")
	_, createError := NewStore(credPath)
	if createError == nil {
		test.Fatal("expected error when directory is not writable, got nil")
	}
}

func TestRegister_FailsOnMarshalError(test *testing.T) {
	tempDir := test.TempDir()
	credPath := filepath.Join(tempDir, "credentials.json")

	store, createError := NewStore(credPath)
	if createError != nil {
		test.Fatalf("NewStore returned error: %v", createError)
	}

	originalMarshal := jsonMarshalIndent
	defer func() { jsonMarshalIndent = originalMarshal }()

	jsonMarshalIndent = func(value any, prefix string, indent string) ([]byte, error) {
		return nil, fmt.Errorf("simulated marshal failure")
	}

	registerError := store.Register("identity-marshal", "key-marshal")
	if registerError == nil {
		test.Fatal("expected error from marshal failure, got nil")
	}
}

func TestRegister_FailsOnTempFileWriteError(test *testing.T) {
	tempDir := test.TempDir()
	credPath := filepath.Join(tempDir, "credentials.json")

	store, createError := NewStore(credPath)
	if createError != nil {
		test.Fatalf("NewStore returned error: %v", createError)
	}

	// Make the directory read-only so the temp file write fails.
	chmodError := os.Chmod(tempDir, 0500)
	if chmodError != nil {
		test.Fatalf("failed to chmod directory: %v", chmodError)
	}
	defer os.Chmod(tempDir, 0700) //nolint:errcheck

	registerError := store.Register("identity-write", "key-write")
	if registerError == nil {
		test.Fatal("expected error from temp file write failure, got nil")
	}
}

func TestRegister_FailsOnRenameError(test *testing.T) {
	tempDir := test.TempDir()
	subDir := filepath.Join(tempDir, "subdir")
	subDirError := os.Mkdir(subDir, 0700)
	if subDirError != nil {
		test.Fatalf("failed to create subdir: %v", subDirError)
	}

	credPath := filepath.Join(subDir, "credentials.json")

	store, createError := NewStore(credPath)
	if createError != nil {
		test.Fatalf("NewStore returned error: %v", createError)
	}

	// Point filePath to a different directory so rename crosses paths and fails.
	// We do this by writing the temp file to a location that prevents rename.
	// Trick: remove the credentials file and make the directory read-only
	// AFTER the temp file is written. We can't do that easily with the current
	// code, so instead we'll make the target a directory to force rename to fail.
	removeError := os.Remove(credPath)
	if removeError != nil {
		test.Fatalf("failed to remove credentials file: %v", removeError)
	}
	// Create a directory with the same name as the credentials file.
	mkdirError := os.Mkdir(credPath, 0700)
	if mkdirError != nil {
		test.Fatalf("failed to create directory at cred path: %v", mkdirError)
	}

	registerError := store.Register("identity-rename", "key-rename")
	if registerError == nil {
		test.Fatal("expected error from rename failure, got nil")
	}

	// Verify the temp file was cleaned up.
	tempPath := credPath + ".tmp"
	_, statError := os.Stat(tempPath)
	if !os.IsNotExist(statError) {
		test.Error("expected temp file to be cleaned up after rename failure")
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
