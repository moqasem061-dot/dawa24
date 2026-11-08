package erp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoOrgasoftOutsideProvider walks the erp module and fails when any
// non-test Go file names the provider anywhere except the registry and the
// registration import (trap 28). The provider seam lives in
// internal/modules/erp/provider; the rest of the module calls provider.Get.
func TestNoOrgasoftOutsideProvider(t *testing.T) {
	roots := []string{
		".",
		filepath.Join("provider"),
	}
	allowed := map[string]bool{
		normalize(filepath.Join("provider", "registry.go")): true,
		// provider/all/all.go holds the blank import of the registration.
		normalize(filepath.Join("provider", "all", "all.go")): true,
	}
	skipped := map[string]bool{
		// The provider's own package owns its spellings.
		normalize(filepath.Join("provider", "orgasoft")): true,
	}
	visited := map[string]bool{}
	var walk func(dir string)
	walk = func(dir string) {
		if skipped[normalize(dir)] {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, entry := range entries {
			path := filepath.Join(dir, entry.Name())
			if visited[normalize(path)] {
				continue
			}
			visited[normalize(path)] = true
			key := normalize(path)
			if entry.IsDir() {
				walk(path)
				continue
			}
			if strings.HasSuffix(entry.Name(), "_test.go") || !strings.HasSuffix(entry.Name(), ".go") {
				continue
			}
			if allowed[key] {
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "orgasoft") {
				t.Errorf("%s names the provider; only the registry and the registration import may", path)
			}
		}
	}
	for _, root := range roots {
		walk(root)
	}
	if !visited[normalize(filepath.Join("provider", "registry.go"))] {
		t.Fatal("the walk never reached the provider package; the layout changed")
	}
}

// TestNeverCallsEnhancement proves the matcher seam is deterministic-only: the
// erp module neither names StartEnhancement nor imports the gateway (trap 18).
func TestNeverCallsEnhancement(t *testing.T) {
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "StartEnhancement") {
			t.Errorf("%s references StartEnhancement; the ERP matcher must stay deterministic", path)
		}
		if strings.Contains(string(data), "internal/platform/gateway") {
			t.Errorf("%s imports the gateway; the ERP module must never reach it", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func normalize(p string) string {
	return strings.TrimPrefix(filepath.ToSlash(p), "./")
}
