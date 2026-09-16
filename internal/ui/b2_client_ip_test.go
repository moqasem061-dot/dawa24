package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

func TestB2_UIHandler_ClientIP(t *testing.T) {
	h := &UIHandler{}

	t.Run("single trusted proxy hop (production standard)", func(t *testing.T) {
		h.SetTrustedProxyHops(1)
		req := httptest.NewRequest(http.MethodGet, "/catalog", nil)
		req.RemoteAddr = "10.0.0.1:54321"
		// Spoofed client at leftmost, real client at rightmost before the proxy
		req.Header.Set("X-Forwarded-For", "192.0.2.1, 203.0.113.50")

		got := h.clientIP(req)
		if got != "203.0.113.50" {
			t.Errorf("clientIP = %q; want rightmost hop 203.0.113.50", got)
		}
	})

	t.Run("two trusted proxy hops (edge CDN + ingress)", func(t *testing.T) {
		h.SetTrustedProxyHops(2)
		req := httptest.NewRequest(http.MethodGet, "/catalog", nil)
		req.RemoteAddr = "10.0.0.1:54321"
		req.Header.Set("X-Forwarded-For", "198.51.100.99, 10.0.0.2")

		got := h.clientIP(req)
		if got != "198.51.100.99" {
			t.Errorf("clientIP = %q; want 198.51.100.99", got)
		}
	})

	t.Run("zero trusted proxy hops ignores headers", func(t *testing.T) {
		h.SetTrustedProxyHops(0)
		req := httptest.NewRequest(http.MethodGet, "/catalog", nil)
		req.RemoteAddr = "198.51.100.42:12345"
		req.Header.Set("X-Forwarded-For", "203.0.113.50")
		req.Header.Set("X-Real-IP", "203.0.113.50")

		got := h.clientIP(req)
		if got != "198.51.100.42" {
			t.Errorf("clientIP = %q; want RemoteAddr host 198.51.100.42", got)
		}
	})

	t.Run("fallback to X-Real-IP when X-Forwarded-For is missing", func(t *testing.T) {
		h.SetTrustedProxyHops(1)
		req := httptest.NewRequest(http.MethodGet, "/catalog", nil)
		req.RemoteAddr = "10.0.0.1:54321"
		req.Header.Set("X-Real-IP", "203.0.113.77")

		got := h.clientIP(req)
		if got != "203.0.113.77" {
			t.Errorf("clientIP = %q; want X-Real-IP 203.0.113.77", got)
		}
	})
}

func TestB2_DetectCountryAndCity(t *testing.T) {
	t.Run("loopback address defaults to Egypt / Cairo", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		country, city := detectCountryAndCity(req, "127.0.0.1")

		wantCountry := i18n.T("ar", "geo.country.eg")
		wantCity := i18n.T("ar", "geo.city.cairo")

		if country != wantCountry {
			t.Errorf("country = %q; want %q", country, wantCountry)
		}
		if city != wantCity {
			t.Errorf("city = %q; want %q", city, wantCity)
		}
	})

	t.Run("private address defaults to Egypt / Cairo", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		country, city := detectCountryAndCity(req, "10.0.1.5")

		wantCountry := i18n.T("ar", "geo.country.eg")
		wantCity := i18n.T("ar", "geo.city.cairo")

		if country != wantCountry {
			t.Errorf("country = %q; want %q", country, wantCountry)
		}
		if city != wantCity {
			t.Errorf("city = %q; want %q", city, wantCity)
		}
	})

	t.Run("public address with CF headers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("CF-IPCountry", "SA")
		req.Header.Set("CF-IPCity", "Riyadh")

		country, city := detectCountryAndCity(req, "198.51.100.1")

		wantCountry := i18n.T("ar", "geo.country.sa")
		wantCity := i18n.T("ar", "geo.city.riyadh")

		if country != wantCountry {
			t.Errorf("country = %q; want %q", country, wantCountry)
		}
		if city != wantCity {
			t.Errorf("city = %q; want %q", city, wantCity)
		}
	})

	t.Run("public address with Accept-Language header resolves correctly", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")

		// With a public IP, detectCountryAndCity must not exit at step 2 (loopback check);
		// it must reach step 3 and detect US from Accept-Language.
		country, city := detectCountryAndCity(req, "198.51.100.1")

		wantCountry := i18n.T("ar", "geo.country.us")
		wantCity := i18n.T("ar", "geo.city.newyork")

		if country != wantCountry {
			t.Errorf("country = %q; want %q", country, wantCountry)
		}
		if city != wantCity {
			t.Errorf("city = %q; want %q", city, wantCity)
		}
	})
}
