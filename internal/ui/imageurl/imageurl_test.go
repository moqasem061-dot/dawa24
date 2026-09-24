package imageurl

import (
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/platform/media"
)

// TestSlotsMatchMediaSizes is the seam between this package and the one that
// produces the files.
//
// A slot asking for "?s=card" when media.Sizes no longer contains "card" does
// not fail: internal/ui.resolveRendition falls back to the original, so every
// page silently goes back to serving four-thousand-pixel photographs into
// thumbnails and nothing anywhere reports it. That is precisely the failure
// this whole change exists to remove, so it is asserted rather than assumed.
func TestSlotsMatchMediaSizes(t *testing.T) {
	known := map[string]bool{}
	for _, s := range media.Sizes {
		known[s.Name] = true
	}
	for slot, spec := range slots {
		if !known[spec.rendition] {
			t.Errorf("slot %q requests rendition %q, which media.Sizes does not produce",
				slot, spec.rendition)
		}
		if spec.next != "" && !known[spec.next] {
			t.Errorf("slot %q offers 2x rendition %q, which media.Sizes does not produce",
				slot, spec.next)
		}
	}
}

func TestHostedImageRequestsARendition(t *testing.T) {
	img := Card("/uploads/products/products_abc123.jpg", "Panadol", "card-img")

	if got, want := img.Src(), "/uploads/products/products_abc123.jpg?s=card"; got != want {
		t.Errorf("Src() = %q, want %q", got, want)
	}
	set := img.SrcSet()
	if !strings.Contains(set, "s=card 1x") || !strings.Contains(set, "s=full 2x") {
		t.Errorf("SrcSet() = %q; want a 1x card and a 2x full", set)
	}
	if img.Loading() != "lazy" {
		t.Errorf("Loading() = %q, want lazy", img.Loading())
	}
	if img.WidthAttr() == "" || img.HeightAttr() == "" {
		t.Error("width and height must always be set; an <img> without them reflows the page")
	}
}

// TestRemoteImageRoutesThroughProxy asserts that remote URLs route through /uploads/proxy
// to satisfy Content-Security-Policy (img-src 'self') and cache to volume.
func TestRemoteImageRoutesThroughProxy(t *testing.T) {
	const remote = "https://supplier.example/img/panadol.jpg"
	img := Card(remote, "Panadol", "")

	if got := img.Src(); !strings.HasPrefix(got, "/uploads/proxy?url=") || !strings.Contains(got, "s=card") {
		t.Errorf("Src() = %q; remote URL must route through /uploads/proxy with rendition", got)
	}
	if got := img.SrcSet(); !strings.Contains(got, "/uploads/proxy?url=") || !strings.Contains(got, "1x") {
		t.Errorf("SrcSet() = %q; want proxy renditions for remote image", got)
	}
	if got := img.Sizes(); got != "200px" {
		t.Errorf("Sizes() = %q; want 200px for card", got)
	}
}

func TestEmptyURLStaysEmpty(t *testing.T) {
	if got := Thumb("", "", "").Src(); got != "" {
		t.Errorf("Src() = %q, want empty", got)
	}
}

// TestFullSlotIsEager: the product-detail hero is the image the reader is
// waiting for. Lazy-loading it delays the only picture that matters.
func TestFullSlotIsEager(t *testing.T) {
	if got := Full("/uploads/products/p.jpg", "", "").Loading(); got != "eager" {
		t.Errorf("Loading() = %q, want eager for the hero image", got)
	}
	if got := Full("/uploads/products/p.jpg", "", "").SrcSet(); got != "" {
		t.Errorf("SrcSet() = %q; the largest slot has nothing above it", got)
	}
}

// TestSrcSetAppendsToAnExistingQuery guards the string-building, which is the
// kind of thing that works until one URL already has a parameter.
func TestSrcSetAppendsToAnExistingQuery(t *testing.T) {
	img := Thumb("/uploads/products/p.jpg?v=2", "", "")
	if got, want := img.Src(), "/uploads/products/p.jpg?v=2&s=thumb"; got != want {
		t.Errorf("Src() = %q, want %q", got, want)
	}
}

func TestAspectRatioDrivesHeight(t *testing.T) {
	img := Card("/uploads/products/p.jpg", "", "")
	img.AspectW, img.AspectH = 4, 3

	if got, want := img.WidthAttr(), "200"; got != want {
		t.Errorf("WidthAttr() = %q, want %q", got, want)
	}
	if got, want := img.HeightAttr(), "150"; got != want {
		t.Errorf("HeightAttr() = %q, want %q (4:3 of 200)", got, want)
	}
}

// TestZeroValueSlotIsUsable: an Image built as a bare literal must still render
// a valid tag rather than an empty src or a division by zero.
func TestZeroValueSlotIsUsable(t *testing.T) {
	img := Image{URL: "/uploads/products/p.jpg"}

	if !strings.Contains(img.Src(), "s=thumb") {
		t.Errorf("Src() = %q; the zero Slot should default to thumb", img.Src())
	}
	if img.HeightAttr() == "0" {
		t.Error("HeightAttr() = 0; a zero aspect ratio must default to square")
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"   ", ""},
		{"/uploads/products/123.jpg", "/uploads/products/123.jpg"},
		{"uploads/products/123.jpg", "/uploads/products/123.jpg"},
		{"products/123.jpg", "/uploads/products/123.jpg"},
		{"/products/123.jpg", "/uploads/products/123.jpg"},
		{"storage/products/123.jpg", "/uploads/products/123.jpg"},
		{"/storage/products/123.jpg", "/uploads/products/123.jpg"},
		{"storage/uploads/123.jpg", "/uploads/123.jpg"},
		{"/storage/uploads/123.jpg", "/uploads/123.jpg"},
		{"data/uploads/products/123.jpg", "/uploads/products/123.jpg"},
		{"/data/uploads/products/123.jpg", "/uploads/products/123.jpg"},
		{"https://dawa24.com/uploads/products/123.jpg", "/uploads/products/123.jpg"},
		{"http://localhost:9000/dawa24/uploads/products/123.jpg", "/uploads/products/123.jpg"},
		{"6224003890152.jpg", "/uploads/products/6224003890152.jpg"},
		{"http://example.com/img.png", "https://example.com/img.png"},
		{"https://example.com/img.png", "https://example.com/img.png"},
	}

	for _, tc := range tests {
		got := Normalize(tc.input)
		if got != tc.want {
			t.Errorf("Normalize(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
