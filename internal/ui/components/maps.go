package components

import (
	"fmt"
	"net/url"
	"sync"
)

// Google Maps Embed API wiring for MapPicker.
//
// maps.google.com refuses to be framed by third-party sites (the "content
// blocked" screen), so embedded previews MUST go through the official Embed
// API, which requires an API key. The key is public by design — it is part of
// every iframe URL the browser fetches — so the operator must restrict it to
// the deployment's HTTP referrers in Google Cloud Console.

var (
	mapsMu      sync.RWMutex
	gmapsAPIKey string
)

// SetGoogleMapsAPIKey installs the Embed API key used by every MapPicker.
// Called once at boot from server configuration, before any page renders.
func SetGoogleMapsAPIKey(key string) {
	mapsMu.Lock()
	defer mapsMu.Unlock()
	gmapsAPIKey = key
}

// GoogleMapsAPIKey returns the configured key, if any.
func GoogleMapsAPIKey() string {
	mapsMu.RLock()
	defer mapsMu.RUnlock()
	return gmapsAPIKey
}

// GoogleMapsEmbedURL builds the official Embed API URL showing a pin at the
// given coordinate. Returns "" when no key is configured; callers must render
// the link-only fallback rather than a maps.google.com iframe, which Google
// blocks.
func GoogleMapsEmbedURL(lat, lon float64, zoom int) string {
	key := GoogleMapsAPIKey()
	if key == "" {
		return ""
	}
	return fmt.Sprintf(
		"https://www.google.com/maps/embed/v1/place?key=%s&q=%s&zoom=%d",
		url.QueryEscape(key),
		url.QueryEscape(fmt.Sprintf("%.8f,%.8f", lat, lon)),
		zoom,
	)
}

// GoogleMapsEmbedURLTemplate is GoogleMapsEmbedURL with {lat}, {lon} and
// {zoom} placeholders, handed to the browser so client-side coordinate changes
// can rebuild the URL without a round trip. Returns "" when no key is set.
func GoogleMapsEmbedURLTemplate() string {
	key := GoogleMapsAPIKey()
	if key == "" {
		return ""
	}
	return fmt.Sprintf(
		"https://www.google.com/maps/embed/v1/place?key=%s&q={lat},{lon}&zoom={zoom}",
		url.QueryEscape(key),
	)
}
