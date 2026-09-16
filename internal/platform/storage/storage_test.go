package storage_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/storage"
)

func TestKeyFor(t *testing.T) {
	tests := []struct {
		name     string
		orgID    int64
		path     string
		expected string
	}{
		{
			name:     "standard path",
			orgID:    42,
			path:     "catalog/products.csv",
			expected: "orgs/42/catalog/products.csv",
		},
		{
			name:     "leading slash trimmed",
			orgID:    101,
			path:     "/invoices/2026/01.pdf",
			expected: "orgs/101/invoices/2026/01.pdf",
		},
		{
			name:     "whitespace trimmed",
			orgID:    7,
			path:     "  uploads/logo.png  ",
			expected: "orgs/7/uploads/logo.png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := storage.KeyFor(tt.orgID, tt.path)
			if got != tt.expected {
				t.Errorf("KeyFor(%d, %q) = %q; want %q", tt.orgID, tt.path, got, tt.expected)
			}
		})
	}
}

func TestNewValidation(t *testing.T) {
	ctx := context.Background()

	// Missing bucket should fail
	_, err := storage.New(ctx, config.Storage{})
	if err == nil {
		t.Fatal("expected error when bucket is empty, got nil")
	}

	// Valid config creates client
	cfg := config.Storage{
		Bucket:          "dawa24-test",
		Region:          "us-east-1",
		Endpoint:        "http://localhost:9000",
		AccessKeyID:     "minioadmin",
		SecretAccessKey: "minioadmin",
		UsePathStyle:    true,
		PublicBaseURL:   "https://cdn.dawa24.com",
	}

	client, err := storage.New(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error creating client: %v", err)
	}

	if client.Bucket() != "dawa24-test" {
		t.Errorf("Bucket() = %q; want %q", client.Bucket(), "dawa24-test")
	}

	// Test PublicURL
	pubURL := client.PublicURL("orgs/42/logo.png")
	if pubURL != "https://cdn.dawa24.com/orgs%2F42%2Flogo.png" && !strings.HasPrefix(pubURL, "https://cdn.dawa24.com/") {
		t.Errorf("unexpected public URL: %q", pubURL)
	}

	// Test empty key validations
	if err := client.Put(ctx, "", strings.NewReader("data"), 4, "text/plain"); err != storage.ErrInvalidKey {
		t.Errorf("expected ErrInvalidKey on Put with empty key, got %v", err)
	}

	if _, _, err := client.Get(ctx, ""); err != storage.ErrInvalidKey {
		t.Errorf("expected ErrInvalidKey on Get with empty key, got %v", err)
	}

	if err := client.Delete(ctx, ""); err != storage.ErrInvalidKey {
		t.Errorf("expected ErrInvalidKey on Delete with empty key, got %v", err)
	}

	if _, err := client.PresignGet(ctx, "", 15*time.Minute); err != storage.ErrInvalidKey {
		t.Errorf("expected ErrInvalidKey on PresignGet with empty key, got %v", err)
	}

	if _, err := client.PresignPut(ctx, "", "text/plain", 15*time.Minute); err != storage.ErrInvalidKey {
		t.Errorf("expected ErrInvalidKey on PresignPut with empty key, got %v", err)
	}
}
