// migratecheck dry-runs pending migrations against a real database and always
// rolls back.
//
// It exists because migration 066 reached production and failed there: it
// inserted into a column promo.highlight_sections does not have. Nothing in the
// build, vet or test gates can catch that — Go does not typecheck SQL, and the
// unit tests never touch a database. The only thing that finds it is executing
// the SQL against the real schema, which is what this does.
//
//	go run ./cmd/migratecheck                     # drift check + apply all, roll back
//	go run ./cmd/migratecheck -from 66            # apply 66..latest, roll back
//	go run ./cmd/migratecheck -from 66 -roundtrip # then run every down, roll back
//
// It also refuses when a migration file changed after being applied to this
// database. That is what the deploy runner does, and finding it here costs a
// second instead of a failed release. See drift.go.
//
// The transaction is never committed under any exit path, so it is safe to run
// against production. It does take brief DDL locks, so prefer a quiet moment.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

type migration struct {
	version int
	name    string
	up      string
	down    string
}

var stripTx = regexp.MustCompile(`(?im)^\s*(BEGIN|COMMIT)\s*;\s*$`)

func main() {
	var (
		dir       = flag.String("dir", "db/migrations", "migrations directory")
		from      = flag.Int("from", 0, "first version to run (default: all)")
		to        = flag.Int("to", 1<<30, "last version to run")
		roundtrip = flag.Bool("roundtrip", false, "after the up pass, run every down in reverse")
		timeout   = flag.Duration("timeout", 4*time.Minute, "overall timeout")
	)
	flag.Parse()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is not set")
		os.Exit(2)
	}

	migrations, err := load(*dir, *from, *to)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if len(migrations) == 0 {
		fmt.Println("no migrations in range")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "connect:", err)
		os.Exit(2)
	}
	defer conn.Close(ctx)

	// Drift first: a file edited after it ran means the runner will refuse to
	// migrate at all, so rehearsing SQL it will never reach proves nothing.
	// Checked against every migration on disk, not just the -from range.
	all, err := load(*dir, 0, 1<<30)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	drift, err := checkDrift(ctx, conn, all)
	if err != nil {
		fmt.Fprintln(os.Stderr, "drift check:", err)
		os.Exit(2)
	}
	if len(drift) > 0 {
		reportDrift(drift)
		os.Exit(1)
	}

	if code := run(ctx, conn, migrations, *roundtrip); code != 0 {
		os.Exit(code)
	}
}

// run executes the whole check inside one transaction and rolls it back. The
// migrations run cumulatively, because that is how they will run for real: a
// later one sees the schema the earlier ones produced.
func run(ctx context.Context, conn *pgx.Conn, migrations []migration, roundtrip bool) int {
	tx, err := conn.Begin(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "begin:", err)
		return 2
	}
	// Rolled back on every path, including panic.
	defer func() { _ = tx.Rollback(ctx) }()

	exec := func(m migration, dir, sql string) bool {
		if sql == "" {
			fmt.Printf("SKIP %d %-28s %s (file missing)\n", m.version, m.name, dir)
			return true
		}
		if _, err := tx.Exec(ctx, stripTx.ReplaceAllString(sql, "")); err != nil {
			fmt.Printf("FAIL %d %-28s %s\n       %v\n", m.version, m.name, dir, err)
			return false
		}
		fmt.Printf("ok   %d %-28s %s\n", m.version, m.name, dir)
		return true
	}

	fmt.Println("--- up ---")
	for _, m := range migrations {
		if !exec(m, "up", m.up) {
			return 1
		}
	}

	if roundtrip {
		fmt.Println("\n--- down (reverse) ---")
		for i := len(migrations) - 1; i >= 0; i-- {
			if !exec(migrations[i], "down", migrations[i].down) {
				return 1
			}
		}
	}

	if err := tx.Rollback(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "\nROLLBACK FAILED:", err)
		return 2
	}
	fmt.Println("\nrolled back — database unchanged")
	return 0
}

func load(dir string, from, to int) ([]migration, error) {
	ups, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		return nil, err
	}
	reName := regexp.MustCompile(`^(\d+)_(.+)\.up\.sql$`)

	var out []migration
	for _, path := range ups {
		m := reName.FindStringSubmatch(filepath.Base(path))
		if m == nil {
			continue
		}
		v, err := strconv.Atoi(m[1])
		if err != nil || v < from || v > to {
			continue
		}
		up, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		// A missing down is reported rather than fatal: not every migration
		// has one, and the up pass is still worth running.
		down, _ := os.ReadFile(filepath.Join(dir, fmt.Sprintf("%s_%s.down.sql", m[1], m[2])))
		out = append(out, migration{version: v, name: m[2], up: string(up), down: string(down)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}
