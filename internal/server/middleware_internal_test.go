package server

import "testing"

func TestItoa_NonZero(test *testing.T) {
	cases := []struct {
		input    int
		expected string
	}{
		{1, "1"},
		{59, "59"},
		{100, "100"},
		{12345, "12345"},
	}
	for _, testCase := range cases {
		result := itoa(testCase.input)
		if result != testCase.expected {
			test.Errorf("itoa(%d) = %q, expected %q", testCase.input, result, testCase.expected)
		}
	}
}

func TestItoa_Zero(test *testing.T) {
	result := itoa(0)
	if result != "0" {
		test.Errorf("itoa(0) = %q, expected %q", result, "0")
	}
}

func TestLoadTokensFromFile_NonExistentFile(test *testing.T) {
	tokens := loadTokensFromFile("/nonexistent/path/tokens.txt")
	if len(tokens) != 0 {
		test.Errorf("expected empty tokens for non-existent file, got %d tokens", len(tokens))
	}
}

func TestLoadTokensFromFile_EmptyPath(test *testing.T) {
	tokens := loadTokensFromFile("")
	if len(tokens) != 0 {
		test.Errorf("expected empty tokens for empty path, got %d tokens", len(tokens))
	}
}
