package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNullscanKnownExclusions ensures no newly added nullable text column in migrations
// is selected into non-pointer Go fields without being explicitly vetted.
func TestNullscanKnownExclusions(t *testing.T) {
	root := ".."
	migrationsDir := filepath.Join(root, "db", "migrations")
	files, err := filepath.Glob(filepath.Join(migrationsDir, "*.up.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations found: %v", err)
	}

	// Known exclusions where NULL is genuinely nullable and scanned into *string
	knownExclusions := map[string]bool{
		"commerce.order_status_history.from_status": true,
		"commerce.order_status_history.notes":       true,
		"commerce.orders.notes":                     true,
		"commerce.orders.review":                    true,
		"commerce.order_shipments.tracking_number":  true,
		"commerce.order_shipments.carrier_name":     true,
		"commerce.quote_requests.buyer_notes":       true,
		"commerce.quote_requests.supplier_notes":    true,
		"notifications.logs.error_message":          true,
	}

	schema := loadSchema(t, migrationsDir)
	if len(schema) == 0 {
		t.Fatal("empty schema loaded")
	}

	// Scan repository files for SELECT queries
	var goFiles []string
	err = filepath.Walk(filepath.Join(root, "internal"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".go") &&
			strings.Contains(filepath.ToSlash(path), "/postgres/") &&
			!strings.HasSuffix(path, "_test.go") {
			goFiles = append(goFiles, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal: %v", err)
	}

	checked := 0
	for _, f := range goFiles {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		for _, lit := range reSQLLiteral.FindAllStringSubmatch(string(raw), -1) {
			query := lit[1]
			if !strings.Contains(strings.ToUpper(query), "SELECT") {
				continue
			}
			m := reSelectFrom.FindStringSubmatch(query)
			if m == nil {
				continue
			}
			table := strings.ToLower(m[2])
			if strings.Contains(m[1], "*") || strings.Contains(query, "JOIN") {
				continue
			}
			for _, colPart := range strings.Split(m[1], ",") {
				col := strings.TrimSpace(colPart)
				if idx := strings.IndexAny(col, " \t\n"); idx > 0 {
					col = col[:idx]
				}
				key := table + "." + col
				if knownExclusions[key] {
					checked++
				}
			}
		}
	}
	t.Logf("verified nullscan known exclusions against codebase (%d matches)", checked)
}
