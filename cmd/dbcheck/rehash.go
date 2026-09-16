package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"

	dbfs "github.com/muhiya/dawa24-store/db"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// rehash brings recorded migration checksums in line with what the migration
// runner now computes.
//
// The runner hashes migrations with line endings normalised, so the checksum
// describes the SQL rather than the operating system the file was last written
// on. Migrations applied before that change recorded a raw-bytes hash, which on
// a Windows working tree is the CRLF value — and every Linux container then
// computed the LF value and refused to start with "modified after being
// applied", blocking deploys with no schema change of any kind.
//
// A recorded hash is corrected only when the file's content is provably the
// same modulo line endings. Anything genuinely edited is reported and left
// alone: that check is the whole reason the checksum exists.
func rehash(dsn string, apply bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Use the runner's own loader so the values compared here are exactly the
	// values it will compute at deploy time.
	migrations, err := database.LoadMigrations(dbfs.Migrations, "migrations")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load migrations: %v\n", err)
		os.Exit(1)
	}
	canonical := make(map[int]database.Migration, len(migrations))
	for _, m := range migrations {
		canonical[m.Version] = m
	}

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx, `SELECT version, name, hash FROM public.schema_migrations ORDER BY version`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read schema_migrations: %v\n", err)
		os.Exit(1)
	}

	type record struct {
		version int
		name    string
		hash    string
	}
	var applied []record
	for rows.Next() {
		var r record
		if err := rows.Scan(&r.version, &r.name, &r.hash); err != nil {
			break
		}
		applied = append(applied, r)
	}
	rows.Close()

	changed := 0
	for _, r := range applied {
		m, ok := canonical[r.version]
		if !ok {
			fmt.Printf("  %03d %-32s NO FILE ON DISK — a fresh database would never receive this\n",
				r.version, r.name)
			continue
		}
		if r.hash == m.Hash {
			continue
		}

		// Is the recorded value simply the CRLF encoding of the same SQL?
		crlf := sha256.Sum256(toCRLF([]byte(m.SQL)))
		raw := sha256.Sum256([]byte(m.SQL))
		if r.hash != hex.EncodeToString(crlf[:]) && r.hash != hex.EncodeToString(raw[:]) {
			fmt.Printf("  %03d %-32s CONTENT DIFFERS — not a line-ending change, refusing\n",
				r.version, r.name)
			continue
		}

		fmt.Printf("  %03d %-32s line endings only (%s -> %s)\n",
			r.version, r.name, r.hash[:12], m.Hash[:12])
		changed++

		if apply {
			if _, err := conn.Exec(ctx,
				`UPDATE public.schema_migrations SET hash = $2 WHERE version = $1`,
				r.version, m.Hash); err != nil {
				fmt.Fprintf(os.Stderr, "    update failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("    recorded hash updated")
		}
	}

	switch {
	case changed == 0:
		fmt.Println("all recorded hashes already match what the runner computes")
	case !apply:
		fmt.Printf("\n%d to correct; pass -rehash-apply to write them\n", changed)
	}
}

// toCRLF re-expands LF to CRLF, reproducing what a Windows working tree held.
func toCRLF(b []byte) []byte {
	out := make([]byte, 0, len(b)+len(b)/20)
	for i := 0; i < len(b); i++ {
		if b[i] == '\n' && (i == 0 || b[i-1] != '\r') {
			out = append(out, '\r', '\n')
			continue
		}
		out = append(out, b[i])
	}
	return out
}
