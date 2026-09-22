package catalog

import "testing"

func TestCleanImageURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://example.com/img.jpg", "https://example.com/img.jpg"},
		{"http://example.com/img.png", "http://example.com/img.png"},
		{"//cdn.example.com/img.webp", "https://cdn.example.com/img.webp"},
		{"www.example.com/img.jpg", "https://www.example.com/img.jpg"},
		{"\"https://example.com/img.jpg\"", "https://example.com/img.jpg"},
		{"  https://example.com/img.jpg  \n", "https://example.com/img.jpg"},
		{"https://example.com/1.jpg, https://example.com/2.jpg", "https://example.com/1.jpg"},
		{"", ""},
	}

	for _, tc := range tests {
		got := cleanImageURL(tc.input)
		if got != tc.expected {
			t.Errorf("cleanImageURL(%q) = %q; want %q", tc.input, got, tc.expected)
		}
	}
}
