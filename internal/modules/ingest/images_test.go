package ingest_test

import (
	"context"
	"net"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
)

func TestSSRFProtectedPrivateIPs(t *testing.T) {
	privateIPs := []string{
		"127.0.0.1",
		"10.0.0.1",
		"10.255.255.255",
		"172.16.0.1",
		"172.31.255.255",
		"192.168.1.1",
		"169.254.169.254",
		"::1",
		"fc00::1",
		"fe80::1",
	}

	for _, ipStr := range privateIPs {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			t.Fatalf("failed parsing test IP: %s", ipStr)
		}
		if !ingest.IsPrivateIP(ip) {
			t.Errorf("expected %s to be recognized as private IP", ipStr)
		}
	}

	publicIPs := []string{
		"8.8.8.8",
		"1.1.1.1",
		"142.250.180.206",
		"2607:f8b0:4005:805::200e",
	}

	for _, ipStr := range publicIPs {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			t.Fatalf("failed parsing test IP: %s", ipStr)
		}
		if ingest.IsPrivateIP(ip) {
			t.Errorf("expected %s to be recognized as public IP", ipStr)
		}
	}
}

func TestDownloadProductImageInvalidURLs(t *testing.T) {
	ctx := context.Background()

	invalidURLs := []string{
		"file:///etc/passwd",
		"ftp://example.com/image.png",
		"gopher://example.com",
		"javascript:alert(1)",
		"http://",
		"",
	}

	for _, url := range invalidURLs {
		_, _, err := ingest.DownloadProductImage(ctx, url)
		if err == nil {
			t.Errorf("expected error for invalid URL %q, got nil", url)
		}
	}
}

func TestDetectImageExtension(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		contentType string
		urlPath     string
		expected    string
	}{
		{
			name:     "JPEG magic bytes",
			data:     []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10},
			expected: "jpg",
		},
		{
			name:     "PNG magic bytes",
			data:     []byte("\x89PNG\r\n\x1a\n\x00\x00"),
			expected: "png",
		},
		{
			name:     "GIF89a magic bytes",
			data:     []byte("GIF89a\x01\x00"),
			expected: "gif",
		},
		{
			name:     "WEBP magic bytes",
			data:     []byte("RIFF\x00\x00\x00\x00WEBPVP8 "),
			expected: "webp",
		},
		{
			name:     "AVIF magic bytes",
			data:     []byte("\x00\x00\x00\x1cftypavif\x00\x00"),
			expected: "avif",
		},
		{
			name:        "Octet-stream with URL extension fallback",
			data:        []byte{0x01, 0x02, 0x03},
			contentType: "application/octet-stream",
			urlPath:     "/images/product.png?version=2",
			expected:    "png",
		},
		{
			name:        "Content-Type header fallback",
			data:        []byte{0x01, 0x02, 0x03},
			contentType: "image/webp",
			urlPath:     "/unnamed",
			expected:    "webp",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ingest.DetectImageExtension(tc.data, tc.contentType, tc.urlPath)
			if got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}
