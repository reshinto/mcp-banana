package security_test

import (
	"encoding/base64"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/reshinto/mcp-banana/internal/security"
)

// TestValidatePrompt_Valid checks that a normal prompt passes validation.
func TestValidatePrompt_Valid(test *testing.T) {
	err := security.ValidatePrompt("Generate a banana image")
	if err != nil {
		test.Errorf("expected nil error, got: %v", err)
	}
}

// TestValidatePrompt_Empty checks that an empty prompt is rejected.
func TestValidatePrompt_Empty(test *testing.T) {
	err := security.ValidatePrompt("")
	if err == nil {
		test.Error("expected error for empty prompt, got nil")
	}
}

// TestValidatePrompt_TooLong checks that a prompt exceeding 10000 runes is rejected.
func TestValidatePrompt_TooLong(test *testing.T) {
	longPrompt := strings.Repeat("a", 10001)
	if utf8.RuneCountInString(longPrompt) != 10001 {
		test.Fatal("test setup error: longPrompt is not 10001 runes")
	}
	err := security.ValidatePrompt(longPrompt)
	if err == nil {
		test.Error("expected error for prompt with 10001 runes, got nil")
	}
}

// TestValidatePrompt_ExactlyAtLimit checks that a prompt of exactly 10000 runes is accepted.
func TestValidatePrompt_ExactlyAtLimit(test *testing.T) {
	boundaryPrompt := strings.Repeat("a", 10000)
	if utf8.RuneCountInString(boundaryPrompt) != 10000 {
		test.Fatal("test setup error: boundaryPrompt is not 10000 runes")
	}
	err := security.ValidatePrompt(boundaryPrompt)
	if err != nil {
		test.Errorf("expected nil error for 10000-rune prompt, got: %v", err)
	}
}

// TestValidatePrompt_NullByte checks that a prompt containing a null byte is rejected.
func TestValidatePrompt_NullByte(test *testing.T) {
	err := security.ValidatePrompt("hello\x00world")
	if err == nil {
		test.Error("expected error for prompt with null byte, got nil")
	}
}

// TestValidateModelAlias_Empty checks that an empty alias is accepted (use default).
func TestValidateModelAlias_Empty(test *testing.T) {
	err := security.ValidateModelAlias("")
	if err != nil {
		test.Errorf("expected nil error for empty alias, got: %v", err)
	}
}

// TestValidateModelAlias_ValidAliases checks all known valid model aliases.
func TestValidateModelAlias_ValidAliases(test *testing.T) {
	validAliases := []string{"nano-banana-2", "nano-banana-original", "nano-banana-pro"}
	for _, alias := range validAliases {
		err := security.ValidateModelAlias(alias)
		if err != nil {
			test.Errorf("expected nil error for alias %q, got: %v", alias, err)
		}
	}
}

// TestValidateModelAlias_Invalid checks that an unknown alias is rejected.
func TestValidateModelAlias_Invalid(test *testing.T) {
	err := security.ValidateModelAlias("unknown-model")
	if err == nil {
		test.Error("expected error for unknown model alias, got nil")
	}
}

// TestValidateAspectRatio_Empty checks that empty ratio is accepted (optional field).
func TestValidateAspectRatio_Empty(test *testing.T) {
	err := security.ValidateAspectRatio("")
	if err != nil {
		test.Errorf("expected nil error for empty ratio, got: %v", err)
	}
}

// TestValidateAspectRatio_ValidRatios checks all allowed aspect ratios.
func TestValidateAspectRatio_ValidRatios(test *testing.T) {
	validRatios := []string{"1:1", "16:9", "9:16", "4:3", "3:4"}
	for _, ratio := range validRatios {
		err := security.ValidateAspectRatio(ratio)
		if err != nil {
			test.Errorf("expected nil error for ratio %q, got: %v", ratio, err)
		}
	}
}

// TestValidateAspectRatio_Invalid checks that an unrecognized ratio is rejected.
func TestValidateAspectRatio_Invalid(test *testing.T) {
	err := security.ValidateAspectRatio("2:1")
	if err == nil {
		test.Error("expected error for invalid aspect ratio, got nil")
	}
}

// TestValidatePriority_Empty checks that empty priority is accepted (optional field).
func TestValidatePriority_Empty(test *testing.T) {
	err := security.ValidatePriority("")
	if err != nil {
		test.Errorf("expected nil error for empty priority, got: %v", err)
	}
}

// TestValidatePriority_ValidValues checks all allowed priority values.
func TestValidatePriority_ValidValues(test *testing.T) {
	validPriorities := []string{"speed", "quality", "balanced"}
	for _, priority := range validPriorities {
		err := security.ValidatePriority(priority)
		if err != nil {
			test.Errorf("expected nil error for priority %q, got: %v", priority, err)
		}
	}
}

// TestValidatePriority_Invalid checks that an unrecognized priority is rejected.
func TestValidatePriority_Invalid(test *testing.T) {
	err := security.ValidatePriority("turbo")
	if err == nil {
		test.Error("expected error for invalid priority, got nil")
	}
}

// makePNGBytes returns a minimal byte slice that starts with the PNG magic bytes.
func makePNGBytes(size int) []byte {
	data := make([]byte, size)
	copy(data, []byte{0x89, 'P', 'N', 'G'})
	return data
}

// makeJPEGBytes returns a minimal byte slice that starts with the JPEG magic bytes.
func makeJPEGBytes(size int) []byte {
	data := make([]byte, size)
	copy(data, []byte{0xFF, 0xD8, 0xFF})
	return data
}

// makeWebPBytes returns a minimal byte slice with RIFF header and WEBP marker.
func makeWebPBytes(size int) []byte {
	data := make([]byte, size)
	copy(data[0:4], []byte("RIFF"))
	copy(data[8:12], []byte("WEBP"))
	return data
}

// TestValidateAndDecodeImage_ValidPNG checks that a valid PNG base64 payload is accepted.
func TestValidateAndDecodeImage_ValidPNG(test *testing.T) {
	raw := makePNGBytes(20)
	encoded := base64.StdEncoding.EncodeToString(raw)
	decoded, err := security.ValidateAndDecodeImage(encoded, "image/png", 1024*1024)
	if err != nil {
		test.Errorf("expected nil error for valid PNG, got: %v", err)
	}
	if len(decoded) != len(raw) {
		test.Errorf("expected %d decoded bytes, got %d", len(raw), len(decoded))
	}
}

// TestValidateAndDecodeImage_ValidJPEG checks that a valid JPEG base64 payload is accepted.
func TestValidateAndDecodeImage_ValidJPEG(test *testing.T) {
	raw := makeJPEGBytes(20)
	encoded := base64.StdEncoding.EncodeToString(raw)
	_, err := security.ValidateAndDecodeImage(encoded, "image/jpeg", 1024*1024)
	if err != nil {
		test.Errorf("expected nil error for valid JPEG, got: %v", err)
	}
}

// TestValidateAndDecodeImage_ValidWebP checks that a valid WebP base64 payload is accepted.
func TestValidateAndDecodeImage_ValidWebP(test *testing.T) {
	raw := makeWebPBytes(20)
	encoded := base64.StdEncoding.EncodeToString(raw)
	_, err := security.ValidateAndDecodeImage(encoded, "image/webp", 1024*1024)
	if err != nil {
		test.Errorf("expected nil error for valid WebP, got: %v", err)
	}
}

// TestValidateAndDecodeImage_Empty checks that an empty encoded string is rejected.
func TestValidateAndDecodeImage_Empty(test *testing.T) {
	_, err := security.ValidateAndDecodeImage("", "image/png", 1024*1024)
	if err == nil {
		test.Error("expected error for empty encoded string, got nil")
	}
}

// TestValidateAndDecodeImage_InvalidBase64 checks that malformed base64 is rejected.
func TestValidateAndDecodeImage_InvalidBase64(test *testing.T) {
	_, err := security.ValidateAndDecodeImage("!!!not-base64!!!", "image/png", 1024*1024)
	if err == nil {
		test.Error("expected error for invalid base64, got nil")
	}
}

// TestValidateAndDecodeImage_Oversized checks that decoded data exceeding maxDecodedBytes is rejected.
func TestValidateAndDecodeImage_Oversized(test *testing.T) {
	raw := makePNGBytes(100)
	encoded := base64.StdEncoding.EncodeToString(raw)
	_, err := security.ValidateAndDecodeImage(encoded, "image/png", 50)
	if err == nil {
		test.Error("expected error for oversized image, got nil")
	}
}

// TestValidateAndDecodeImage_InvalidMIME checks that an unsupported MIME type is rejected.
func TestValidateAndDecodeImage_InvalidMIME(test *testing.T) {
	raw := makePNGBytes(20)
	encoded := base64.StdEncoding.EncodeToString(raw)
	_, err := security.ValidateAndDecodeImage(encoded, "image/gif", 1024*1024)
	if err == nil {
		test.Error("expected error for invalid MIME type, got nil")
	}
}

// TestValidateAndDecodeImage_TooSmall checks that decoded data under 12 bytes is rejected.
func TestValidateAndDecodeImage_TooSmall(test *testing.T) {
	raw := []byte{0x89, 'P', 'N', 'G', 0x00, 0x00}
	encoded := base64.StdEncoding.EncodeToString(raw)
	_, err := security.ValidateAndDecodeImage(encoded, "image/png", 1024*1024)
	if err == nil {
		test.Error("expected error for decoded data under 12 bytes, got nil")
	}
}

// TestValidateAndDecodeImage_MagicByteMismatch checks that magic bytes mismatching the declared MIME are rejected.
func TestValidateAndDecodeImage_MagicByteMismatch(test *testing.T) {
	// Encode PNG bytes but declare MIME as JPEG
	raw := makePNGBytes(20)
	encoded := base64.StdEncoding.EncodeToString(raw)
	_, err := security.ValidateAndDecodeImage(encoded, "image/jpeg", 1024*1024)
	if err == nil {
		test.Error("expected error for PNG bytes declared as JPEG, got nil")
	}
}

// TestValidateTaskDescription_Valid checks that a normal description passes.
func TestValidateTaskDescription_Valid(test *testing.T) {
	err := security.ValidateTaskDescription("Generate a high-quality banana image")
	if err != nil {
		test.Errorf("expected nil error, got: %v", err)
	}
}

// TestValidateTaskDescription_Empty checks that an empty description is rejected.
func TestValidateTaskDescription_Empty(test *testing.T) {
	err := security.ValidateTaskDescription("")
	if err == nil {
		test.Error("expected error for empty description, got nil")
	}
}

// TestValidateTaskDescription_TooLong checks that a description over 1000 runes is rejected.
func TestValidateTaskDescription_TooLong(test *testing.T) {
	longDesc := strings.Repeat("b", 1001)
	err := security.ValidateTaskDescription(longDesc)
	if err == nil {
		test.Error("expected error for description over 1000 runes, got nil")
	}
}
