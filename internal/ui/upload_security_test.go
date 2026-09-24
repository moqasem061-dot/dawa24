package ui

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/ui/imageurl"
)

func TestIsPrivateUploadPath(t *testing.T) {
	cases := map[string]bool{
		"licenses/licenses_ab.pdf":          true,
		"products/licenses/licenses_ab.pdf": true,
		"uploads/products/receipts/r.png":   true,
		"products/products/cvs/c.pdf":       true,
		"Licenses/x.pdf":                    true,
		"products/p.jpg":                    false,
		"avatars/a.png":                     false,
	}
	for p, want := range cases {
		if got := isPrivateUploadPath(p); got != want {
			t.Errorf("isPrivateUploadPath(%q) = %v; want %v", p, got, want)
		}
	}
}

// A private file must not be reachable through the products/ fallback.
func TestUploadsServeRefusesProductsPrefixedPrivateFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("UPLOAD_DIR", dir)
	t.Setenv("DATA_DIR", dir)
	base := GetUploadBaseDir()
	if err := os.MkdirAll(filepath.Join(base, "licenses"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "licenses", "licenses_secret.pdf"), []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	RegisterUploadRoutes(r)
	for _, p := range []string{"/uploads/licenses/licenses_secret.pdf", "/uploads/products/licenses/licenses_secret.pdf"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code == http.StatusOK {
			t.Errorf("GET %s served a private file", p)
		}
	}
}

// The public image proxy must refuse a URL the server did not sign.
func TestUploadsProxyRequiresSignature(t *testing.T) {
	remote := "https://attacker.example/big.jpg"
	if imageurl.ValidProxySignature(remote, "") || imageurl.ValidProxySignature(remote, "00") {
		t.Fatal("unsigned proxy URL accepted")
	}
	signed, err := url.Parse(imageurl.ProxyURL(remote))
	if err != nil {
		t.Fatal(err)
	}
	if !imageurl.ValidProxySignature(signed.Query().Get("url"), signed.Query().Get("sig")) {
		t.Fatal("server-signed proxy URL rejected")
	}
	if imageurl.ValidProxySignature("https://attacker.example/other.jpg", signed.Query().Get("sig")) {
		t.Fatal("signature reusable for a different URL")
	}

	t.Setenv("UPLOAD_DIR", t.TempDir())
	r := chi.NewRouter()
	RegisterUploadRoutes(r)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/uploads/proxy?url="+url.QueryEscape(remote), nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unsigned proxy request status = %d; want 404", rec.Code)
	}
}
