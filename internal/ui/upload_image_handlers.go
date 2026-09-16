package ui

import (
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/platform/media"
)

func init() {
	_ = mime.AddExtensionType(".mp4", "video/mp4")
	_ = mime.AddExtensionType(".webm", "video/webm")
	_ = mime.AddExtensionType(".mov", "video/quicktime")
	_ = mime.AddExtensionType(".webp", "image/webp")
}

var privateUploadCategories = map[string]bool{
	"licenses":        true,
	"documents":       true,
	"receipts":        true,
	"cvs":             true,
	"resumes":         true,
	"compare":         true,
	"imports":         true,
	"temp_warehouses": true,
}

// RegisterUploadRoutes registers the public static file server for uploaded documents & media.
func RegisterUploadRoutes(r chi.Router) {
	baseDir := GetUploadBaseDir()
	_ = os.MkdirAll(baseDir, 0755)
	for cat := range allowedUploadCategories {
		_ = os.MkdirAll(filepath.Join(baseDir, cat), 0755)
	}

	r.Get("/uploads/*", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
		// Immutable, not 24 hours.
		//
		// Every filename this application writes is "<category>_<16 random hex
		// characters><ext>" — see saveUploadedFile. A stored file's content
		// therefore never changes: replacing an image mints a new name and the
		// old URL stops being referenced. Telling browsers it might change in a
		// day made every returning visitor revalidate every product thumbnail
		// on the page, daily, to be told nothing had changed.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("Accept-Ranges", "bytes")
		rctx := chi.RouteContext(r.Context())
		path := rctx.URLParam("*")
		cleanPath := filepath.Clean(filepath.FromSlash(path))
		if strings.Contains(cleanPath, "..") {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		// Private categories (KYC licenses, institutional documents, receipts, CVs, spreadsheets)
		// must not be served publicly. They are accessed via authenticated endpoints only.
		normalizedSlash := strings.TrimPrefix(filepath.ToSlash(cleanPath), "/")
		parts := strings.Split(normalizedSlash, "/")
		if len(parts) > 0 && privateUploadCategories[parts[0]] {
			http.Error(w, "Forbidden: private document", http.StatusForbidden)
			return
		}

		ext := strings.ToLower(filepath.Ext(cleanPath))
		switch ext {
		case ".mp4":
			w.Header().Set("Content-Type", "video/mp4")
		case ".webm":
			w.Header().Set("Content-Type", "video/webm")
		case ".mov":
			w.Header().Set("Content-Type", "video/quicktime")
		}

		// ?s=thumb|card|full selects a rendition; anything else, or a
		// rendition that was never produced, serves the original.
		size := r.URL.Query().Get("s")
		fullPath := resolveRendition(baseDir, cleanPath, size)
		if _, err := os.Stat(fullPath); err == nil {
			// The rendition may be a different type from the original — a WebP
			// upload derives to JPEG — so the Content-Type set from the request
			// path above would be wrong. Clear it and let ServeFile sniff.
			if fullPath != filepath.Join(baseDir, cleanPath) {
				w.Header().Del("Content-Type")
			}
			http.ServeFile(w, r, fullPath)
			return
		}
		// Resilient fallback check in default relative directory
		if baseDir != "data/uploads" {
			fallbackPath := filepath.Join("data/uploads", cleanPath)
			if _, err := os.Stat(fallbackPath); err == nil {
				http.ServeFile(w, r, fallbackPath)
				return
			}
		}
		http.NotFound(w, r)
	})
}

// deriveAndStore writes the scaled renditions of an uploaded image next to the
// original, named "<base>_<size><ext>".
//
// It is best-effort by design. A PDF licence, a spreadsheet or an image in a
// format we cannot decode simply has no renditions, and that must not fail the
// upload — the original is still stored and still served, which is exactly what
// happened for every upload before this existed. Failures are returned so the
// caller can log them; no caller should treat one as fatal.
//
// Note what is NOT here: no image is ever re-encoded in place, and the original
// is never replaced. Deriving is additive, so an upload that predates this code
// keeps working and a bad derivation can be deleted and regenerated.
func deriveAndStore(destDir, uniqueName string, data []byte) error {
	derivatives, err := media.Derive(data)
	if err != nil {
		// Not an image, or one we cannot read. Not an error for the upload.
		return nil
	}
	base := strings.TrimSuffix(uniqueName, filepath.Ext(uniqueName))
	for _, d := range derivatives {
		path := filepath.Join(destDir, base+"_"+d.Size.Name+d.Ext)
		if err := os.WriteFile(path, d.Bytes, 0o644); err != nil {
			return fmt.Errorf("write %s rendition: %w", d.Size.Name, err)
		}
	}
	return nil
}

// resolveRendition maps a requested size onto a file on disk.
//
// It returns the original's path when the rendition does not exist, so a
// missing derivative degrades to a correct-but-larger image rather than to a
// broken one.
func resolveRendition(baseDir, cleanPath, size string) string {
	full := filepath.Join(baseDir, cleanPath)
	if size == "" {
		return full
	}
	known := false
	for _, s := range media.Sizes {
		if s.Name == size {
			known = true
			break
		}
	}
	if !known {
		return full
	}
	ext := filepath.Ext(cleanPath)
	base := strings.TrimSuffix(full, ext)
	// A photograph derives to .jpg and anything with transparency to .png.
	// Trying both is cheaper than recording which, and self-correcting if the
	// rule ever changes.
	for _, candidate := range []string{base + "_" + size + ".jpg", base + "_" + size + ".png"} {
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate
		}
	}
	return full
}
