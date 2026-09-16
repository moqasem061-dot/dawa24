package ui_test

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain points every upload in this package at a temporary directory.
//
// The handlers under test write real files. Without this they write relative to
// the package directory — internal/ui/data/uploads — so a plain `go test ./...`
// left dozens of files in the working tree. Some of them had been committed,
// which meant the suite also *deleted* tracked files, and the resulting diff
// looked like a deliberate change.
//
// UPLOAD_DIR is the same variable production uses, so this exercises the
// configured path rather than a test-only branch.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "dawa24-ui-uploads-")
	if err != nil {
		panic("test uploads dir: " + err.Error())
	}
	if err := os.Setenv("UPLOAD_DIR", dir); err != nil {
		panic("set UPLOAD_DIR: " + err.Error())
	}
	if err := os.Setenv("DATA_DIR", filepath.Dir(dir)); err != nil {
		panic("set DATA_DIR: " + err.Error())
	}

	code := m.Run()

	_ = os.RemoveAll(dir)
	os.Exit(code)
}
