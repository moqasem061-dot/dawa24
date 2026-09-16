package imageurl

import (
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/media"
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

// TestRemoteImageIsUntouched: catalog.products.image_link is a supplier's own
// URL. We do not host it, cannot derive renditions for it, and rewriting it
// would produce a 404.
func TestRemoteImageIsUntouched(t *testing.T) {
	const remote = "https://supplier.example/img/panadol.jpg"
	img := Card(remote, "Panadol", "")

	if got := img.Src(); got != remote {
		t.Errorf("Src() = %q; a remote URL must pass through unchanged", got)
	}
	if got := img.SrcSet(); got != "" {
		t.Errorf("SrcSet() = %q; there are no renditions of a remote image", got)
	}
	if got := img.Sizes(); got != "" {
		t.Errorf("Sizes() = %q; want empty without a srcset", got)
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
