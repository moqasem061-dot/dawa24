package postgres

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

var validIdentifier = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// assertSoftDeletable re-validates the identifier against information_schema.
// The schema and table arrive from a URL segment, so they are never trusted on
// shape alone — if the pair is not a real soft-deletable table, nothing runs.
func assertSoftDeletable(ctx context.Context, tx pgx.Tx, schema, table string) error {
	if !validIdentifier.MatchString(schema) || !validIdentifier.MatchString(table) {
		return apperr.Validation("invalid_identifier", "schema or table name format is invalid", nil)
	}
	ok, err := columnExists(ctx, tx, schema, table, "deleted_at")
	if err != nil {
		return err
	}
	if !ok {
		return apperr.NotFound("trash_model")
	}
	hasID, err := columnExists(ctx, tx, schema, table, "id")
	if err != nil {
		return err
	}
	if !hasID {
		return apperr.NotFound("trash_model")
	}
	return nil
}

func columnExists(ctx context.Context, tx pgx.Tx, schema, table, column string) (bool, error) {
	var exists bool
	const q = `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = $1 AND table_name = $2 AND column_name = $3
		)`
	err := tx.QueryRow(ctx, q, schema, table, column).Scan(&exists)
	return exists, err
}

// trashLabelExpr picks the first present display column so one generic query
// can label rows from tables with very different shapes.
func trashLabelExpr(ctx context.Context, tx pgx.Tx, schema, table string) (string, error) {
	for _, col := range []string{"trade_name", "name", "legal_name", "title", "subject", "email"} {
		ok, err := columnExists(ctx, tx, schema, table, col)
		if err != nil {
			return "", err
		}
		if ok {
			// JSONB bilingual columns need the Arabic key pulled out.
			return fmt.Sprintf(
				`COALESCE(CASE WHEN jsonb_typeof(to_jsonb(t.%q)) = 'object' THEN to_jsonb(t.%q)->>'ar' ELSE t.%q::text END, '')`,
				col, col, col), nil
		}
	}
	return `''`, nil
}

// trashCodeExpr picks the SKU / code / order number column to identify the record.
func trashCodeExpr(ctx context.Context, tx pgx.Tx, schema, table string) (string, error) {
	for _, col := range []string{"sku", "code", "order_number", "invoice_number", "organization_number", "public_id", "slug"} {
		ok, err := columnExists(ctx, tx, schema, table, col)
		if err != nil {
			return "", err
		}
		if ok {
			return fmt.Sprintf(`COALESCE(t.%q::text, '')`, col), nil
		}
	}
	return `''`, nil
}

// writeTrashAudit records the action in the append-only audit trail. For a
// purge, `before` carries the row that was destroyed — that snapshot is the
// only remaining trace of it.
func writeTrashAudit(ctx context.Context, tx pgx.Tx, action, schema, table string, id, actorID int64, snapshot string) error {
	const q = `
		INSERT INTO platform.audit_log (actor_user_id, action, entity_type, entity_id, before, created_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, now())`
	var before *string
	if snapshot != "" {
		before = &snapshot
	}
	_, err := tx.Exec(ctx, q, actorID, "trash."+action, schema+"."+table, strconv.FormatInt(id, 10), before)
	return err
}
