package ui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/modules/ingest"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/media"
	"github.com/moqasem061-dot/dawa24/internal/platform/storage"
	"github.com/moqasem061-dot/dawa24/internal/ui/imageurl"
)

var (
	uploadStorageClient  *storage.Client
	uploadCatalogService *catalog.Service
)

// SetUploadStorageClient sets the optional object storage client for serving and caching uploads from S3/MinIO.
func SetUploadStorageClient(sc *storage.Client) {
	uploadStorageClient = sc
}

// SetUploadCatalogService sets the catalog service for wiring cached proxy images to the database.
func SetUploadCatalogService(cs *catalog.Service) {
	uploadCatalogService = cs
}

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

// isPrivateUploadPath reports whether p (a relative upload path or storage key,
// optionally under "uploads/" or "products/") lies in a private category.
func isPrivateUploadPath(p string) bool {
	norm := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(filepath.FromSlash(p))), "/")
	for {
		trimmed := strings.TrimPrefix(strings.TrimPrefix(norm, "uploads/"), "products/")
		if trimmed == norm {
			break
		}
		norm = trimmed
	}
	first, _, _ := strings.Cut(norm, "/")
	return privateUploadCategories[strings.ToLower(first)]
}

// RegisterUploadRoutes registers the public static file server for uploaded documents & media.
func RegisterUploadRoutes(r chi.Router) {
	baseDir := GetUploadBaseDir()
	_ = os.MkdirAll(baseDir, 0755)
	for cat := range allowedUploadCategories {
		_ = os.MkdirAll(filepath.Join(baseDir, cat), 0755)
	}

	// /uploads/proxy securely fetches and caches remote images to satisfy CSP 'self'
	r.Get("/uploads/proxy", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("Accept-Ranges", "bytes")

		rawURL := r.URL.Query().Get("url")
		if rawURL == "" {
			http.NotFound(w, r)
			return
		}
		// Only URLs this server rendered are fetched; see imageurl.ProxyURL.
		if !imageurl.ValidProxySignature(rawURL, r.URL.Query().Get("sig")) {
			http.NotFound(w, r)
			return
		}

		h := sha256.Sum256([]byte(rawURL))
		hashStr := hex.EncodeToString(h[:16])
		ext := strings.ToLower(filepath.Ext(rawURL))
		if idx := strings.Index(ext, "?"); idx != -1 {
			ext = ext[:idx]
		}
		if ext == "" || len(ext) > 5 || (!strings.HasPrefix(ext, ".jp") && ext != ".png" && ext != ".webp" && ext != ".gif" && ext != ".avif") {
			ext = ".jpg"
		}
		cacheFilename := filepath.Join("products", "proxy_"+hashStr+ext)
		fullPath := filepath.Join(baseDir, cacheFilename)

		size := r.URL.Query().Get("s")

		if st, err := os.Stat(fullPath); err == nil && !st.IsDir() {
			wireProxyImage(cacheFilename, rawURL)
			rendPath := resolveRendition(baseDir, cacheFilename, size)
			serveStaticUpload(w, r, rendPath)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		data, detectedExt, err := ingest.DownloadProductImage(ctx, rawURL)
		if err != nil || len(data) == 0 {
			serveFallbackPlaceholder(w)
			return
		}
		if detectedExt != "" {
			ext = "." + detectedExt
			cacheFilename = filepath.Join("products", "proxy_"+hashStr+ext)
			fullPath = filepath.Join(baseDir, cacheFilename)
		}
		_ = os.MkdirAll(filepath.Dir(fullPath), 0755)
		_ = os.WriteFile(fullPath, data, 0644)
		_ = deriveAndStore(filepath.Dir(fullPath), filepath.Base(fullPath), data)

		wireProxyImage(cacheFilename, rawURL)

		rendPath := resolveRendition(baseDir, cacheFilename, size)
		serveStaticUpload(w, r, rendPath)
	})

	r.Get("/uploads/*", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("Accept-Ranges", "bytes")
		rctx := chi.RouteContext(r.Context())
		path := rctx.URLParam("*")
		if unescaped, err := url.PathUnescape(path); err == nil { // nolint:errswallow: an invalid escape keeps the raw path, which is cleaned and checked below
			path = unescaped
		}
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

		// ?s=thumb|card|full selects a rendition; anything else, or a
		// rendition that was never produced, serves the original.
		size := r.URL.Query().Get("s")

		// Search baseDir and known fallback directories, testing both cleanPath
		// and cleanPath with/without "products/" prefix.
		candidatePaths := []string{cleanPath}
		normCandidate := strings.TrimPrefix(filepath.ToSlash(cleanPath), "/")
		if strings.HasPrefix(normCandidate, "products/") {
			candidatePaths = append(candidatePaths, strings.TrimPrefix(normCandidate, "products/"))
		} else {
			candidatePaths = append(candidatePaths, filepath.Join("products", cleanPath))
		}

		searchDirs := []string{baseDir}
		for _, dir := range []string{"data/uploads", "cmd/server/data/uploads", "internal/ui/data/uploads"} {
			if dir != baseDir {
				searchDirs = append(searchDirs, dir)
			}
		}

		// Every path this handler may open is checked, not only the requested
		// one: "products/licenses/x.pdf" passed the first-segment check and
		// then resolved, via the products/-stripped candidate, to the private
		// licenses/x.pdf.
		for _, cp := range candidatePaths {
			if isPrivateUploadPath(cp) {
				http.Error(w, "Forbidden: private document", http.StatusForbidden)
				return
			}
		}

		for _, dir := range searchDirs {
			for _, cp := range candidatePaths {
				fullPath := resolveRendition(dir, cp, size)
				if st, err := os.Stat(fullPath); err == nil && !st.IsDir() {
					serveStaticUpload(w, r, fullPath)
					return
				}
				orig := filepath.Join(dir, cp)
				if st, err := os.Stat(orig); err == nil && !st.IsDir() {
					serveStaticUpload(w, r, orig)
					return
				}
			}
		}

		// Fallback to S3/MinIO storage client if file is missing locally
		if uploadStorageClient != nil {
			ctx := r.Context()
			targetSavePath := filepath.Join(baseDir, cleanPath)
			s3Keys := []string{strings.TrimPrefix(filepath.ToSlash(cleanPath), "/")}
			for _, cp := range candidatePaths {
				k := strings.TrimPrefix(filepath.ToSlash(cp), "/")
				if k != s3Keys[0] {
					s3Keys = append(s3Keys, k)
				}
				s3Keys = append(s3Keys, "uploads/"+k)
			}

			for _, s3Key := range s3Keys {
				if isPrivateUploadPath(s3Key) {
					continue
				}
				reader, _, err := uploadStorageClient.Get(ctx, s3Key)
				if err == nil && reader != nil {
					data, readErr := io.ReadAll(reader)
					_ = reader.Close()
					if readErr == nil && len(data) > 0 {
						_ = os.MkdirAll(filepath.Dir(targetSavePath), 0755)
						_ = os.WriteFile(targetSavePath, data, 0644)
						_ = deriveAndStore(filepath.Dir(targetSavePath), filepath.Base(targetSavePath), data)
						serveStaticUpload(w, r, targetSavePath)
						return
					}
				}
			}
		}

		http.NotFound(w, r)
	})
}

// serveStaticUpload sets exact MIME Content-Type and serves the static upload file without breaking image rendering with sandbox CSP.
func serveStaticUpload(w http.ResponseWriter, r *http.Request, fullPath string) {
	ext := strings.ToLower(filepath.Ext(fullPath))
	switch ext {
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".webp":
		w.Header().Set("Content-Type", "image/webp")
	case ".gif":
		w.Header().Set("Content-Type", "image/gif")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	case ".mp4":
		w.Header().Set("Content-Type", "video/mp4")
	case ".webm":
		w.Header().Set("Content-Type", "video/webm")
	case ".mov":
		w.Header().Set("Content-Type", "video/quicktime")
	case ".pdf":
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; object-src 'self' blob:; frame-ancestors 'self'; style-src 'unsafe-inline'; font-src 'self' data:")
	default:
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	}
	http.ServeFile(w, r, fullPath)
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

// wireProxyImage updates catalog.products with the local cached path for any product referencing rawURL.
func wireProxyImage(cacheFilename, rawURL string) {
	if uploadCatalogService == nil || cacheFilename == "" || rawURL == "" {
		return
	}
	publicPath := "/uploads/" + filepath.ToSlash(cacheFilename)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = uploadCatalogService.WireProductImageByLink(database.AsSystem(ctx), rawURL, publicPath)
	}()
}

const fallbackPlaceholderSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128" viewBox="0 0 128 128" fill="none"><rect width="128" height="128" rx="16" fill="#F8FAFC"/><rect x="1" y="1" width="126" height="126" rx="15" stroke="#E2E8F0" stroke-width="2"/><path d="M44 48a12 12 0 1 1 24 0v32a12 12 0 1 1-24 0V48z" fill="#E2E8F0"/><path d="M56 40c-4.4 0-8 3.6-8 8v16h16V48c0-4.4-3.6-8-8-8z" fill="#94A3B8"/><circle cx="84" cy="44" r="6" fill="#CBD5E1"/><path d="M72 88l12-16 12 16H72z" fill="#CBD5E1"/></svg>`

func serveFallbackPlaceholder(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write([]byte(fallbackPlaceholderSVG))
}
