package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// nullscan finds columns that are nullable in PostgreSQL but selected into a
// non-pointer Go field.
//
// pgx cannot scan NULL into a plain string, so the first row with that field
// empty fails the whole query and takes the endpoint with it. This has already
// happened twice: org.organizations.tax_number took down /org/organizations,
// and catalog.products.sku took down /catalog/products. Both compiled, passed
// every unit test, and failed on real data.
//
// The check is deliberately blunt: it reports every nullable text-like column
// that appears in a repository SELECT list. Reviewing the list is quick;
// discovering one of these in production is not.
func nullscan(dsn string) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	// Nullable, non-defaulted scalar columns are the risky ones. A column with a
	// default still returns NULL for rows written before the default existed.
	rows, err := conn.Query(ctx, `
		SELECT table_schema || '.' || table_name, column_name
		FROM information_schema.columns
		WHERE table_schema NOT IN ('pg_catalog','information_schema')
		  AND is_nullable = 'YES'
		  AND data_type IN ('text','character varying','character')
		ORDER BY 1, 2`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "query columns: %v\n", err)
		os.Exit(1)
	}

	nullable := map[string]map[string]bool{}
	for rows.Next() {
		var table, col string
		if err := rows.Scan(&table, &col); err != nil {
			break
		}
		if nullable[table] == nil {
			nullable[table] = map[string]bool{}
		}
		nullable[table][col] = true
	}
	rows.Close()

	// A column under a unique index must not be back-filled to ''. PostgreSQL
	// treats NULLs as distinct, so any number of rows may have a NULL sku; the
	// moment they all become '' the index rejects the second one. Those columns
	// have to be scanned into a *string instead.
	uniq := map[string]bool{}
	urows, err := conn.Query(ctx, `
		SELECT DISTINCT n.nspname || '.' || t.relname || '.' || a.attname
		FROM pg_index i
		JOIN pg_class t ON t.oid = i.indrelid
		JOIN pg_namespace n ON n.oid = t.relnamespace
		JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY(i.indkey)
		WHERE i.indisunique
		  AND n.nspname NOT IN ('pg_catalog','information_schema')
		  AND NOT a.attnotnull`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "query unique: %v\n", err)
		os.Exit(1)
	}
	for urows.Next() {
		var c string
		if err := urows.Scan(&c); err != nil {
			break
		}
		uniq[c] = true
	}
	urows.Close()

	reSQL := regexp.MustCompile("(?s)`([^`]*?)`")
	reSelect := regexp.MustCompile(`(?is)SELECT\s+(.*?)\s+FROM\s+([a-z_]+\.[a-z_]+)`)

	type hit struct{ file, table, col string }
	var hits []hit
	seen := map[string]bool{}

	err = filepath.Walk("internal", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".go") ||
			!strings.Contains(filepath.ToSlash(path), "/postgres/") ||
			strings.HasSuffix(path, "_test.go") {
			return nil
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(path)

		for _, lit := range reSQL.FindAllStringSubmatch(string(raw), -1) {
			m := reSelect.FindStringSubmatch(lit[1])
			if m == nil {
				continue
			}
			cols, table := m[1], strings.ToLower(m[2])
			if strings.Contains(cols, "*") || strings.Contains(cols, "(") {
				continue
			}
			for _, c := range strings.Split(cols, ",") {
				name := strings.TrimSpace(c)
				if i := strings.IndexAny(name, " \t\n"); i > 0 {
					name = name[:i]
				}
				name = strings.TrimPrefix(name[strings.LastIndex(name, ".")+1:], " ")
				if !nullable[table][name] {
					continue
				}
				key := table + "." + name
				if seen[key] {
					continue
				}
				seen[key] = true
				hits = append(hits, hit{rel, table, name})
			}
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "walk: %v\n", err)
		os.Exit(1)
	}

	if len(hits) == 0 {
		fmt.Println("no nullable text columns are selected into repository scans")
		return
	}

	// Two populations with two different remedies. Printing them together would
	// invite the wrong fix on the unique ones.
	var backfill, pointer []hit
	for _, h := range hits {
		if uniq[h.table+"."+h.col] {
			pointer = append(pointer, h)
			continue
		}
		backfill = append(backfill, h)
	}

	fmt.Printf("%d nullable text column(s) selected by a repository.\n\n", len(hits))

	fmt.Printf("-- %d safe to make NOT NULL DEFAULT '' (no unique index) --\n", len(backfill))
	for _, h := range backfill {
		fmt.Printf("  %-44s %s\n", h.table+"."+h.col, h.file)
	}

	fmt.Printf("\n-- %d MUST be scanned into *string (unique index; '' would collide) --\n", len(pointer))
	for _, h := range pointer {
		fmt.Printf("  %-44s %s\n", h.table+"."+h.col, h.file)
	}
}

// auditPeek prints the most recent audit rows, to confirm that admin mutations
// are recording what they claim to.
func auditPeek(dsn string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx, `
		SELECT action, entity_type, entity_id,
		       COALESCE(before::text, '-'), COALESCE(after::text, '-'), created_at
		FROM platform.audit_log
		ORDER BY created_at DESC
		LIMIT 10`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "query audit: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()

	n := 0
	for rows.Next() {
		var action, entity, id, before, after string
		var at time.Time
		if err := rows.Scan(&action, &entity, &id, &before, &after, &at); err != nil {
			break
		}
		fmt.Printf("  %s  %-34s %s#%s\n      before=%s after=%s\n",
			at.Format("15:04:05"), action, entity, id, before, after)
		n++
	}
	if n == 0 {
		fmt.Println("no audit rows")
	}
}
