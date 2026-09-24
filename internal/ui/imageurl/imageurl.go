// Package imageurl decides which rendition of an uploaded image a given slot
// on the page should ask for.
//
// It lives apart from internal/ui so templates in internal/ui/components can
// use it without an import cycle, and so the rules are testable without a
// rendered page.
//
// The rendition names and sizes are the ones internal/platform/media produces.
// This package deliberately does NOT import that one: media is about turning
// bytes into bytes and knows nothing about URLs, and this is about URLs and
// knows nothing about pixels beyond the numbers below. Keeping the two in step
// is the job of TestSlotsMatchMediaSizes.
package imageurl

import (
	"strconv"
	"strings"
)

// Slot is where on the page an image appears. It decides which rendition is
// requested, not how the image is styled.
type Slot string

const (
	// SlotThumb is a table row, a cart line, a search result: roughly 64 CSS
	// pixels, served by the 128-pixel rendition so it stays sharp at 2x.
	SlotThumb Slot = "thumb"
	// SlotCard is a catalogue grid tile or an offer card: roughly 200 CSS
	// pixels.
	SlotCard Slot = "card"
	// SlotFull is a product detail image or a lightbox: up to 600 CSS pixels.
	SlotFull Slot = "full"
)

// slotSpec is the geometry a slot reserves and the rendition it requests.
type slotSpec struct {
	// rendition is the ?s= value, and must name one of media.Sizes.
	rendition string
	// next is the rendition one step up, offered through srcset for 2x
	// displays. Empty when there is nothing larger.
	next string
	// cssWidth is the layout width in CSS pixels, used for the width/height
	// attributes and for the sizes hint.
	cssWidth int
}

var slots = map[Slot]slotSpec{
	SlotThumb: {rendition: "thumb", next: "card", cssWidth: 64},
	SlotCard:  {rendition: "card", next: "full", cssWidth: 200},
	SlotFull:  {rendition: "full", next: "", cssWidth: 600},
}

// Image is one image on one page, described well enough to render a complete
// <img> tag.
type Image struct {
	// URL is the stored path ("/uploads/products/...") or a remote address.
	URL string
	// Alt is the accessible description. An empty Alt is legitimate for a
	// decorative image but is almost always a mistake on a product.
	Alt string
	// Class is the CSS class list for the tag.
	Class string
	// Slot decides the rendition. The zero value is SlotThumb, which is the
	// safe default: asking for too small an image degrades to a soft picture,
	// asking for too large one degrades to a slow page.
	Slot Slot
	// Eager suppresses lazy loading, for the one image above the fold that the
	// reader is actually waiting for.
	Eager bool
	// AspectH and AspectW describe the shape the layout reserves. Both default
	// to 1, a square, which is what every product slot in this platform uses.
	AspectW, AspectH int
}

// Normalize cleans and standardises an image URL stored in the database or passed from an import.
func Normalize(rawURL string) string {
	raw := strings.TrimSpace(rawURL)
	if raw == "" {
		return ""
	}

	// Strip own domain / localhost / minio prefixes
	for _, prefix := range []string{
		"https://dawa24.com/",
		"http://dawa24.com/",
		"https://api.dawa24.com/",
		"http://api.dawa24.com/",
		"http://localhost:8080/",
		"http://localhost:3000/",
		"http://localhost:9000/dawa24/",
		"http://127.0.0.1:9000/dawa24/",
		"http://minio:9000/dawa24/",
	} {
		if strings.HasPrefix(raw, prefix) {
			raw = "/" + strings.TrimPrefix(raw, prefix)
			break
		}
	}

	// Normalize slashes
	raw = strings.ReplaceAll(raw, "\\", "/")

	// Handle legacy storage/app/public paths and data/uploads paths
	if strings.HasPrefix(raw, "/data/uploads/") {
		raw = strings.TrimPrefix(raw, "/data")
	} else if strings.HasPrefix(raw, "data/uploads/") {
		raw = "/" + strings.TrimPrefix(raw, "data/")
	} else if strings.HasPrefix(raw, "/storage/uploads/") {
		raw = strings.TrimPrefix(raw, "/storage")
	} else if strings.HasPrefix(raw, "storage/uploads/") {
		raw = "/" + strings.TrimPrefix(raw, "storage/")
	} else if strings.HasPrefix(raw, "/storage/products/") {
		raw = "/uploads/products/" + strings.TrimPrefix(raw, "/storage/products/")
	} else if strings.HasPrefix(raw, "storage/products/") {
		raw = "/uploads/products/" + strings.TrimPrefix(raw, "storage/products/")
	} else if strings.HasPrefix(raw, "/storage/") {
		raw = "/uploads/" + strings.TrimPrefix(raw, "/storage/")
	} else if strings.HasPrefix(raw, "storage/") {
		raw = "/uploads/" + strings.TrimPrefix(raw, "storage/")
	} else if strings.HasPrefix(raw, "uploads/") {
		raw = "/" + raw
	} else if strings.HasPrefix(raw, "products/") {
		raw = "/uploads/" + raw
	} else if strings.HasPrefix(raw, "/products/") {
		raw = "/uploads" + raw
	}

	// If it's a bare filename like "xyz.jpg" or "6224003890152.jpg"
	if !strings.Contains(raw, "/") {
		lower := strings.ToLower(raw)
		if strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg") ||
			strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".webp") ||
			strings.HasSuffix(lower, ".gif") {
			raw = "/uploads/products/" + raw
		}
	}

	// If it is an insecure HTTP URL, upgrade to HTTPS to avoid mixed-content blocking
	if strings.HasPrefix(raw, "http://") && !strings.Contains(raw, "localhost") && !strings.Contains(raw, "127.0.0.1") {
		raw = "https://" + strings.TrimPrefix(raw, "http://")
	}

	return raw
}

// hosted reports whether this URL points at a file we stored or proxy and can resize.
func (i Image) hosted() bool {
	norm := Normalize(i.URL)
	return strings.HasPrefix(norm, "/uploads/") || strings.HasPrefix(norm, "http://") || strings.HasPrefix(norm, "https://")
}

func (i Image) spec() slotSpec {
	if s, ok := slots[i.Slot]; ok {
		return s
	}
	return slots[SlotThumb]
}

// sized appends the rendition selector to a hosted URL.
func sized(rawURL, rendition string) string {
	if rendition == "" {
		return rawURL
	}
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	return rawURL + sep + "s=" + rendition
}

// Src is the 1x source.
func (i Image) Src() string {
	norm := Normalize(i.URL)
	if strings.HasPrefix(norm, "http://") || strings.HasPrefix(norm, "https://") {
		// Route remote images through /uploads/proxy to satisfy Content-Security-Policy (img-src 'self')
		// and ensure server-side caching on local volume.
		proxyBase := ProxyURL(norm)
		return sized(proxyBase, i.spec().rendition)
	}
	if !strings.HasPrefix(norm, "/uploads/") {
		return norm
	}
	return sized(norm, i.spec().rendition)
}

// SrcSet offers the next rendition up for high-density displays.
//
// Empty for the largest slot, and an empty srcset is valid and inert — the browser simply uses src.
func (i Image) SrcSet() string {
	norm := Normalize(i.URL)
	if strings.HasPrefix(norm, "http://") || strings.HasPrefix(norm, "https://") {
		s := i.spec()
		if s.next == "" {
			return ""
		}
		proxyBase := ProxyURL(norm)
		return sized(proxyBase, s.rendition) + " 1x, " + sized(proxyBase, s.next) + " 2x"
	}
	if !strings.HasPrefix(norm, "/uploads/") {
		return ""
	}
	s := i.spec()
	if s.next == "" {
		return ""
	}
	return sized(norm, s.rendition) + " 1x, " + sized(norm, s.next) + " 2x"
}

// Sizes is the layout hint that goes with SrcSet.
func (i Image) Sizes() string {
	if !i.hosted() || i.spec().next == "" {
		return ""
	}
	return strconv.Itoa(i.spec().cssWidth) + "px"
}

func (i Image) aspect() (int, int) {
	w, h := i.AspectW, i.AspectH
	if w <= 0 {
		w = 1
	}
	if h <= 0 {
		h = 1
	}
	return w, h
}

// WidthAttr and HeightAttr reserve the image's space before it loads.
//
// They are the layout size, not the file's pixel size, and they only have to
// carry the right RATIO: the browser uses them to compute an aspect ratio and
// then lets CSS size the box. Getting the ratio right is what removes the
// layout shift; getting the absolute numbers "right" is not possible here,
// because responsive CSS will size the box differently on every device.
func (i Image) WidthAttr() string {
	return strconv.Itoa(i.spec().cssWidth)
}

// HeightAttr is WidthAttr scaled by the declared aspect ratio.
func (i Image) HeightAttr() string {
	w, h := i.aspect()
	cssH := (i.spec().cssWidth * h) / w
	return strconv.Itoa(cssH)
}

// Loading is "eager" for the one image the reader is waiting for and "lazy"
// for everything else.
func (i Image) Loading() string {
	if i.Eager {
		return "eager"
	}
	return "lazy"
}

// Thumb, Card and Full are the constructors templates use, so a call site reads
// as the slot it is rather than as a struct literal.
func Thumb(url, alt, class string) Image {
	return Image{URL: Normalize(url), Alt: alt, Class: class, Slot: SlotThumb}
}

func Card(url, alt, class string) Image {
	return Image{URL: Normalize(url), Alt: alt, Class: class, Slot: SlotCard}
}

func Full(url, alt, class string) Image {
	return Image{URL: Normalize(url), Alt: alt, Class: class, Slot: SlotFull, Eager: true}
}
