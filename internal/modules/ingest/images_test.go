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
