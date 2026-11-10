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
		rows = append(rows, []any{c.OrganizationID, c.APIKeyID, c.KeyPrefix, c.RequestID, c.Method, c.Route, c.Status, c.ErrorCode, c.DurationMS, c.BytesIn, c.IP, c.SensitiveRead, c.CreatedAt, c.Query, c.UserAgent, c.BytesOut, c.Ref, c.AuthReason, errorDetailText(c.ErrorDetail)})
	}
	return r.db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.CopyFrom(ctx, pgx.Identifier{"erp", "api_calls"}, []string{"organization_id", "api_key_id", "key_prefix", "request_id", "method", "route", "status", "error_code", "duration_ms", "bytes_in", "ip", "sensitive_read", "created_at", "query", "user_agent", "bytes_out", "ref", "auth_reason", "error_detail"}, pgx.CopyFromRows(rows))
		return err
	})
}

// errorDetailText is the JSONB text of a call's error details, or nil (SQL NULL)
// when the call had none.
func errorDetailText(raw []byte) *string {
	if len(raw) == 0 {
		return nil
	}
	text := string(raw)
	return &text
}
