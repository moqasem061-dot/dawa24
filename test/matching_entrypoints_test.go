package test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
)

// TestMatchingEntrypoints enforces that any internal Go file calling
// productmatch / matchflow entry points (.Match, MatchAll, Run, NewIndex)
// is registered in catalog/matching.EntryPoints or is explicitly allowlisted.
func TestMatchingEntrypoints(t *testing.T) {
	const (
		productmatchImport = "github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
		matchflowImport    = "github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	)

	allowedFiles := make(map[string]bool)
	for _, ep := range matching.EntryPoints {
		clean := filepath.ToSlash(ep.File)
		allowedFiles[clean] = true
	}

	allowlistPrefixes := []string{
		"internal/shared/productmatch/",
		"internal/shared/matchflow/",
		"internal/modules/catalog/matching/",
		"test/corpus/",
	}

	repoRoot := ".."
	internalDir := filepath.Join(repoRoot, "internal")

	var violations []string

	err := filepath.WalkDir(internalDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_templ.go") {
			return nil
		}

		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		relPath := filepath.ToSlash(rel)

		for _, pfx := range allowlistPrefixes {
			if strings.HasPrefix(relPath, pfx) {
				return nil
			}
		}

		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}

		importsProductmatch := false
		importsMatchflow := false
		for _, imp := range f.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				continue
			}
			if p == productmatchImport {
				importsProductmatch = true
			}
			if p == matchflowImport {
				importsMatchflow = true
			}
		}

		hasMatchingCall := false
		ast.Inspect(f, func(n ast.Node) bool {
			if hasMatchingCall {
				return false
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			switch fun := call.Fun.(type) {
			case *ast.SelectorExpr:
				if pkgIdent, ok := fun.X.(*ast.Ident); ok {
					if pkgIdent.Name == "productmatch" && (fun.Sel.Name == "MatchAll" || fun.Sel.Name == "NewIndex") {
						hasMatchingCall = true
						return false
					}
					if pkgIdent.Name == "matchflow" && fun.Sel.Name == "Run" {
						hasMatchingCall = true
						return false
					}
				}
				if importsProductmatch && (fun.Sel.Name == "Match" || fun.Sel.Name == "MatchAll" || fun.Sel.Name == "NewIndex") {
					hasMatchingCall = true
					return false
				}
				if importsMatchflow && fun.Sel.Name == "Run" {
					hasMatchingCall = true
					return false
				}
			case *ast.Ident:
				if importsProductmatch && fun.Name == "NewIndex" {
					hasMatchingCall = true
					return false
				}
			}
			return true
		})

		if hasMatchingCall && !allowedFiles[relPath] {
			violations = append(violations, relPath)
		}

		return nil
	})

	if err != nil {
		t.Fatalf("walking internal directory: %v", err)
	}

	if len(violations) > 0 {
		t.Errorf("unregistered matching entry points found:\n  %s\nAdd them to matching.EntryPoints or refactor.",
			strings.Join(violations, "\n  "))
	}
}
