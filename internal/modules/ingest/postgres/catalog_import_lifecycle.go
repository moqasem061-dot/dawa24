package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Fail records a run that stopped on an error.
func (r *Repository) Fail(ctx context.Context, id int64, message string) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE ingest.catalog_imports
			SET phase = 'failed', completed_at = now(), error_message = $2
			WHERE id = $1 AND phase IN ('processing','review','confirm','settings','mapping')`, id, message)
		if err != nil {
			return fmt.Errorf("ingest postgres: fail import: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.Conflict("import.not_processing",
				i18n.TDefault("w4_mod.w4str_224_224"))
		}
		return nil
	})
}

// Cancel discards an import without touching the catalogue. Cancelling a
// session that is no longer open reports the conflict rather than pretending
// the file was purged when it was not.
func (r *Repository) Cancel(ctx context.Context, id int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE ingest.catalog_imports
			SET phase = 'cancelled', completed_at = now(), source_file = ''::BYTEA
			WHERE id = $1 AND phase IN ('mapping','settings','review','confirm')`, id)
		if err != nil {
			return fmt.Errorf("ingest postgres: cancel import: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.Conflict("import.not_open", i18n.TDefault("w4_mod.w4str_225_225"))
		}
		return nil
	})
}

// List backs the history panel on the upload screen.
func (r *Repository) List(ctx context.Context, orgID int64, limit int) ([]*ingest.Session, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var out []*ingest.Session
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `SELECT `+importColumns+`
			FROM ingest.catalog_imports
			WHERE organization_id = $1
			ORDER BY created_at DESC
			LIMIT $2`, orgID, limit)
		if err != nil {
			return fmt.Errorf("ingest postgres: list imports: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var s ingest.Session
			if err := scanImport(rows, &s); err != nil {
				return err
			}
			out = append(out, &s)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Sweep collects abandoned imports and the files they hold.
func (r *Repository) Sweep(ctx context.Context) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// The bytes go first and unconditionally: an abandoned review holds a
		// copy of a vendor's whole catalogue, and that is the part worth
		// reclaiming even where the record itself is kept for the history panel.
		if _, err := tx.Exec(txCtx, `
			UPDATE ingest.catalog_imports
			SET source_file = ''::BYTEA
			WHERE expires_at < now() AND octet_length(source_file) > 0`); err != nil {
			return fmt.Errorf("ingest postgres: sweep import files: %w", err)
		}
		if _, err := tx.Exec(txCtx, `
			UPDATE ingest.catalog_imports
			SET phase = 'cancelled', completed_at = now()
			WHERE expires_at < now() AND phase IN ('mapping','settings','review','confirm')`); err != nil {
			return fmt.Errorf("ingest postgres: sweep imports: %w", err)
		}
		// A run wedged in processing past the stale threshold is dead with its
		// process; recording it failed is what lets the vendor re-run it.
		if _, err := tx.Exec(txCtx, `
			UPDATE ingest.catalog_imports
			SET phase = 'failed', completed_at = now(),
			    error_message = 'توقفت العملية قبل اكتمالها. يمكن بدء الاستيراد من جديد.'
			WHERE phase = 'processing' AND started_at < now() - INTERVAL '`+staleRunAfter+`'`); err != nil {
			return fmt.Errorf("ingest postgres: sweep stale processing imports: %w", err)
		}
		return nil
	})
}

// importDocs are the JSON columns of a session, encoded together so a failure
// to marshal one never leaves a half-written row.
type importDocs struct {
	source    []byte
	overrides []byte
	settings  []byte
	mapping   []byte
	aiStats   []byte
}

func encodeImport(s *ingest.Session) (importDocs, error) {
	var docs importDocs
	var err error
	if docs.source, err = json.Marshal(s.Source); err != nil {
		return docs, fmt.Errorf("ingest postgres: encode import source: %w", err)
	}
	if docs.overrides, err = json.Marshal(s.Overrides); err != nil {
		return docs, fmt.Errorf("ingest postgres: encode import overrides: %w", err)
	}
	if docs.settings, err = json.Marshal(s.Settings); err != nil {
		return docs, fmt.Errorf("ingest postgres: encode import settings: %w", err)
	}
	if s.Mapping == nil {
		docs.mapping = []byte(`{}`)
	} else if docs.mapping, err = json.Marshal(s.Mapping); err != nil {
		return docs, fmt.Errorf("ingest postgres: encode import mapping: %w", err)
	}
	if docs.aiStats, err = json.Marshal(s.AI); err != nil {
		return docs, fmt.Errorf("ingest postgres: encode import ai stats: %w", err)
	}
	return docs, nil
}

// scanner is the shape pgx.Row and pgx.Rows share.
type scanner interface {
	Scan(dest ...any) error
}

func scanImport(row scanner, s *ingest.Session) error {
	var phase string
	var source, overrides, settings, mapping, stats, findings, aiStats []byte
	err := row.Scan(
		&s.ID, &s.PublicID, &s.OrganizationID, &s.CreatedBy, &s.Filename,
		&s.FileSizeBytes, &phase, &source, &overrides, &settings, &mapping,
		&stats, &findings, &aiStats,
		&s.TotalRows, &s.InsertedRows, &s.UpdatedRows, &s.SkippedRows, &s.ErrorRows,
		&s.MatchedRows, &s.ReviewRows, &s.UnmatchedRows, &s.CreatedProducts,
		&s.ProgressPercent, &s.ProgressNote, &s.ErrorMessage,
		&s.StartedAt, &s.CompletedAt, &s.CreatedAt, &s.UpdatedAt, &s.ExpiresAt,
	)
	if err != nil {
		return err
	}
	s.Phase = ingest.Phase(phase)
	_ = json.Unmarshal(source, &s.Source)
	_ = json.Unmarshal(overrides, &s.Overrides)
	_ = json.Unmarshal(settings, &s.Settings)
	_ = json.Unmarshal(stats, &s.Stats)
	_ = json.Unmarshal(findings, &s.Findings)
	_ = json.Unmarshal(aiStats, &s.AI)
	if len(mapping) > 2 {
		var snap ingest.MappingSnapshot
		if json.Unmarshal(mapping, &snap) == nil {
			s.Mapping = &snap
		}
	}
	s.Settings = s.Settings.Normalize()
	return nil
}
