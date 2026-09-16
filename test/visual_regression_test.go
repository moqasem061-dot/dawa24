package test

import (
	"context"
	"crypto/md5"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// TestVisualRegressionMatrix captures the Phase 3 component matrix across
// all 8 combinations: 2 densities (comfortable, is-compact) x 2 themes (light, dark) x 2 directions (RTL, LTR).
func TestVisualRegressionMatrix(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping visual regression in short mode")
	}

	chromePath := `C:\Program Files\Google\Chrome\Application\chrome.exe`
	if _, err := os.Stat(chromePath); err != nil {
		chromePath = `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`
		if _, err := os.Stat(chromePath); err != nil {
			t.Skip("no headless browser (Chrome/Edge) available on host")
		}
	}

	outDir, err := filepath.Abs(filepath.Join("..", "test", "visual_baselines"))
	if err != nil {
		t.Fatalf("resolving baseline dir: %v", err)
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("creating baseline dir: %v", err)
	}

	// Serve the component gallery
	r := chi.NewRouter()
	r.Get("/admin/gallery", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		density := req.URL.Query().Get("density")
		if density == "" {
			density = "comfortable"
		}
		theme := req.URL.Query().Get("theme")
		if theme == "" {
			theme = "light"
		}
		dir := req.URL.Query().Get("dir")
		if dir == "" {
			dir = "rtl"
		}
		lang := req.URL.Query().Get("lang")
		if lang == "" {
			lang = "ar"
		}

		ctx := authctx.WithActor(req.Context(), authctx.Actor{
			UserID: 1,
			Role:   "admin",
		})

		props := pages.ComponentGalleryProps{
			Density: density,
			Theme:   theme,
			Lang:    lang,
			Dir:     dir,
		}
		_ = pages.ComponentGallery(props).Render(ctx, w)
	})

	// Serve static CSS/JS assets for accurate rendering
	staticDir, err := filepath.Abs(filepath.Join("..", "internal", "ui", "static"))
	if err != nil {
		t.Fatalf("resolving static dir: %v", err)
	}
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))

	ts := httptest.NewServer(r)
	defer ts.Close()

	densities := []string{"comfortable", "is-compact"}
	themes := []string{"light", "dark"}
	directions := []struct {
		dir  string
		lang string
	}{
		{dir: "rtl", lang: "ar"},
		{dir: "ltr", lang: "en"},
	}

	capturedCount := 0
	for _, density := range densities {
		for _, theme := range themes {
			for _, d := range directions {
				filename := fmt.Sprintf("gallery_%s_%s_%s.png", density, theme, d.dir)
				targetPath := filepath.Join(outDir, filename)

				url := fmt.Sprintf("%s/admin/gallery?density=%s&theme=%s&dir=%s&lang=%s", ts.URL, density, theme, d.dir, d.lang)

				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				cmd := exec.CommandContext(ctx, chromePath,
					"--headless=new",
					"--disable-gpu",
					"--no-sandbox",
					"--disable-web-security",
					"--hide-scrollbars",
					"--window-size=1280,1800",
					fmt.Sprintf("--screenshot=%s", targetPath),
					url,
				)

				out, err := cmd.CombinedOutput()
				cancel()
				if err != nil {
					t.Fatalf("capturing %s: %v (output: %s)", filename, err, string(out))
				}

				data, err := os.ReadFile(targetPath)
				if err != nil || len(data) < 1000 {
					t.Fatalf("screenshot %s invalid or empty (len: %d)", filename, len(data))
				}

				hash := fmt.Sprintf("%x", md5.Sum(data))
				t.Logf("? Captured baseline: %-38s %7d bytes  md5:%s", filename, len(data), hash)
				capturedCount++
			}
		}
	}

	if capturedCount != 8 {
		t.Fatalf("expected 8 baseline captures, got %d", capturedCount)
	}
}
