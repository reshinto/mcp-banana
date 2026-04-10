// Package security provides input validation functions for the mcp-banana MCP server.
// All user-supplied input must pass through these validators before reaching the Gemini client.
package security

import (
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/reshinto/mcp-banana/internal/gemini"
)

const (
	maxPromptRunes   = 10000
	maxTaskDescRunes = 1000
	minImageBytes    = 12
	pngMagic         = "\x89PNG"
	jpegMagicByte0   = byte(0xFF)
	jpegMagicByte1   = byte(0xD8)
	jpegMagicByte2   = byte(0xFF)
	webpRIFF         = "RIFF"
	webpWEBP         = "WEBP"
)

// validAspectRatios holds the set of accepted aspect ratio strings.
var validAspectRatios = map[string]struct{}{
	"1:1":  {},
	"16:9": {},
	"9:16": {},
	"4:3":  {},
	"3:4":  {},
}

// validPriorities holds the set of accepted priority strings.
var validPriorities = map[string]struct{}{
	"speed":    {},
	"quality":  {},
	"balanced": {},
}

// validMIMETypes holds the set of accepted image MIME type strings.
var validMIMETypes = map[string]struct{}{
	"image/png":  {},
	"image/jpeg": {},
	"image/webp": {},
}

// ValidatePrompt validates a text prompt before sending it to the Gemini client.
// It returns an error if the prompt is empty, exceeds 10000 runes, or contains null bytes.
func ValidatePrompt(prompt string) error {
	if prompt == "" {
		return fmt.Errorf("prompt must not be empty")
	}
	if utf8.RuneCountInString(prompt) > maxPromptRunes {
		return fmt.Errorf("prompt exceeds maximum length of %d runes", maxPromptRunes)
	}
	if strings.ContainsRune(prompt, '\x00') {
		return fmt.Errorf("prompt must not contain null bytes")
	}
	return nil
}

// ValidateModelAlias validates a model alias against the registered model registry.
// An empty alias is accepted and means "use the default model".
// Returns an error if the alias is non-empty and not in the known alias list.
func ValidateModelAlias(alias string) error {
	if alias == "" {
		return nil
	}
	for _, validAlias := range gemini.ValidAliases() {
		if alias == validAlias {
			return nil
		}
	}
	return fmt.Errorf("unknown model alias: %q", alias)
}

// ValidateAspectRatio validates an aspect ratio string.
// An empty value is accepted (the field is optional).
// Returns an error if the value is non-empty and not one of the accepted ratios.
func ValidateAspectRatio(ratio string) error {
	if ratio == "" {
		return nil
	}
	if _, ok := validAspectRatios[ratio]; !ok {
		return fmt.Errorf("invalid aspect ratio %q: must be one of 1:1, 16:9, 9:16, 4:3, 3:4", ratio)
	}
	return nil
}

// ValidatePriority validates a priority string.
// An empty value is accepted (the field is optional).
// Returns an error if the value is non-empty and not one of the accepted priorities.
func ValidatePriority(priority string) error {
	if priority == "" {
		return nil
	}
	if _, ok := validPriorities[priority]; !ok {
		return fmt.Errorf("invalid priority %q: must be one of speed, quality, balanced", priority)
	}
	return nil
}

// ValidateAndDecodeImage validates a base64-encoded image and returns the decoded bytes.
// It checks that the encoded string is non-empty, the base64 is valid, the decoded size
// is within maxDecodedBytes, the MIME type is supported, the decoded data is at least 12 bytes,
// and the magic bytes match the declared MIME type.
// Returns the decoded bytes on success to avoid a double-decode in callers.
func ValidateAndDecodeImage(encoded string, mimeType string, maxDecodedBytes int) ([]byte, error) {
	if encoded == "" {
		return nil, fmt.Errorf("image data must not be empty")
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("image data is not valid base64: %w", err)
	}

	if len(decoded) > maxDecodedBytes {
		return nil, fmt.Errorf("decoded image size %d bytes exceeds maximum of %d bytes", len(decoded), maxDecodedBytes)
	}

	if _, ok := validMIMETypes[mimeType]; !ok {
		return nil, fmt.Errorf("unsupported image MIME type %q: must be one of image/png, image/jpeg, image/webp", mimeType)
	}

	if len(decoded) < minImageBytes {
		return nil, fmt.Errorf("decoded image is too small (%d bytes): minimum is %d bytes for magic byte validation", len(decoded), minImageBytes)
	}

	if err := checkMagicBytes(decoded, mimeType); err != nil {
		return nil, err
	}

	return decoded, nil
}

// checkMagicBytes verifies that the decoded image bytes match the magic bytes
// expected for the declared MIME type.
func checkMagicBytes(decoded []byte, mimeType string) error {
	switch mimeType {
	case "image/png":
		if string(decoded[:4]) != pngMagic {
			return fmt.Errorf("image magic bytes do not match declared MIME type %q", mimeType)
		}
	case "image/jpeg":
		if decoded[0] != jpegMagicByte0 || decoded[1] != jpegMagicByte1 || decoded[2] != jpegMagicByte2 {
			return fmt.Errorf("image magic bytes do not match declared MIME type %q", mimeType)
		}
	case "image/webp":
		if string(decoded[0:4]) != webpRIFF || string(decoded[8:12]) != webpWEBP {
			return fmt.Errorf("image magic bytes do not match declared MIME type %q", mimeType)
		}
	default:
		return fmt.Errorf("unsupported MIME type %q for magic byte check", mimeType)
	}
	return nil
}

// ValidateTaskDescription validates a task description used for the recommend_model tool.
// It returns an error if the description is empty or exceeds 1000 runes.
func ValidateTaskDescription(description string) error {
	if description == "" {
		return fmt.Errorf("task description must not be empty")
	}
	if utf8.RuneCountInString(description) > maxTaskDescRunes {
		return fmt.Errorf("task description exceeds maximum length of %d runes", maxTaskDescRunes)
	}
	return nil
}
