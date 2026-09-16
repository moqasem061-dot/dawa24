package database

import (
	"context"
	"fmt"
	"sort"
)

// OrphanMigration is a migration the database records as applied but which no
// longer exists as a file.
type OrphanMigration struct {
	Version int
	Name    string
}

// Orphans reports migrations that ran against this database and were then
// deleted from the repository.
//
// The checksum in Migrate catches the opposite mistake — editing a migration
// after it ran — but it iterates over files, so a migration that exists only in
// the ledger is invisible to it. That is not hypothetical here: version 48
// (seed_realistic_data) was applied to production on 17 August and its file was
// later removed, which means a fresh environment cannot reproduce production
// from the migrations alone and nothing said so.
//
// This is reported rather than fatal. Refusing to boot would strand a running
// deployment over a file that is already gone, and the operator's fix — restore
// the file, or accept the divergence deliberately — is not something the
// process can perform for them.
func (db *DB) Orphans(ctx context.Context, migrations []Migration) ([]OrphanMigration, error) {
	pool, err := db.getPool()
	if err != nil {
		return nil, err
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrate: acquire connection: %w", err)
	}
	defer conn.Release()

	var exists bool
	if err := conn.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables
		  WHERE table_schema='public' AND table_name='schema_migrations')`).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}

	rows, err := conn.Query(ctx,
		`SELECT version, name FROM public.schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("migrate: read schema_migrations: %w", err)
	}
	defer rows.Close()

	known := make(map[int]struct{}, len(migrations))
	for _, m := range migrations {
		known[m.Version] = struct{}{}
	}

	var out []OrphanMigration
	for rows.Next() {
		var o OrphanMigration
		if err := rows.Scan(&o.Version, &o.Name); err != nil {
			return nil, err
		}
		if _, ok := known[o.Version]; !ok {
			out = append(out, o)
		}
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}
