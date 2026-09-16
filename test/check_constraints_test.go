package test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	reCheckIn = regexp.MustCompile(`(?is)CHECK\s*\(\s*([a-z_]+)\s+IN\s*\(([^)]+)\)\s*\)`)
	reGoConst = regexp.MustCompile(`(?m)^\s*(?:[A-Za-z0-9_]+)\s*(?:[A-Za-z0-9_]+)?\s*=\s*"([^"]+)"`)
)

// loadCheckConstraints extracts column -> allowed string values from all migrations.
func loadCheckConstraints(t *testing.T, dir string) map[string]map[string]bool {
	t.Helper()

	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations found in %s: %v", dir, err)
	}

	constraints := map[string]map[string]bool{}

	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		sql := reComment.ReplaceAllString(string(raw), "")

		for _, m := range reCheckIn.FindAllStringSubmatch(sql, -1) {
			col := strings.ToLower(strings.TrimSpace(m[1]))
			valuesList := m[2]

			if constraints[col] == nil {
				constraints[col] = map[string]bool{}
			}

			// Extract 'val1', 'val2', ...
			for _, valPart := range strings.Split(valuesList, ",") {
				val := strings.Trim(strings.TrimSpace(valPart), "'\"")
				if val != "" {
					constraints[col][val] = true
				}
			}
		}
	}
	return constraints
}

// TestCheckConstraintValues verifies that domain constants in internal/modules
// do not assign values disallowed by PostgreSQL CHECK constraints.
func TestCheckConstraintValues(t *testing.T) {
	root := ".."
	constraints := loadCheckConstraints(t, filepath.Join(root, "db", "migrations"))
	if len(constraints) == 0 {
		t.Fatalf("failed to load any CHECK constraints from migrations")
	}

	var goFiles []string
	err := filepath.Walk(filepath.Join(root, "internal", "modules"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			goFiles = append(goFiles, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal/modules: %v", err)
	}

	checked := 0
	for _, f := range goFiles {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		rel := filepath.ToSlash(f)

		// Find string constants and check if their type/name matches constrained columns
		for _, m := range reGoConst.FindAllStringSubmatch(string(raw), -1) {
			val := m[1]
			// We check known constrained columns
			for col, allowed := range constraints {
				// If a constant matches the pattern of a known enum domain, verify it
				if isRelevantColumnValue(col, val) {
					if !allowed[val] {
						t.Errorf("%s: uses value %q for %s, which violates migration CHECK constraints (%v)",
							rel, val, col, allowed)
					} else {
						checked++
					}
				}
			}
		}
	}

	t.Logf("verified %d domain constants against migration CHECK constraints across %d columns", checked, len(constraints))
}

func isRelevantColumnValue(col, val string) bool {
	// Specific heuristic mappings to avoid false positives on arbitrary strings
	switch col {
	case "status":
		switch val {
		case "pending", "approved", "rejected", "suspended", "active", "inactive", "confirmed",
			"processing", "on_hold", "shipped", "in_transit", "out_for_delivery", "delivered",
			"completed", "cancelled", "failed", "returned", "refunded", "unread", "read", "resolved":
			return true
		}
	case "payment_status":
		switch val {
		case "unpaid", "authorized", "paid", "partially_refunded", "refunded", "failed":
			return true
		}
	case "policy_type":
		switch val {
		case "terms", "returns", "privacy", "shipping", "warranty":
			return true
		}
	case "type":
		switch val {
		case "supplier", "company", "agency", "pharmacy", "chain_pharmacy":
			return true
		}
	}
	return false
}
