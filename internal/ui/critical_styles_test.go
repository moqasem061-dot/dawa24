package ui_test

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestHomepageCriticalCSSIsCurrent(t *testing.T) {
	var filenames []string
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "critical" {
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(path) {
		case ".templ", ".css", ".woff2":
			filenames = append(filenames, "internal/ui/"+filepath.ToSlash(path))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	filenames = append(filenames, "internal/ui/static_build.go", "scripts/pagespeed/critical.cjs")
	slices.Sort(filenames)
	hash := sha256.New()
	for _, filename := range filenames {
		content, err := os.ReadFile(filepath.Join("../..", filename))
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Ext(filename) != ".woff2" {
			content = []byte(strings.ReplaceAll(string(content), "\r\n", "\n"))
		}
		_, _ = hash.Write([]byte(filename + "\x00"))
		_, _ = hash.Write(content)
		_, _ = hash.Write([]byte{0})
	}
	want, err := os.ReadFile("static/critical/home.sha256")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", hash.Sum(nil)) != strings.TrimSpace(string(want)) {
		t.Fatal("homepage critical CSS is stale; regenerate with scripts/pagespeed/critical.cjs (see docs/modules/ui.md)")
	}
}
