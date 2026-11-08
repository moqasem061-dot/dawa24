package postgres

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

func TestAsSystemOnlyInNamedFunctions(t *testing.T) {
	fset := token.NewFileSet()
	packages, err := parser.ParseDir(fset, ".", func(info fs.FileInfo) bool { return !strings.HasSuffix(info.Name(), "_test.go") }, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	allowedComment := regexp.MustCompile(`plan\.md E[1-8]`)
	for _, pkg := range packages {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					sel, ok := call.Fun.(*ast.SelectorExpr)
					if !ok || sel.Sel.Name != "AsSystem" {
						return true
					}
					id, ok := sel.X.(*ast.Ident)
					if !ok || id.Name != "database" {
						return true
					}
					if !strings.HasSuffix(fn.Name.Name, "AsSystem") && !strings.HasSuffix(fn.Name.Name, "AsStaff") {
						t.Errorf("%s: AsSystem in %s", fset.Position(call.Pos()), fn.Name.Name)
					}
					if fn.Doc == nil || !allowedComment.MatchString(fn.Doc.Text()) {
						t.Errorf("%s: missing named exception comment", fset.Position(fn.Pos()))
					}
					return true
				})
			}
		}
	}
}
