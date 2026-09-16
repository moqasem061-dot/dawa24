package database

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Migration is one versioned schema change.
type Migration struct {
	Version int
	Name    string
	SQL     string
	Hash    string
}

// LoadMigrations reads and orders the embedded .up.sql files.
//
// Filenames must be NNN_name.up.sql. The numeric prefix is the version and
// determines order; the name is documentation.
func LoadMigrations(fsys fs.FS, dir string) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("migrate: read %s: %w", dir, err)
	}

	var out []Migration
	seen := map[int]string{}

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".up.sql") {
			continue
		}

		prefix, rest, ok := strings.Cut(strings.TrimSuffix(name, ".up.sql"), "_")
		if !ok {
			return nil, fmt.Errorf("migrate: %s does not match NNN_name.up.sql", name)
		}
		version, err := strconv.Atoi(prefix)
		if err != nil {
			return nil, fmt.Errorf("migrate: %s has a non-numeric version prefix", name)
		}
		if dup, exists := seen[version]; exists {
			// Two migrations claiming one version is how a schema silently
			// diverges between environments depending on filesystem ordering.
			return nil, fmt.Errorf("migrate: version %d claimed by both %s and %s", version, dup, name)
		}
		seen[version] = name

		body, err := fs.ReadFile(fsys, dir+"/"+name)
		if err != nil {
			return nil, fmt.Errorf("migrate: read %s: %w", name, err)
		}
		body = bytes.TrimPrefix(body, []byte("\xef\xbb\xbf"))

		// Hash the content with line endings normalised.
		//
		// The checksum exists to detect an edited migration. A file is edited
		// when its statements change, not when it crosses an operating system:
		// a developer's Windows working tree holds CRLF while git and every
		// Linux container hold LF, so hashing the bytes verbatim makes the
		// same migration produce two different hashes depending on where it is
		// read. That is exactly what happened — a migration applied from a
		// Windows machine then blocked every container deploy with "modified
		// after being applied", and the deploy failed with no schema change of
		// any kind.
		//
		// Normalising here means the hash describes the SQL, which is the
		// thing the check is actually about.
		sum := sha256.Sum256(normaliseLineEndings(body))
		out = append(out, Migration{
			Version: version,
			Name:    rest,
			SQL:     string(body),
			Hash:    hex.EncodeToString(sum[:]),
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

const migrationTableDDL = `
CREATE TABLE IF NOT EXISTS public.schema_migrations (
    version     INTEGER PRIMARY KEY,
    name        TEXT NOT NULL,
    hash        TEXT NOT NULL,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    duration_ms BIGINT NOT NULL
)`

// Migrate applies every pending migration in order.
//
// Each migration runs in its own transaction, together with the row that records
// it. A migration and the record of that migration therefore commit or roll back
// as a unit — there is no window in which the schema has changed but the ledger
// disagrees.
//
// Applied migrations are checksummed. Editing a migration that has already run
// is refused rather than ignored: environments would otherwise drift apart with
// no signal, which is precisely the state the legacy project reached with 153
// archived migrations and a schema nobody could rebuild.
func (db *DB) Migrate(ctx context.Context, migrations []Migration, log func(string, ...any)) error {
	pool, err := db.getPool()
	if err != nil {
		return err
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("migrate: acquire connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, migrationTableDDL); err != nil {
		return fmt.Errorf("migrate: create schema_migrations: %w", err)
	}

	// An advisory lock serialises concurrent deploys. Two instances starting
	// simultaneously would otherwise both try to create the same table.
	const lockID = 0x6461776132340001
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", lockID); err != nil {
		return fmt.Errorf("migrate: acquire advisory lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", lockID)
	}()

	applied, err := loadApplied(ctx, conn.Conn())
	if err != nil {
		return err
	}

	// A migration recorded here but missing from disk means this database
	// cannot be rebuilt from the repository. Reported, not fatal: see Orphans.
	known := make(map[int]struct{}, len(migrations))
	for _, m := range migrations {
		known[m.Version] = struct{}{}
	}
	for version := range applied {
		if _, ok := known[version]; !ok {
			log("migration applied but its file is missing; this database cannot be "+
				"rebuilt from the repository", "version", version)
		}
	}

	for _, m := range migrations {
		if prev, ok := applied[m.Version]; ok {
			if prev != m.Hash {
				return fmt.Errorf(
					"migrate: migration %d (%s) was modified after being applied "+
						"(recorded %s, current %s); add a new migration instead of editing this one",
					m.Version, m.Name, short(prev), short(m.Hash))
			}
			continue
		}

		log("applying migration", "version", m.Version, "name", m.Name)
		start := time.Now()

		if err := applyOne(ctx, conn.Conn(), m, start); err != nil {
			return err
		}
		log("applied migration", "version", m.Version, "name", m.Name,
			"duration_ms", time.Since(start).Milliseconds())
	}

	return nil
}

func applyOne(ctx context.Context, conn *pgx.Conn, m Migration, start time.Time) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("migrate: begin %d: %w", m.Version, err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	if _, err := tx.Exec(ctx, m.SQL); err != nil {
		return fmt.Errorf("migrate: apply %d (%s): %w", m.Version, m.Name, err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO public.schema_migrations (version, name, hash, duration_ms)
		 VALUES ($1, $2, $3, $4)`,
		m.Version, m.Name, m.Hash, time.Since(start).Milliseconds(),
	); err != nil {
		return fmt.Errorf("migrate: record %d: %w", m.Version, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("migrate: commit %d: %w", m.Version, err)
	}
	return nil
}

func loadApplied(ctx context.Context, conn *pgx.Conn) (map[int]string, error) {
	rows, err := conn.Query(ctx, "SELECT version, hash FROM public.schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("migrate: read schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := map[int]string{}
	for rows.Next() {
		var version int
		var hash string
		if err := rows.Scan(&version, &hash); err != nil {
			return nil, fmt.Errorf("migrate: scan schema_migrations: %w", err)
		}
		applied[version] = hash
	}
	return applied, rows.Err()
}

// PendingCount reports how many migrations have not been applied, for the
// readiness probe. A pod serving traffic against a stale schema is worse than a
// pod that reports itself unready.
func (db *DB) PendingCount(ctx context.Context, migrations []Migration) (int, error) {
	pool, err := db.getPool()
	if err != nil {
		return 0, err
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Release()

	var exists bool
	if err := conn.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables
		  WHERE table_schema='public' AND table_name='schema_migrations')`).Scan(&exists); err != nil {
		return 0, err
	}
	if !exists {
		return len(migrations), nil
	}

	applied, err := loadApplied(ctx, conn.Conn())
	if err != nil {
		return 0, err
	}

	pending := 0
	for _, m := range migrations {
		if _, ok := applied[m.Version]; !ok {
			pending++
		}
	}
	return pending, nil
}

func short(hash string) string {
	if len(hash) <= 12 {
		return hash
	}
	return hash[:12]
}

// normaliseLineEndings converts CRLF to LF so a migration's checksum describes
// its SQL rather than the operating system it was last written on.
func normaliseLineEndings(b []byte) []byte {
	if !bytes.Contains(b, []byte("\r\n")) {
		return b
	}
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
}
