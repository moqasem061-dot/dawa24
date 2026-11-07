package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// InsertCallsAsSystem permits failed-authentication records with no tenant (plan.md E4).
func (r *Repository) InsertCallsAsSystem(ctx context.Context, calls []erp.CallRecord) error {
	rows := make([][]any, 0, len(calls))
	for _, c := range calls {
		rows = append(rows, []any{c.OrganizationID, c.APIKeyID, c.KeyPrefix, c.RequestID, c.Method, c.Route, c.Status, c.ErrorCode, c.DurationMS, c.BytesIn, c.IP, c.SensitiveRead, c.CreatedAt})
	}
	return r.db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.CopyFrom(ctx, pgx.Identifier{"erp", "api_calls"}, []string{"organization_id", "api_key_id", "key_prefix", "request_id", "method", "route", "status", "error_code", "duration_ms", "bytes_in", "ip", "sensitive_read", "created_at"}, pgx.CopyFromRows(rows))
		return err
	})
}
