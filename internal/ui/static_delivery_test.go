package ui_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/moqasem061-dot/dawa24/internal/platform/httpx"
	"github.com/moqasem061-dot/dawa24/internal/ui"
	"github.com/moqasem061-dot/dawa24/internal/ui/layouts"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

func staticResponse(router http.Handler, url, encoding string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	req.Header.Set("Accept-Encoding", encoding)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestStaticDeliveryMinifiesBeforeHashingAndCompression(t *testing.T) {
	router := chi.NewRouter()
	ui.RegisterStaticRoutes(router, func(*http.Request) string { return "https://dawa24.com" })
	for _, file := range []string{"css/components.css", "css/public.css", "js/security.js", "js/app.js", "js/maps.js"} {
		t.Run(file, func(t *testing.T) {
			source, err := os.ReadFile("static/" + file)
			if err != nil {
				t.Fatal(err)
			}
			url := ui.AssetURL("/static/" + file)
			plain := staticResponse(router, url, "identity")
			if plain.Code != http.StatusOK {
				t.Fatalf("status: %d", plain.Code)
			}
			if plain.Body.Len()*100 >= len(source)*85 {
				t.Fatalf("minification saved less than 15%%: %d -> %d", len(source), plain.Body.Len())
			}
			digest := sha256.Sum256(plain.Body.Bytes())
			if !strings.HasSuffix(url, fmt.Sprintf("?v=%x", digest[:4])) {
				t.Fatal("URL must hash the delivered bytes")
			}
			if got := plain.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
				t.Fatalf("cache: %s", got)
			}
			compressed := staticResponse(router, url, "gzip")
			if compressed.Header().Get("Content-Encoding") != "gzip" {
				t.Fatal("missing gzip")
			}
			reader, err := gzip.NewReader(compressed.Body)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := io.ReadAll(reader)
			if err != nil {
				t.Fatal(err)
			}
			if err := reader.Close(); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(decoded, plain.Body.Bytes()) {
				t.Fatal("gzip changed the minified content")
			}
			request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
			request.Header.Set("If-None-Match", plain.Header().Get("ETag"))
			cached := httptest.NewRecorder()
			router.ServeHTTP(cached, request)
			if cached.Code != http.StatusNotModified || cached.Body.Len() != 0 {
				t.Fatal("conditional GET must return an empty 304")
			}
			stale := staticResponse(router, "/static/"+file+"?v=old", "identity")
			if strings.Contains(stale.Header().Get("Cache-Control"), "immutable") {
				t.Fatal("stale version must not be immutable")
			}
		})
	}
}

func TestHomepageDeliveryPreservesSecurityAndDefersFullStyles(t *testing.T) {
	ctx := layouts.WithPath(httpx.WithNonce(context.Background(), "delivery-test"), "/")
	var markup strings.Builder
	if err := pages.CustomerHome(nil, nil, nil, pages.HomeStats{}, "ar", "rtl").Render(ctx, &markup); err != nil {
		t.Fatal(err)
	}
	html := markup.String()
	for _, wanted := range []string{
		`data-security-runtime nonce="delivery-test">`,
		`<style id="home-critical" data-clarity-unmask="true" nonce="delivery-test">`,
		`data-deferred-css="true"`, `<noscript>`, `width="480" height="164"`,
		`doctor-capsule-120.webp?v=`, `logo-240.avif?v=`,
	} {
		if !strings.Contains(html, wanted) {
			t.Errorf("missing %s", wanted)
		}
	}
	for _, unwanted := range []string{`src="/static/js/security.js`, "fonts.googleapis.com", "fonts.gstatic.com", `src="/static/img/logo.png"`, `src="/static/img/doctor-capsule.png"`} {
		if strings.Contains(html, unwanted) {
			t.Errorf("unexpected %s", unwanted)
		}
	}
	if strings.Count(html, `media="print" data-deferred-css="true"`) != 2 {
		t.Fatal("homepage must defer both complete stylesheets")
	}
	if strings.Contains(html, `</script></script>`) {
		t.Fatal("inline runtime closed its own script element")
	}
}

func TestUploadedLogoIsNotReplaced(t *testing.T) {
	const custom = "/uploads/brands/my-logo.png"
	if layouts.BrandLogoURL(custom) != custom || layouts.BrandLogoSrcset(custom) != "" {
		t.Fatal("custom branding must remain unchanged")
	}
}
