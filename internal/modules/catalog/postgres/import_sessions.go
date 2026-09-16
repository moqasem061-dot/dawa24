package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

const sessionColumns = `
	s.id, s.public_id, s.organization_id, s.created_by,
	s.filename, s.file_size_bytes, s.source_format, s.sheet_name, s.delimiter,
	s.status, s.import_mode, s.options, s.layout_overrides,
	s.total_rows, s.parsed_rows, s.insert_rows, s.update_rows, s.skip_rows,
	s.error_rows, s.warning_rows, s.block_count, s.new_brands, s.new_categories,
	s.ai_calls, s.ai_applied, s.ai_matched, s.ai_note, s.ai_fallback,
	s.error_message, s.created_at, s.updated_at, s.committed_at, s.expires_at,
	s.structure, s.progress_phase, s.progress_current, s.progress_total, s.progress_at`

// CreateImportSession opens a review session and collects abandoned ones.
//
// The uploaded bytes are kept on the row so the admin can correct the column
// mapping and re-run the same file without uploading it again.
func (r *Repository) CreateImportSession(ctx context.Context, s *catalog.ImportSession, sourceFile []byte) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// An abandoned review holds a copy of the admin's entire file. Reaping
		// on open keeps the staging tables bounded without a scheduled job,
		// and the work is proportional to what actually expired.
		if _, err := tx.Exec(txCtx, `
			DELETE FROM catalog.import_sessions
			WHERE status IN ('draft','ready') AND expires_at < now()
		`); err != nil {
			return fmt.Errorf("catalog postgres: reap expired import sessions: %w", err)
		}

		// A finished session keeps its counts as history but has no further use
		// for its rows. Without this they are never collected — the expiry sweep
		// above only looks at sessions still under review — and a few hundred
		// imports leave millions of staging rows behind.
		if _, err := tx.Exec(txCtx, `
			DELETE FROM catalog.import_staging_rows r
			USING catalog.import_sessions s
			WHERE r.session_id = s.id
			  AND s.status IN ('committed','cancelled','failed')
		`); err != nil {
			return fmt.Errorf("catalog postgres: reap finished staging rows: %w", err)
		}

		// A preparation run that never came back was interrupted the same way,
		// and leaves a worse mess: 'processing' is not reviewable, so the admin
		// gets a screen that polls for ever and can be neither cancelled nor
		// corrected. The run itself is bounded at thirty minutes, so an hour
		// without a heartbeat means no goroutine is coming back for it.
		if _, err := tx.Exec(txCtx, `
			UPDATE catalog.import_sessions
			SET status = 'failed',
			    progress_phase = 'failed',
			    error_message = 'توقفت معالجة الملف قبل اكتمالها. يمكنك تصحيح ربط الأعمدة وإعادة المعالجة.'
			WHERE status = 'processing' AND updated_at < now() - INTERVAL '1 hour'
		`); err != nil {
			return fmt.Errorf("catalog postgres: reap stale processing sessions: %w", err)
		}

		// A session claimed by a commit that never came back was interrupted —
		// a crash, a deploy, a lost connection. Its claim must not stand
		// forever: the admin would see a review screen that can neither be
		// committed nor cancelled. Two hours is several times the longest sane
		// commit, so anything older than that failed.
		if _, err := tx.Exec(txCtx, `
			UPDATE catalog.import_sessions
			SET status = 'failed',
			    error_message = 'توقفت عملية الحفظ قبل اكتمالها. يرجى بدء العملية من جديد.'
			WHERE status = 'committing' AND updated_at < now() - INTERVAL '2 hours'
		`); err != nil {
			return fmt.Errorf("catalog postgres: reap stale committing sessions: %w", err)
		}

		options, err := json.Marshal(s.Options)
		if err != nil {
			return fmt.Errorf("catalog postgres: encode import options: %w", err)
		}
		overrides, err := json.Marshal(s.Overrides)
		if err != nil {
			return fmt.Errorf("catalog postgres: encode layout overrides: %w", err)
		}

		structure, err := json.Marshal(s.Structure)
		if err != nil {
			return fmt.Errorf("catalog postgres: encode import structure: %w", err)
		}

		// The analysis counts are written here rather than left for the
		// preparation pass. Omitting them is why a session sitting on the
		// mapping step reported nought rows and nought blocks for a file of
		// nine thousand: the screen was reading a row nobody had filled in.
		return tx.QueryRow(txCtx, `
			INSERT INTO catalog.import_sessions (
				organization_id, created_by, filename, file_size_bytes,
				source_format, sheet_name, delimiter, status, import_mode,
				options, layout_overrides, source_file,
				total_rows, block_count, structure
			) VALUES (
				$1,
				-- Attribution must never be the reason an import fails. A user id
				-- that no longer resolves — a deleted account, a stale session —
				-- records as NULL, which is what the column's ON DELETE SET NULL
				-- would have done anyway.
				(SELECT u.id FROM identity.users u WHERE u.id = $2),
				$3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11::jsonb, $12,
				$13, $14, $15::jsonb)
			RETURNING id, public_id, created_at, updated_at, expires_at
		`,
			s.OrganizationID, s.CreatedBy, s.Filename, s.FileSizeBytes,
			s.SourceFormat, s.SheetName, s.Delimiter, string(s.Status), string(s.Mode),
			string(options), string(overrides), sourceFile,
			s.TotalRows, s.BlockCount, string(structure),
		).Scan(&s.ID, &s.PublicID, &s.CreatedAt, &s.UpdatedAt, &s.ExpiresAt)
	})
}

// GetImportSession loads a session by its public id.
func (r *Repository) GetImportSession(ctx context.Context, publicID string) (*catalog.ImportSession, error) {
	var out *catalog.ImportSession
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(txCtx,
			`SELECT `+sessionColumns+` FROM catalog.import_sessions s WHERE s.public_id = $1`, publicID)
		s, err := scanImportSession(row)
		if err != nil {
			return err
		}
		out = s
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ClearStagingRows drops a finished session's rows.
//
// Called the moment a session is committed or cancelled: the rows have done
// their job, the session row keeps the counts that history needs, and holding
// nine thousand of them per import indefinitely is how the staging table grows
// without bound.
func (r *Repository) ClearStagingRows(ctx context.Context, sessionID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx,
			`DELETE FROM catalog.import_staging_rows WHERE session_id = $1`, sessionID); err != nil {
			return fmt.Errorf("catalog postgres: clear finished staging rows: %w", err)
		}
		return nil
	})
}

// ImportSourceFile reads back the bytes a session was opened with.
func (r *Repository) ImportSourceFile(ctx context.Context, sessionID int64) ([]byte, error) {
	var content []byte
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(txCtx,
			`SELECT source_file FROM catalog.import_sessions WHERE id = $1`, sessionID).Scan(&content)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound("import_session")
		}
		if err != nil {
			return fmt.Errorf("catalog postgres: read import source file: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return content, nil
}

// ReleaseImportSourceFile drops the stored bytes once a session is finished
// with, so a committed import does not keep a copy of the file indefinitely.
func (r *Repository) ReleaseImportSourceFile(ctx context.Context, sessionID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx,
			`UPDATE catalog.import_sessions SET source_file = ''::bytea WHERE id = $1`, sessionID)
		if err != nil {
			return fmt.Errorf("catalog postgres: release import source file: %w", err)
		}
		return nil
	})
}

// UpdateImportSession writes back the counts, status and AI account.
//
// The variadic fromStatuses turns the write into a guarded transition: given
// any, the UPDATE carries `AND status = ANY(...)`, so a background prepare
// cannot resurrect a session an admin cancelled while it ran — the previous
// unconditional overwrite did exactly that. With none, the write is a plain
// save of the row the caller already owns.
func (r *Repository) UpdateImportSession(
	ctx context.Context, s *catalog.ImportSession, fromStatuses ...catalog.SessionStatus,
) error {
	options, err := json.Marshal(s.Options)
	if err != nil {
		return fmt.Errorf("catalog postgres: encode import options: %w", err)
	}
	overrides, err := json.Marshal(s.Overrides)
	if err != nil {
		return fmt.Errorf("catalog postgres: encode layout overrides: %w", err)
	}
	brands, err := json.Marshal(nonNilStrings(s.NewBrands))
	if err != nil {
		return fmt.Errorf("catalog postgres: encode new brands: %w", err)
	}
	categories, err := json.Marshal(nonNilStrings(s.NewCategories))
	if err != nil {
		return fmt.Errorf("catalog postgres: encode new categories: %w", err)
	}
	structure, err := json.Marshal(s.Structure)
	if err != nil {
		return fmt.Errorf("catalog postgres: encode import structure: %w", err)
	}

	query := `
		UPDATE catalog.import_sessions SET
			status = $2, import_mode = $3, options = $4::jsonb, layout_overrides = $5::jsonb,
			sheet_name = $6, source_format = $7, delimiter = $8,
			total_rows = $9, parsed_rows = $10, insert_rows = $11, update_rows = $12,
			skip_rows = $13, error_rows = $14, warning_rows = $15, block_count = $16,
			new_brands = $17::jsonb, new_categories = $18::jsonb,
			ai_calls = $19, ai_applied = $20, ai_matched = $21, ai_note = $22, ai_fallback = $23,
			error_message = $24, committed_at = $25, structure = $26::jsonb,
			progress_phase = $27, updated_at = now()
		WHERE id = $1`
	args := []any{
		s.ID, string(s.Status), string(s.Mode), string(options), string(overrides),
		s.SheetName, s.SourceFormat, s.Delimiter,
		s.TotalRows, s.ParsedRows, s.InsertRows, s.UpdateRows,
		s.SkipRows, s.ErrorRows, s.WarningRows, s.BlockCount,
		string(brands), string(categories),
		s.AICalls, s.AIApplied, s.AIMatched, s.AINote, s.AIFallback,
		s.ErrorMessage, s.CommittedAt, string(structure), string(s.Progress.Phase),
	}

	if len(fromStatuses) > 0 {
		statuses := make([]string, len(fromStatuses))
		for i, st := range fromStatuses {
			statuses[i] = string(st)
		}
		query += ` AND status = ANY($28::text[])`
		args = append(args, statuses)
	}

	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, query, args...)
		if err != nil {
			return fmt.Errorf("catalog postgres: update import session: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.Conflict("catalog.import_state_changed",
				i18n.TDefault("w4_mod.w4str_122_122"))
		}
		return nil
	})
}

// SaveImportProgress records a background run's phase on the session row.
//
// It writes only the progress columns and only while the session is still the
// running one. The alternative — reusing the full save — would have the
// background goroutine overwrite counts, options and status it does not own,
// which is a race that only ever shows up on a big file.
func (r *Repository) SaveImportProgress(
	ctx context.Context, publicID string, p catalog.ImportProgress,
) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			UPDATE catalog.import_sessions
			SET progress_phase = $2, progress_current = $3, progress_total = $4,
			    progress_at = now()
			WHERE public_id = $1 AND status = 'processing'
		`, publicID, string(p.Phase), p.Current, p.Total)
		if err != nil {
			return fmt.Errorf("catalog postgres: save import progress: %w", err)
		}
		return nil
	})
}

// ClaimImportSessionForCommit atomically takes ownership of a reviewable
// session for committing, flipping it to 'committing' in the same statement
// that reads it.
//
// The claim is what makes concurrent commits safe across processes: two
// requests racing on one session cannot both pass, whatever in-process guards
// say, because only one of them finds a reviewable row to flip. A claimed
// session is not reviewable, so cancel and re-prepare are refused until the
// commit finishes or the reaper records it failed.
func (r *Repository) ClaimImportSessionForCommit(ctx context.Context, publicID string) (*catalog.ImportSession, error) {
	var out *catalog.ImportSession
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// The alias is not decoration: sessionColumns qualifies every column
		// with "s.", and an UPDATE's RETURNING can only see the target table
		// under its own name unless one is given. Without it every commit
		// failed with `missing FROM-clause entry for table "s"` — after the
		// admin had reviewed the whole file and pressed save.
		row := tx.QueryRow(txCtx, `
			UPDATE catalog.import_sessions AS s
			SET status = 'committing', updated_at = now()
			WHERE s.public_id = $1 AND s.status = 'ready'
			RETURNING `+sessionColumns, publicID)
		s, err := scanImportSession(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Conflict("catalog.import_not_committable",
				i18n.TDefault("w4_mod.w4str_123_123"))
		}
		if err != nil {
			return err
		}
		out = s
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
