package ui

import (
	"net/url"
	"strings"
)

// safeLocalRedirect validates that a target redirect destination is a safe relative
// path on the local origin, preventing open redirect vulnerabilities.
func safeLocalRedirect(target, fallback string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return fallback
	}

	// If target is an absolute URL (e.g. from a browser Referer header),
	// extract its local RequestURI (path + query) to remain on the same page.
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		u, err := url.Parse(target)
		if err != nil || u == nil {
			return fallback
		}
		target = u.RequestURI()
	}

	if !strings.HasPrefix(target, "/") {
		return fallback
	}
	if strings.HasPrefix(target, "//") || strings.HasPrefix(target, "/\\") || strings.HasPrefix(target, "/\t") {
		return fallback
	}
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "" || u.Host != "" {
		return fallback
	}
	return target
}
