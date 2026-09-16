package main

// Exporting the regression corpus.
//
// A matcher cannot be refactored against opinion. Every change to scoring,
// column detection or retrieval has to be judged against the same real files
// that produced the outcomes people are complaining about, and those files are
// already in the database: the three import systems each retain the bytes they
// were given.
//
// This command copies them out to disk once, so the test suite reads files
// rather than a production database, and so a corpus survives the retention
// window that would otherwise reap it.

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// corpusDir is where the exported files and their manifest land.
const corpusDir = "test/corpus"

// corpusEntry is one exported file and where it came from.
type corpusEntry struct {
	// System names the importer that received the file, which decides which
	// profile the harness runs it under.
	System string `json:"system"`
	// Source is the table and primary key the bytes were read from, so an
	// entry can always be traced back.
	Source string `json:"source"`
	File   string `json:"file"`
	Bytes  int    `json:"bytes"`
	// Rows is what the importer recorded at the time, kept as a sanity check
	// rather than as an expectation: it is the old engine's answer.
	Rows int `json:"rows,omitempty"`
	// Duplicates names the other imports that uploaded byte-identical content.
	// Recorded rather than exported, so the corpus stays small without losing
	// the fact that this file is what suppliers actually re-send.
	Duplicates []string `json:"duplicates,omitempty"`
}

// exportCorpus writes every retained import file to test/corpus.
func exportCorpus(ctx context.Context, db *database.DB) error {
	if err := os.MkdirAll(filepath.Join(corpusDir, "files"), 0o755); err != nil {
		return err
	}

	var entries []corpusEntry
	// Re-uploads dominate the retained bytes: one 2.7 MB pharmacy file appears
	// three times and one vendor workbook five. Keeping every copy would triple
	// the corpus and triple every run against it while testing nothing extra,
	// so identical content is exported once and the duplicates are recorded on
	// the entry that kept it.
	seen := make(map[string]int)
	err := db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		for _, src := range corpusSources() {
			if err := exportOne(txCtx, tx, src, seen, &entries); err != nil {
				return fmt.Errorf("%s: %w", src.system, err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	manifest, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(corpusDir, "manifest.json")
	if err := os.WriteFile(path, append(manifest, '\n'), 0o644); err != nil {
		return err
	}

	fmt.Printf("exported %d files to %s\n", len(entries), corpusDir)
	for _, e := range entries {
		fmt.Printf("  %-12s %-40s %8d bytes\n", e.System, e.File, e.Bytes)
	}
	return exportCatalogue(ctx, db)
}

// corpusSource is one table holding retained import files.
type corpusSource struct {
	system string
	query  string
}

// corpusSources lists where each importer keeps the bytes it was given.
//
// The savings importer is absent on purpose: it holds staged rows in process
// memory and retains nothing, which is itself a finding and is fixed when that
// importer gets a durable session.
func corpusSources() []corpusSource {
	return []corpusSource{
		{
			system: "admin",
			query: `SELECT id, filename, source_file, total_rows
			        FROM catalog.import_sessions
			        WHERE octet_length(source_file) > 0
			        ORDER BY id;`,
		},
		{
			system: "vendor",
			query: `SELECT id, filename, source_file, total_rows
			        FROM ingest.catalog_imports
			        WHERE octet_length(source_file) > 0
			        ORDER BY id;`,
		},
		{
			system: "smartorder",
			query: `SELECT run_id, filename, content, 0
			        FROM smartorder.run_files
			        WHERE octet_length(content) > 0
			        ORDER BY run_id;`,
		},
	}
}

// exportOne writes every distinct file one source holds, appending to entries.
func exportOne(
	ctx context.Context, tx pgx.Tx, src corpusSource,
	seen map[string]int, entries *[]corpusEntry,
) error {
	rows, err := tx.Query(ctx, src.query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id       int64
			filename string
			content  []byte
			total    int
		)
		if err := rows.Scan(&id, &filename, &content, &total); err != nil {
			return err
		}
		if len(content) == 0 {
			continue
		}

		origin := fmt.Sprintf("%s:%d", src.system, id)
		digest := fmt.Sprintf("%x", sha256.Sum256(content))
		if at, dup := seen[digest]; dup {
			(*entries)[at].Duplicates = append((*entries)[at].Duplicates, origin)
			continue
		}

		name := fmt.Sprintf("%s-%d%s", src.system, id, corpusExt(filename))
		if err := os.WriteFile(filepath.Join(corpusDir, "files", name), content, 0o644); err != nil {
			return err
		}
		seen[digest] = len(*entries)
		*entries = append(*entries, corpusEntry{
			System: src.system,
			Source: origin,
			File:   name,
			Bytes:  len(content),
			Rows:   total,
		})
	}
	return rows.Err()
}

// corpusExt keeps the original extension, because the reader dispatches on it.
// A file whose name carried none is assumed to be a workbook, which is what
// every observed upload without an extension turned out to be.
func corpusExt(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".xlsx", ".xls", ".csv", ".tsv", ".txt":
		return ext
	default:
		return ".xlsx"
	}
}
