package test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// knownCrossModuleImports would list tolerated module-to-module imports.
// There are none: ingest, marketing and the chat bridges reach other modules
// through ports that cmd/server adapts, and chatbridge lives in platform/.
// Keep it empty. A new entry is a design problem to solve, not to record.
var knownCrossModuleImports = map[string]string{}

// TestModulesDoNotImportEachOther enforces AGENTS.md rule 5 for modules:
// internal/modules/A may not import internal/modules/B. depguard covers the
// shared/ and platform/ directions; it cannot express "any module but your own".
func TestModulesDoNotImportEachOther(t *testing.T) {
	const prefix = "github.com/moqasem061-dot/dawa24/internal/modules/"
	root := filepath.Join("..", "internal", "modules")

	seen := map[string]bool{}
	var violations []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		own := strings.Split(filepath.ToSlash(rel), "/")[0]
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil || !strings.HasPrefix(p, prefix) {
				continue
			}
			other := strings.Split(strings.TrimPrefix(p, prefix), "/")[0]
			if other == own {
				continue
			}
			edge := own + " -> " + other
			seen[edge] = true
			if _, known := knownCrossModuleImports[edge]; !known {
				violations = append(violations, filepath.ToSlash(rel)+" imports "+p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk modules: %v", err)
	}

	sort.Strings(violations)
	for _, v := range violations {
		t.Errorf("cross-module import (AGENTS.md rule 5): %s", v)
	}
	// Keep the allowlist honest: an edge that no longer exists must be removed,
	// or it silently becomes permission to reintroduce it.
	for edge := range knownCrossModuleImports {
		if !seen[edge] {
			t.Errorf("knownCrossModuleImports lists %q but no such import remains; delete the entry", edge)
		}
	}
}
