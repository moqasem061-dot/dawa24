package integration_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	comparepg "github.com/moqasem061-dot/dawa24/internal/modules/compare/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

// Batch 3 (S13): an organization-less upload is visible to every tenant, so a
// manual match must be limited to its owner. Another user gets not found and the
// row is unchanged; the uploader can still change it.
// Not an isolation test under RLS: the owner check is a WHERE clause, so it runs
// the same for a superuser connection.
func TestManualMatchOnlyChangesOwnOrgLessRow(t *testing.T) {
	db := connectTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()
	ctx := context.Background()
	repo := comparepg.NewRepository(db)

	const stranger int64 = 88412
	var uploader, fileID, rowID int64
	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, _ = tx.Exec(txCtx, `DELETE FROM identity.users WHERE email = 'test-batch3-row-owner@example.com';`)
		if err := tx.QueryRow(txCtx, `
			INSERT INTO identity.users (email, password_hash, name, role, status)
			VALUES ('test-batch3-row-owner@example.com', 'x', '{"ar":"مرفع","en":"Uploader"}'::jsonb, 'user', 'active')
			RETURNING id;`).Scan(&uploader); err != nil {
			return err
		}
		if err := tx.QueryRow(txCtx, `
			INSERT INTO compare.files (
				organization_id, user_id, supplier_name, original_filename, storage_key, mime_type, status
			) VALUES (NULL, $1, 'مورد بلا منشأة', 'orgless.csv', 'key_orgless', 'text/csv', 'ready')
			RETURNING id;`, uploader).Scan(&fileID); err != nil {
			return err
		}
		return tx.QueryRow(txCtx, `
			INSERT INTO compare.file_rows (
				file_id, organization_id, row_number, raw_name, normalized_name, price, discount, price_after_discount,
				match_state, match_basis, match_granularity, match_revision,
				human_locked, ai_state, match_decision, baseline_decision, group_basis, match_method
			) VALUES (
				$1, NULL, 1, 'صنف بلا منشأة', 'صنف بلا منشأة', 10.00, 0, 10.00,
				'needs_review', 'none', 'pack', 1,
				false, 'off', '{"v":1,"state":"none"}'::jsonb, '{"v":1,"state":"none"}'::jsonb, 'canonical_pack', 'none'
			) RETURNING id;`, fileID).Scan(&rowID)
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(txCtx, `DELETE FROM compare.file_rows WHERE file_id = $1`, fileID)
			_, _ = tx.Exec(txCtx, `DELETE FROM compare.files WHERE id = $1`, fileID)
			_, _ = tx.Exec(txCtx, `DELETE FROM identity.users WHERE id = $1`, uploader)
			return nil
		})
	})

	err = repo.UpdateFileRowMatch(database.AsSystem(ctx), rowID, nil, compare.MatchMethodManual, 100, &compare.RowOwner{UserID: stranger})
	if apperr.KindOf(err) != apperr.KindNotFound {
		t.Fatalf("stranger's match write: err=%v, want not found", err)
	}
	if got := matchMethodOf(t, db, rowID); got == "manual" {
		t.Fatal("row changed by a user who does not own its file")
	}

	if err := repo.UpdateFileRowMatch(database.AsSystem(ctx), rowID, nil, compare.MatchMethodManual, 100, &compare.RowOwner{UserID: uploader}); err != nil {
		t.Fatalf("uploader's match write: %v", err)
	}
	if got := matchMethodOf(t, db, rowID); got != "manual" {
		t.Fatalf("match_method %q after the uploader's write, want manual", got)
	}
}

func matchMethodOf(t *testing.T, db *database.DB, rowID int64) string {
	t.Helper()
	var method string
	if err := db.InReadTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT match_method FROM compare.file_rows WHERE id = $1`, rowID).Scan(&method)
	}); err != nil {
		t.Fatalf("read match_method: %v", err)
	}
	return method
}
