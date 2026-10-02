package ui

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"mime"
	"path"
	"regexp"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

// Bundles preserve the cascade order from Base. Source files stay readable and
// are also served individually for page-specific styles and boosted navigation.
var stylesheetBundles = map[string][]string{
	"/css/shell.css": {
		"tokens", "fonts", "base", "layout", "components", "nav",
		"foundations", "utilities", "wizard", "app",
	},
	"/css/public-shell.css": {"customer", "public", "marketing"},
}

func initStaticAssetCache() {
	err := fs.WalkDir(staticFS, "static", func(filename string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		content, err := staticFS.ReadFile(filename)
		if err != nil {
			return err
		}
		assetCache[strings.TrimPrefix(filename, "static")] = buildStaticAsset(filename, content)
		return nil
	})
	if err != nil {
		panic(fmt.Errorf("read embedded assets: %w", err))
	}
	versionFontURLs()
	for filename, sources := range stylesheetBundles {
		var css bytes.Buffer
		for _, source := range sources {
			css.Write(assetCache["/css/"+source+".css"].content)
			css.WriteByte('\n')
		}
		assetCache[filename] = buildStaticAsset(filename, css.Bytes())
	}
}

func versionFontURLs() {
	const filename = "/css/fonts.css"
	fontURL := regexp.MustCompile(`/static/fonts/[a-z0-9.-]+\.woff2`)
	css := fontURL.ReplaceAllStringFunc(string(assetCache[filename].content), func(url string) string {
		font := assetCache[strings.TrimPrefix(url, "/static")]
		return url + "?v=" + assetVersion(font.etag)
	})
	assetCache[filename] = buildStaticAsset(filename, []byte(css))
}

func buildStaticAsset(filename string, content []byte) *staticAsset {
	content = minifyStaticAsset(filename, content)
	digest := sha256.Sum256(content)
	asset := &staticAsset{
		content:     content,
		etag:        fmt.Sprintf(`"%x"`, digest[:16]),
		contentType: staticContentType(filename),
	}
	if compressibleAsset(asset.contentType) {
		asset.gzipped = gzipAsset(content)
	}
	return asset
}

func minifyStaticAsset(filename string, content []byte) []byte {
	var loader api.Loader
	switch path.Ext(filename) {
	case ".css":
		loader = api.LoaderCSS
	case ".js":
		loader = api.LoaderJS
	default:
		return content
	}
	compiled := api.Transform(string(content), api.TransformOptions{
		Loader: loader, Sourcefile: filename,
		MinifyWhitespace: true, MinifyIdentifiers: true, MinifySyntax: true,
		LegalComments: api.LegalCommentsInline,
		Engines: []api.Engine{
			{Name: api.EngineChrome, Version: "111"},
			{Name: api.EngineFirefox, Version: "113"},
			{Name: api.EngineSafari, Version: "16.4"},
		},
	})
	if len(compiled.Errors) > 0 {
		panic(fmt.Sprintf("minify embedded asset %s: %s", filename, compiled.Errors[0].Text))
	}
	return compiled.Code
}

func gzipAsset(content []byte) []byte {
	var compressed bytes.Buffer
	writer, err := gzip.NewWriterLevel(&compressed, gzip.BestCompression)
	if err != nil {
		panic(err)
	}
	if _, err := writer.Write(content); err != nil {
		panic(err)
	}
	if err := writer.Close(); err != nil {
		panic(err)
	}
	if compressed.Len() >= len(content) {
		return nil
	}
	return compressed.Bytes()
}

func staticContentType(filename string) string {
	// Windows registry MIME associations must not make scripts text/plain.
	switch path.Ext(filename) {
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".woff2":
		return "font/woff2"
	case ".webp":
		return "image/webp"
	case ".avif":
		return "image/avif"
	}
	if contentType := mime.TypeByExtension(path.Ext(filename)); contentType != "" {
		return contentType
	}
	return "application/octet-stream"
}
