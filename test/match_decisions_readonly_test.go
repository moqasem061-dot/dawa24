package test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNoMatchDecisionsInsertsOutsideMigrations asserts that catalog.match_decisions
// has become read-only and no code writes it (US8 T114).
func TestNoMatchDecisionsInsertsOutsideMigrations(t *testing.T) {
	repoRoot := ".."
	pattern := regexp.MustCompile(`(?i)insert\s+into\s+catalog\.match_decisions`)

	var violations []string

	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".agents" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		relPath := filepath.ToSlash(rel)

		// Allow migrations and specification/documentation markdown
		if strings.HasPrefix(relPath, "db/migrations/") ||
			strings.HasPrefix(relPath, "specs/") ||
			strings.HasPrefix(relPath, "docs/") ||
			strings.HasSuffix(relPath, ".md") {
			return nil
		}

		// Only check Go source and SQL files
		if !strings.HasSuffix(relPath, ".go") && !strings.HasSuffix(relPath, ".sql") {
			return nil
		}

		// Exclude this test itself
		if strings.HasSuffix(relPath, "match_decisions_readonly_test.go") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		lines := bytes.Split(content, []byte("\n"))
		for i, line := range lines {
			trimmed := strings.TrimSpace(string(line))
			if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "--") {
				continue // Skip comments
			}
			if pattern.Match(line) {
				violations = append(violations, relPath+":"+string(rune('0'+i+1)))
			}
		}

		return nil
	})

	if err != nil {
		t.Fatalf("walking repository: %v", err)
	}

	if len(violations) > 0 {
		t.Errorf("found disallowed INSERT INTO catalog.match_decisions outside migrations:\n%s",
			strings.Join(violations, "\n"))
	}
}
