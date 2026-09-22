package ui

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

func TestCleanTargetImageURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard https",
			input:    "https://example.com/images/panadol.jpg",
			expected: "https://example.com/images/panadol.jpg",
		},
		{
			name:     "standard http",
			input:    "http://example.com/images/aspirin.png",
			expected: "http://example.com/images/aspirin.png",
		},
		{
			name:     "protocol relative //",
			input:    "//cdn.dawa24.eg/catalog/cataflam.webp",
			expected: "https://cdn.dawa24.eg/catalog/cataflam.webp",
		},
		{
			name:     "www prefix without scheme",
			input:    "www.supplier.com/photos/congestal.jpg",
			expected: "https://www.supplier.com/photos/congestal.jpg",
		},
		{
			name:     "surrounding quotes and whitespace",
			input:    "  \"https://example.com/images/augon.jpg\" \n",
			expected: "https://example.com/images/augon.jpg",
		},
		{
			name:     "comma-separated multiple URLs",
			input:    "https://example.com/img1.jpg, https://example.com/img2.jpg",
			expected: "https://example.com/img1.jpg",
		},
		{
			name:     "newline-separated multiple URLs",
			input:    "https://example.com/front.jpg\nhttps://example.com/back.jpg",
			expected: "https://example.com/front.jpg",
		},
		{
			name:     "empty string",
			input:    "   ",
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cleanTargetImageURL(tc.input)
			if got != tc.expected {
				t.Errorf("cleanTargetImageURL(%q) = %q; want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestGroupImageTargets_Deduplication(t *testing.T) {
	// 5 products sharing only 2 unique image URLs
	targets := []catalogImageTarget{
		{ProductID: 101, SKU: "SKU-A1", Barcode: "6221001", URL: "https://supplier.com/p1.jpg"},
		{ProductID: 102, SKU: "SKU-A2", Barcode: "6221002", URL: "https://supplier.com/p1.jpg"},
		{ProductID: 103, SKU: "SKU-A3", Barcode: "6221003", URL: "  https://supplier.com/p1.jpg  "}, // with spaces
		{ProductID: 201, SKU: "SKU-B1", Barcode: "6222001", URL: "https://supplier.com/p2.jpg"},
		{ProductID: 202, SKU: "SKU-B2", Barcode: "6222002", URL: "https://supplier.com/p2.jpg"},
	}

	groups := groupImageTargets(targets)
	if len(groups) != 2 {
		t.Fatalf("expected 2 unique image groups, got %d", len(groups))
	}

	for _, g := range groups {
		switch g.URL {
		case "https://supplier.com/p1.jpg":
			if len(g.ProductIDs) != 3 {
				t.Errorf("expected 3 product IDs for p1.jpg, got %d", len(g.ProductIDs))
			}
			if len(g.SKUs) != 3 {
				t.Errorf("expected 3 SKUs for p1.jpg, got %d", len(g.SKUs))
			}
		case "https://supplier.com/p2.jpg":
			if len(g.ProductIDs) != 2 {
				t.Errorf("expected 2 product IDs for p2.jpg, got %d", len(g.ProductIDs))
			}
			if len(g.SKUs) != 2 {
				t.Errorf("expected 2 SKUs for p2.jpg, got %d", len(g.SKUs))
			}
		default:
			t.Errorf("unexpected group URL: %s", g.URL)
		}
	}
}

func TestExtractCommittedImageTargets(t *testing.T) {
	prods := []*catalog.Product{
		{
			ID:        1,
			SKU:       "SKU-1",
			ImageLink: "https://example.com/p1.jpg",
		},
		{
			ID:        2,
			SKU:       "SKU-2",
			ImageLink: "//cdn.example.com/p2.png", // protocol relative
		},
		{
			ID:        3,
			SKU:       "SKU-3",
			ImageLink: "",
			Image:     "www.example.com/p3.jpg", // www prefix
		},
		{
			ID:        4,
			SKU:       "SKU-4",
			ImageLink: "not-a-url",
		},
		{
			ID: 0, // uncommitted
		},
	}

	targets := extractCommittedImageTargets(prods)
	if len(targets) != 3 {
		t.Fatalf("expected 3 valid targets, got %d", len(targets))
	}
	if targets[0].URL != "https://example.com/p1.jpg" {
		t.Errorf("expected p1.jpg, got %s", targets[0].URL)
	}
	if targets[1].URL != "https://cdn.example.com/p2.png" {
		t.Errorf("expected https://cdn.example.com/p2.png, got %s", targets[1].URL)
	}
	if targets[2].URL != "https://www.example.com/p3.jpg" {
		t.Errorf("expected https://www.example.com/p3.jpg, got %s", targets[2].URL)
	}
}
