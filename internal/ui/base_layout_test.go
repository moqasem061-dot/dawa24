package ui_test

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/ui"
	"github.com/muhiya/dawa24-store/internal/ui/layouts"
)

// renderBase renders the document shell on its own.
func renderBase(t *testing.T) string {
	t.Helper()
	var sb strings.Builder
	body := layouts.Base("عنوان", "ar", "rtl")
	if err := body.Render(context.Background(), &sb); err != nil {
		t.Fatalf("render base layout: %v", err)
	}
	return sb.String()
}

// TestBaseLayoutLoadsNoThirdPartyScripts guards the change that moved htmx,
// Alpine and Leaflet into this application.
//
// They were served from unpkg and cdnjs on every request, two of the three
// without an integrity attribute, which made every page render depend on hosts
// this project does not control. Nothing but this test stops the next person
// pasting a CDN tag back into the layout because it is quicker.
func TestBaseLayoutLoadsNoThirdPartyScripts(t *testing.T) {
	html := renderBase(t)

	forbidden := []string{
		"unpkg.com",
		"cdnjs.cloudflare.com",
		"cdn.jsdelivr.net",
	}
	for _, host := range forbidden {
		if strings.Contains(html, host) {
			t.Errorf("base layout references %s; vendor assets are served from /static/vendor", host)
		}
	}
}

// TestBaseLayoutDoesNotLoadLeaflet records that the mapping library is no
// longer on the critical path.
//
// Leaflet's CSS, JavaScript and marker images are roughly 200 KB and were
// requested by every page in the platform for the two screens that draw a map.
// app.js loads it when a map element is present.
func TestBaseLayoutDoesNotLoadLeaflet(t *testing.T) {
	if html := renderBase(t); strings.Contains(html, "leaflet") {
		t.Error("base layout loads leaflet; it is loaded on demand by app.js (ensureLeaflet)")
	}
}

var assetHref = regexp.MustCompile(`(?:href|src)="(/static/[^"]+)"`)

// TestBaseLayoutAssetsCarryContentHashes guards the replacement of the
// hand-typed cache-busting version strings.
//
// Those had already drifted — the stylesheets said 2026082903 while app.js said
// 2026082901 — so a stylesheet change could ship to browsers without the script
// change that belonged with it. Every local asset URL must now carry the hash
// of the file it points at.
func TestBaseLayoutAssetsCarryContentHashes(t *testing.T) {
	html := renderBase(t)

	matches := assetHref.FindAllStringSubmatch(html, -1)
	if len(matches) == 0 {
		t.Fatal("base layout requested no local assets; the layout is probably broken")
	}

	for _, m := range matches {
		url := m[1]
		path, query, found := strings.Cut(url, "?")
		if !found || !strings.HasPrefix(query, "v=") {
			t.Errorf("%s has no content hash; use layouts.Asset()", url)
			continue
		}
		if want := ui.AssetURL(path); want != url {
			t.Errorf("%s does not match its file's hash (want %s)", url, want)
		}
	}
}

// TestBaseLayoutAlwaysRenders records that the shell has no conditional escape.
//
// The template used to wrap its whole body in a nil check on site settings.
// GetSiteSettings never returns nil — it falls back to a complete default — so
// the branch could not fire, but a reader could not know that without going to
// look, and a future change to GetSiteSettings would have turned every page in
// the platform into a silent, empty HTTP 200.
func TestBaseLayoutAlwaysRenders(t *testing.T) {
	html := renderBase(t)

	for _, want := range []string{"<!doctype html>", "<html", "</html>"} {
		if !strings.Contains(strings.ToLower(html), want) {
			t.Errorf("base layout output is missing %q", want)
		}
	}
}
