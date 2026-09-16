// Package test holds checks that span packages.
//
// This one substitutes, partially, for integration tests we cannot run without
// a live PostgreSQL: it parses the migrations to build the expected schema, then
// verifies that every column named in a repository SELECT actually exists.
//
// It exists because that class of bug is invisible to the compiler and to every
// unit test with a mocked repository — the Go code compiles, the mocks are
// happy, and the query fails at runtime with `column "..." does not exist`,
// taking the endpoint with it. When first written, this check found 28 such
// columns across the address book, organization and branch repositories, all of
// which would have 500'd on first contact with a real database.
package test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	reCreateTable = regexp.MustCompile(`(?is)CREATE TABLE (?:IF NOT EXISTS )?([a-z_]+)\.([a-z_]+)\s*\((.*?)\n\);`)
	reColumnDef   = regexp.MustCompile(`^([a-z_][a-z_0-9]*)\s+[A-Za-z]`)
	reAlterTable  = regexp.MustCompile(`(?is)ALTER TABLE ([a-z_]+)\.([a-z_]+)(.*?);`)
	reAddColumn   = regexp.MustCompile(`(?i)ADD COLUMN (?:IF NOT EXISTS )?([a-z_][a-z_0-9]*)`)
	reRenameCol   = regexp.MustCompile(`(?i)RENAME COLUMN ([a-z_][a-z_0-9]*) TO ([a-z_][a-z_0-9]*)`)
	reSQLLiteral  = regexp.MustCompile("(?s)`([^`]*?)`")
	reSelectFrom  = regexp.MustCompile(`(?is)SELECT\s+(.*?)\s+FROM\s+([a-z_]+\.[a-z_]+)`)
	reIdent       = regexp.MustCompile(`^[a-z_][a-z_0-9]*$`)
	reComment     = regexp.MustCompile(`--[^\n]*`)
)

var nonColumnPrefixes = []string{"CONSTRAINT", "PRIMARY", "UNIQUE", "FOREIGN", "CHECK", "EXCLUDE"}

// loadSchema builds table -> columns from the migration files.
func loadSchema(t *testing.T, dir string) map[string]map[string]bool {
	t.Helper()

	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations found in %s: %v", dir, err)
	}

	schema := map[string]map[string]bool{}
	add := func(table, col string) {
		if schema[table] == nil {
			schema[table] = map[string]bool{}
		}
		schema[table][col] = true
	}

	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		sql := reComment.ReplaceAllString(string(raw), "")

		for _, m := range reCreateTable.FindAllStringSubmatch(sql, -1) {
			table := m[1] + "." + m[2]
			for _, line := range strings.Split(m[3], "\n") {
				line = strings.TrimSpace(line)
				if isNonColumn(line) {
					continue
				}
				if cm := reColumnDef.FindStringSubmatch(line); cm != nil {
					add(table, cm[1])
				}
			}
		}

		// Applied in file order, so a rename in a later migration supersedes
		// the original name rather than adding a second one.
		for _, m := range reAlterTable.FindAllStringSubmatch(sql, -1) {
			table, body := m[1]+"."+m[2], m[3]
			for _, c := range reAddColumn.FindAllStringSubmatch(body, -1) {
				add(table, c[1])
			}
			for _, c := range reRenameCol.FindAllStringSubmatch(body, -1) {
				if schema[table] != nil {
					delete(schema[table], c[1])
				}
				add(table, c[2])
			}
		}
	}
	return schema
}

func isNonColumn(line string) bool {
	upper := strings.ToUpper(line)
	for _, p := range nonColumnPrefixes {
		if strings.HasPrefix(upper, p+" ") || strings.HasPrefix(upper, p+"(") || upper == p {
			return true
		}
	}
	return false
}

func TestRepositorySQLMatchesMigrations(t *testing.T) {
	root := ".."
	schema := loadSchema(t, filepath.Join(root, "db", "migrations"))
	if len(schema) < 50 {
		t.Fatalf("parsed only %d tables from migrations; the parser is probably broken", len(schema))
	}

	var goFiles []string
	err := filepath.Walk(filepath.Join(root, "internal"), func(path string, info os.FileInfo, err error) error {
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
		rel := filepath.ToSlash(f)

		for _, lit := range reSQLLiteral.FindAllStringSubmatch(string(raw), -1) {
			query := lit[1]
			if !strings.Contains(strings.ToUpper(query), "SELECT") {
				continue
			}
			m := reSelectFrom.FindStringSubmatch(query)
			if m == nil {
				continue
			}
			cols, table := m[1], strings.ToLower(m[2])

			// Postgres catalog views are not created by our migrations. The
			// trash screens read information_schema on purpose: the list of
			// soft-deletable tables is discovered rather than hand-maintained.
			if strings.HasPrefix(table, "information_schema.") || strings.HasPrefix(table, "pg_") {
				continue
			}

			if _, ok := schema[table]; !ok {
				t.Errorf("%s: selects FROM %s, which no migration creates", rel, table)
				continue
			}
			// Skip anything this deliberately simple parser cannot read
			// correctly: wildcards, function calls, and joined queries where a
			// qualified name may belong to a different table.
			if strings.Contains(cols, "*") || strings.Contains(cols, "(") ||
				strings.Contains(cols, ".") || strings.Contains(strings.ToUpper(query), "JOIN") {
				continue
			}
			checked++

			for _, c := range strings.Split(cols, ",") {
				name := strings.TrimSpace(c)
				if i := strings.IndexAny(name, " \t\n"); i > 0 {
					name = name[:i]
				}
				if !reIdent.MatchString(name) {
					continue
				}
				if !schema[table][name] {
					t.Errorf("%s: %s.%s is selected but no migration defines it",
						rel, table, name)
				}
			}
		}
	}

	if checked < 40 {
		t.Errorf("only %d SELECT statements were checked; the extractor is probably missing queries", checked)
	}
	t.Logf("verified %d SELECT statements against %d tables", checked, len(schema))
}

var (
	reInsertInto = regexp.MustCompile(`(?is)INSERT INTO\s+([a-z_]+\.[a-z_]+)\s*\(([^)]*)\)`)
	reUpdateSet  = regexp.MustCompile(`(?is)UPDATE\s+([a-z_]+\.[a-z_]+)\s+SET\s+(.*?)(?:\bWHERE\b|;|$)`)
	reAssignment = regexp.MustCompile(`(?m)(?:^|,)\s*([a-z_][a-z_0-9]*)\s*=`)
)

// TestWriteSQLMatchesMigrations covers the other half: columns a repository
// writes to.
//
// SELECT coverage alone missed three columns added by an endpoint whose table
// had never gained them, because a write is just as capable of naming a column
// that does not exist — and a failing write loses data rather than merely
// failing a read.
func TestWriteSQLMatchesMigrations(t *testing.T) {
	root := ".."
	schema := loadSchema(t, filepath.Join(root, "db", "migrations"))

	var goFiles []string
	err := filepath.Walk(filepath.Join(root, "internal"), func(path string, info os.FileInfo, err error) error {
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

	inserts, updates := 0, 0
	for _, f := range goFiles {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		rel := filepath.ToSlash(f)

		for _, lit := range reSQLLiteral.FindAllStringSubmatch(string(raw), -1) {
			query := lit[1]

			for _, m := range reInsertInto.FindAllStringSubmatch(query, -1) {
				table := strings.ToLower(m[1])
				if _, ok := schema[table]; !ok {
					t.Errorf("%s: inserts into %s, which no migration creates", rel, table)
					continue
				}
				inserts++
				for _, c := range strings.Split(m[2], ",") {
					name := strings.TrimSpace(c)
					if !reIdent.MatchString(name) {
						continue
					}
					if !schema[table][name] {
						t.Errorf("%s: INSERT names %s.%s but no migration defines it", rel, table, name)
					}
				}
			}

			for _, m := range reUpdateSet.FindAllStringSubmatch(query, -1) {
				table := strings.ToLower(m[1])
				if _, ok := schema[table]; !ok {
					t.Errorf("%s: updates %s, which no migration creates", rel, table)
					continue
				}
				updates++
				for _, a := range reAssignment.FindAllStringSubmatch(m[2], -1) {
					name := a[1]
					if !schema[table][name] {
						t.Errorf("%s: UPDATE sets %s.%s but no migration defines it", rel, table, name)
					}
				}
			}
		}
	}

	if inserts < 20 || updates < 20 {
		t.Errorf("only %d INSERT and %d UPDATE statements checked; the extractor is probably missing queries",
			inserts, updates)
	}
	t.Logf("verified %d INSERT and %d UPDATE statements", inserts, updates)
}
