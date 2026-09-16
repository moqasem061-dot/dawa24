package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Editing a migration that has already run.
//
// The runner refuses it — an applied migration is immutable, and a database
// whose files no longer describe how it was built cannot be reproduced. But
// the refusal happens at deploy time, in a container whose only visible symptom
// is `service "migrate" didn't complete successfully: exit 1`, with the
// migrations themselves already applied and everything that runs after them
// (the permission catalogue sync, in particular) silently skipped.
//
// That is a slow, confusing way to learn something a query answers in a second,
// so migratecheck answers it first. It costs one round trip and runs before the
// dry-run, because there is no point rehearsing SQL that the runner will not
// reach.
//
// The fix is never to restore the hash by hand: put the change in a NEW
// migration and return the applied file to exactly what it was.

// driftReport is one migration whose file changed after it ran.
type driftReport struct {
	version  int
	name     string
	recorded string
	current  string
}

// checkDrift compares recorded checksums against the files on disk.
//
// It mirrors internal/platform/database.Migrate: same normalisation, same hash,
// so the two cannot disagree about whether a file changed.
func checkDrift(ctx context.Context, conn *pgx.Conn, migrations []migration) ([]driftReport, error) {
	var exists bool
	if err := conn.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables
		  WHERE table_schema='public' AND table_name='schema_migrations')`).Scan(&exists); err != nil {
		return nil, fmt.Errorf("look for schema_migrations: %w", err)
	}
	if !exists {
		// A database that has never been migrated cannot have drifted.
		return nil, nil
	}

	rows, err := conn.Query(ctx,
		`SELECT version, name, hash FROM public.schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()

	onDisk := make(map[int]migration, len(migrations))
	for _, m := range migrations {
		onDisk[m.version] = m
	}

	var out []driftReport
	for rows.Next() {
		var version int
		var name, recorded string
		if err := rows.Scan(&version, &name, &recorded); err != nil {
			return nil, err
		}
		m, ok := onDisk[version]
		if !ok || m.up == "" {
			// Missing file: a different problem, reported by `cli migrate-status`.
			continue
		}
		if current := migrationHash(m.up); current != recorded {
			out = append(out, driftReport{version, name, recorded, current})
		}
	}
	return out, rows.Err()
}

// migrationHash reproduces the runner's checksum.
//
// It mirrors internal/platform/database.loadMigrations exactly: strip a UTF-8
// byte-order mark, normalise CRLF to LF, then SHA-256.
//
// Line endings are normalised so a file checked out on Windows and the same
// file in a Linux container hash identically, and the BOM is stripped for the
// same reason: several migrations were saved by an editor that writes one, and
// hashing those three bytes made this tool report 184 and 187 as "edited after
// being applied" against a database the real runner is perfectly happy with —
// a false alarm that tells a developer to undo a file they never touched.
func migrationHash(sql string) string {
	sql = strings.TrimPrefix(sql, "\ufeff")
	sum := sha256.Sum256([]byte(strings.ReplaceAll(sql, "\r\n", "\n")))
	return hex.EncodeToString(sum[:])
}

// reportDrift prints the findings and says what to do about them.
func reportDrift(drift []driftReport) {
	fmt.Println("--- checksum drift ---")
	for _, d := range drift {
		fmt.Printf("EDITED %d %-28s recorded=%s current=%s\n",
			d.version, d.name, short(d.recorded), short(d.current))
	}
	fmt.Println()
	fmt.Println("These files changed after being applied to this database, so the")
	fmt.Println("runner will refuse to migrate at all — and everything it does after")
	fmt.Println("migrating, including the permission-catalogue sync, will not run.")
	fmt.Println()
	fmt.Println("Restore each file to exactly what was applied and put the change in a")
	fmt.Println("new migration. Never edit the recorded hash.")
}

func short(hash string) string {
	if len(hash) <= 12 {
		return hash
	}
	return hash[:12]
}
