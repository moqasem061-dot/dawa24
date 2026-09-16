package ui

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/ui/layouts"
)

// DynamicRobotsTxtFetcher optionally loads live robots.txt directives from the database.
var DynamicRobotsTxtFetcher func(ctx context.Context) (string, error)

//go:embed static/*
var staticFS embed.FS

type staticAsset struct {
	content     []byte
	etag        string
	contentType string
	// gzipped is the same bytes compressed once, at start-up, or nil when
	// compressing them was not worth it.
	//
	// The router's generic Compress middleware re-ran gzip on every response it
	// had not already sent: 385 KB of stylesheets and 188 KB of script, level 5,
	// recompressed for every cold visitor to produce a byte-for-byte identical
	// result each time. These files change only when the binary is rebuilt, so
	// the work belongs at start-up, where it happens once for the life of the
	// process.
	gzipped []byte
}

// compressibleAsset reports whether gzip is worth storing for this type.
//
// Text compresses to roughly a fifth; PNG, WOFF2 and JPEG are already
// compressed and gzip makes them marginally larger while costing CPU.
func compressibleAsset(contentType string) bool {
	switch {
	case strings.HasPrefix(contentType, "text/"),
		strings.HasPrefix(contentType, "application/javascript"),
		strings.HasPrefix(contentType, "application/json"),
		strings.HasPrefix(contentType, "image/svg+xml"):
		return true
	}
	return false
}

// acceptsGzip reports whether the caller advertised gzip.
//
// A substring test is enough here and a full Accept-Encoding parse is not
// warranted: the only risk it carries is "gzip;q=0", which no browser sends,
// and the fallback for a wrong answer is an uncompressed response.
func acceptsGzip(req *http.Request) bool {
	return strings.Contains(req.Header.Get("Accept-Encoding"), "gzip")
}

var (
	assetCache = make(map[string]*staticAsset)
	assetOnce  sync.Once
)

func initStaticAssetCache() {
	_ = fs.WalkDir(staticFS, "static", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, err := staticFS.ReadFile(path)
		if err != nil {
			return nil
		}
		hash := sha256.Sum256(data)
		etag := "\"" + hex.EncodeToString(hash[:16]) + "\""

		ext := filepath.Ext(path)
		cType := mime.TypeByExtension(ext)
		if cType == "" {
			switch ext {
			case ".css":
				cType = "text/css; charset=utf-8"
			case ".js":
				cType = "application/javascript; charset=utf-8"
			case ".svg":
				cType = "image/svg+xml"
			case ".png":
				cType = "image/png"
			case ".jpg", ".jpeg":
				cType = "image/jpeg"
			case ".webp":
				cType = "image/webp"
			case ".woff2":
				cType = "font/woff2"
			case ".txt":
				cType = "text/plain; charset=utf-8"
			default:
				cType = "application/octet-stream"
			}
		}

		relPath := strings.TrimPrefix(path, "static")
		if !strings.HasPrefix(relPath, "/") {
			relPath = "/" + relPath
		}

		a := &staticAsset{
			content:     data,
			etag:        etag,
			contentType: cType,
		}
		// Compress once, now. Level 9 rather than the middleware's 5: this runs
		// a single time per process, so the slower setting costs nothing per
		// request and buys a few per cent on every one of them.
		if compressibleAsset(cType) {
			var buf bytes.Buffer
			zw, zErr := gzip.NewWriterLevel(&buf, gzip.BestCompression)
			if zErr == nil {
				if _, wErr := zw.Write(data); wErr == nil && zw.Close() == nil {
					// Keep it only if it actually helped. A tiny file can come
					// out larger than it went in once the gzip header is added.
					if buf.Len() < len(data) {
						a.gzipped = buf.Bytes()
					}
				}
			}
		}

		assetCache[relPath] = a
		return nil
	})
}

// AssetURL returns the public URL for an embedded asset with its content hash
// appended as a cache-busting query parameter.
//
// It replaces a version string that was typed into the layout by hand and had
// already drifted: the stylesheets carried ?v=2026082903 while app.js carried
// ?v=2026082901, so a CSS change shipped without the JS change that went with
// it, and both relied on someone remembering to edit the layout at all. The
// hash is computed from the file, so it cannot be forgotten and cannot be
// wrong.
//
// path is the URL path, e.g. "/static/css/app.css". An unknown path is returned
// unchanged rather than failing the render — a missing stylesheet is a visible
// bug, and a blank page is not a better way to report it.
func AssetURL(path string) string {
	assetOnce.Do(initStaticAssetCache)

	key := strings.TrimPrefix(path, "/static")
	asset, ok := assetCache[key]
	if !ok {
		return path
	}
	return path + "?v=" + assetVersion(asset.etag)
}

// assetVersion is the short digest AssetURL puts in the ?v= parameter.
func assetVersion(etag string) string {
	digest := strings.Trim(etag, `"`)
	if len(digest) > 8 {
		digest = digest[:8]
	}
	return digest
}

// matchesAssetVersion reports whether the request named the build we hold.
func matchesAssetVersion(req *http.Request, asset *staticAsset) bool {
	v := req.URL.Query().Get("v")
	return v != "" && v == assetVersion(asset.etag)
}

// RegisterStaticRoutes mounts embedded static assets with in-RAM caching, strong ETags, and 304 Not Modified support.
func RegisterStaticRoutes(r chi.Router) {
	assetOnce.Do(initStaticAssetCache)

	r.Get("/static/*", func(w http.ResponseWriter, req *http.Request) {
		urlPath := "/" + strings.TrimPrefix(chi.URLParam(req, "*"), "/")
		asset, exists := assetCache[urlPath]
		if !exists {
			http.NotFound(w, req)
			return
		}

		w.Header().Set("Content-Type", asset.contentType)
		w.Header().Set("ETag", asset.etag)
		w.Header().Set("Vary", "Accept-Encoding")

		// Immutable when the caller asked for a specific build of this file.
		//
		// AssetURL appends ?v=<content hash>, so a request carrying a v that
		// matches the asset we hold is asking for content that cannot change:
		// editing the file changes the hash, which changes the URL. Serving
		// that with max-age=86400 made every returning visitor revalidate all
		// nineteen stylesheets and scripts once a day to be told nothing had
		// changed.
		//
		// A request with no v — a hand-typed URL, an old bookmark — keeps the
		// conservative window, because nothing then ties the URL to a build.
		switch {
		case matchesAssetVersion(req, asset):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case strings.Contains(urlPath, "vendor"), strings.Contains(urlPath, "fonts"),
			strings.HasSuffix(urlPath, ".woff2"):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		default:
			w.Header().Set("Cache-Control", "public, max-age=86400, stale-while-revalidate=604800")
		}

		// Conditional GET: 304 Not Modified for zero-payload instant responses
		if match := req.Header.Get("If-None-Match"); match != "" {
			if match == asset.etag || match == "*" {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}

		body := asset.content
		if asset.gzipped != nil && acceptsGzip(req) {
			// Setting Content-Encoding here also stops the router's Compress
			// middleware from touching the response: it declines to compress a
			// body that is already encoded. That is the point — without it the
			// stored copy would be gzipped a second time.
			w.Header().Set("Content-Encoding", "gzip")
			body = asset.gzipped
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	})

	r.Get("/robots.txt", func(w http.ResponseWriter, req *http.Request) {
		asset, exists := assetCache["/robots.txt"]
		if !exists {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600, must-revalidate")
		w.Header().Set("Content-Signal", "ai-train=no, search=yes, ai-input=no")

		content := asset.content
		if DynamicRobotsTxtFetcher != nil {
			if dyn, err := DynamicRobotsTxtFetcher(req.Context()); err == nil && strings.TrimSpace(dyn) != "" {
				content = []byte(dyn)
			}
		}
		content = []byte(publicRobots(string(content)))
		// The tag is the content's hash: a length-only tag served a stale file
		// to every cache after an edit that kept the length.
		etag := fmt.Sprintf(`"%x"`, sha256.Sum256(content))

		w.Header().Set("ETag", etag)
		if match := req.Header.Get("If-None-Match"); match != "" && (match == etag || match == "*") {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	})
}

// Layout templates resolve asset URLs through layouts.Asset, which cannot call
// into this package directly without an import cycle. Installing the resolver
// here keeps the hash computation in one place.
func init() {
	layouts.Asset = AssetURL
}

// publicRobots removes the lines that name private areas.
//
// robots.txt is public and read by anyone, so a Disallow line for /admin/ is a
// map of where the admin screens are rather than a protection. Those areas are
// kept out of search indexes by the X-Robots-Tag header httpx.SecurityHeaders
// sends on every response under them, and by authentication; the file keeps
// every other directive the operator wrote.
func publicRobots(content string) string {
	lines := strings.Split(content, "\n")
	out := lines[:0]
	for _, line := range lines {
		field, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok && strings.EqualFold(strings.TrimSpace(field), "disallow") && httpx.PrivateArea(strings.TrimSpace(value)) {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
