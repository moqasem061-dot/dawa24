package ui

import (
	"embed"
	"net/http"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/ui/layouts"
)

//go:embed tutorials/vendor.html tutorials/pharmacy.html tutorials/driver.html
var tutorialMaps embed.FS

func tutorialMap(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		content, err := tutorialMaps.ReadFile("tutorials/" + name + ".html")
		if err != nil {
			http.Error(w, "tutorial unavailable", http.StatusInternalServerError)
			return
		}
		rendered := strings.ReplaceAll(string(content), "https://dawa24.com", "")
		rendered = strings.ReplaceAll(rendered, "http://dawa24.com", "")
		rendered = strings.ReplaceAll(rendered, "<style>", `<style nonce="`+layouts.Nonce(r.Context())+`">`)
		content = []byte(rendered)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "private, no-cache")
		_, _ = w.Write(content)
	}
}
