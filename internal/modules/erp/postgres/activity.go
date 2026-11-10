package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// The activity screen's reads (plan section 3b). Each runs in InReadTx with the
// organization's tenant GUC and filters by organization_id = $1 as well, so a
// foreign organization id reads nothing even if the policy were missing.

// ActivityCounts reads the summary in one read transaction.
func (r *Repository) ActivityCounts(ctx context.Context, orgID int64) (erp.ActivityCounts, error) {
	var out erp.ActivityCounts
	err := r.db.InReadTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT COALESCE((SELECT id FROM erp.integrations
				WHERE organization_id = $1 AND status <> 'closed' ORDER BY id DESC LIMIT 1), 0),
			COALESCE((SELECT status FROM erp.integrations
				WHERE organization_id = $1 AND status <> 'closed' ORDER BY id DESC LIMIT 1), '')`, orgID).
			Scan(&out.IntegrationID, &out.IntegrationStatus)
		if err != nil {
			return fmt.Errorf("erp: activity integration: %w", err)
		}
		if err := tx.QueryRow(ctx, `SELECT count(*)::int FROM erp.api_keys
			WHERE organization_id = $1 AND status = 'active' AND (expires_at IS NULL OR expires_at > now())`, orgID).
			Scan(&out.ActiveKeys); err != nil {
			return fmt.Errorf("erp: activity keys: %w", err)
		}
		var last time.Time
		err = tx.QueryRow(ctx, `SELECT created_at, status, route FROM erp.api_calls
			WHERE organization_id = $1 ORDER BY id DESC LIMIT 1`, orgID).Scan(&last, &out.LastCallStatus, &out.LastCallRoute)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return fmt.Errorf("erp: activity last call: %w", err)
		default:
			out.LastCallAt = &last
		}
		if err := tx.QueryRow(ctx, `SELECT count(*)::int,
				count(*) FILTER (WHERE status >= 400)::int,
				COALESCE(avg(duration_ms), 0)::int
			FROM erp.api_calls WHERE organization_id = $1 AND created_at > now() - interval '24 hours'`, orgID).
			Scan(&out.Calls24h, &out.Errors24h, &out.AvgDurationMS); err != nil {
			return fmt.Errorf("erp: activity calls: %w", err)
		}
		if err := tx.QueryRow(ctx, `SELECT
				count(*) FILTER (WHERE state = 'queued')::int,
				count(*) FILTER (WHERE state = 'processing')::int,
				count(*) FILTER (WHERE state = 'failed' AND accepted_at > now() - interval '24 hours')::int
			FROM erp.sync_batches WHERE organization_id = $1`, orgID).
			Scan(&out.BatchesQueued, &out.BatchesProcessing, &out.BatchesFailed24h); err != nil {
			return fmt.Errorf("erp: activity batches: %w", err)
		}
		if err := tx.QueryRow(ctx, `SELECT count(*)::int FROM erp.review_items
			WHERE organization_id = $1 AND state = 'open'`, orgID).Scan(&out.OpenReview); err != nil {
			return fmt.Errorf("erp: activity review: %w", err)
		}
		return nil
	})
	return out, err
}

// ListCalls pages the call log by id, newest first. The key name comes from a
// left join, so a call whose key was deleted still shows.
func (r *Repository) ListCalls(ctx context.Context, orgID int64, f erp.CallFilter) ([]erp.CallRow, error) {
	var out []erp.CallRow
	err := r.db.InReadTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT c.id, c.created_at, COALESCE(k.name, ''), c.key_prefix,
				c.method, c.route, c.status, c.error_code, c.duration_ms, c.request_id,
				c.query, c.user_agent, COALESCE(host(c.ip), ''), c.bytes_in, c.bytes_out,
				c.ref, c.auth_reason, COALESCE(c.error_detail::text, '')
			FROM erp.api_calls c
			LEFT JOIN erp.api_keys k ON k.id = c.api_key_id AND k.organization_id = $1
			WHERE c.organization_id = $1
			  AND ($2::bigint = 0 OR c.id < $2)
			  AND ($3 = 'all' OR ($3 = 'ok' AND c.status < 400) OR ($3 = 'error' AND c.status >= 400))
			  AND ($4 = '' OR c.key_prefix = $4)
			  AND ($5 = '' OR c.route = $5)
			  AND ($6::timestamptz IS NULL OR c.created_at >= $6)
			  AND ($7::timestamptz IS NULL OR c.created_at < $7)
			ORDER BY c.id DESC
			LIMIT $8`, orgID, f.BeforeID, f.Result, f.KeyPrefix, f.Route, f.From, f.To, f.Limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c erp.CallRow
			if err := rows.Scan(&c.ID, &c.CreatedAt, &c.KeyName, &c.KeyPrefix, &c.Method, &c.Route, &c.Status,
				&c.ErrorCode, &c.DurationMS, &c.RequestID, &c.Query, &c.UserAgent, &c.IP, &c.BytesIn,
				&c.BytesOut, &c.Ref, &c.AuthReason, &c.ErrorDetail); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

// batchKeyName scans the batch columns and then the key name, which the batch
// queries select last.
type batchKeyName struct {
	row  scanner
	name *string
}

func (b batchKeyName) Scan(dest ...any) error {
	return b.row.Scan(append(dest, b.name)...)
}

// ListBatches pages the organization's batches by id, newest first.
func (r *Repository) ListBatches(ctx context.Context, orgID, beforeID int64, limit int) ([]erp.BatchRow, error) {
	return r.queryBatches(ctx, orgID, `AND ($2::bigint = 0 OR b.id < $2)
		ORDER BY b.id DESC LIMIT $3`, beforeID, limit)
}

// ListActiveBatches returns the queued and processing batches, oldest first so
// the panel lists them in the order their chunks run.
func (r *Repository) ListActiveBatches(ctx context.Context, orgID int64, limit int) ([]erp.BatchRow, error) {
	return r.queryBatches(ctx, orgID, `AND b.state IN ('queued', 'processing')
		AND ($2::bigint = 0 OR b.id > $2) ORDER BY b.id ASC LIMIT $3`, 0, limit)
}

// queryBatches runs a batch list. The caller's tail adds its filter, with $1 the
// organization, $2 a cursor id and $3 the limit.
func (r *Repository) queryBatches(ctx context.Context, orgID int64, tail string, cursor int64, limit int) ([]erp.BatchRow, error) {
	var out []erp.BatchRow
	err := r.db.InReadTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+batchColumns+`, COALESCE(k.name, '')
			FROM erp.sync_batches b
			JOIN erp.integrations i ON i.id = b.integration_id
			LEFT JOIN erp.api_keys k ON k.id = b.api_key_id AND k.organization_id = $1
			WHERE b.organization_id = $1 `+tail,
			orgID, cursor, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			b, err := scanBatch(batchKeyName{row: rows, name: &name})
			if err != nil {
				return err
			}
			out = append(out, erp.BatchRow{SyncBatch: b, KeyName: name})
		}
		return rows.Err()
	})
	return out, err
}

// ListOrderImports returns the latest acknowledged orders, both directions.
func (r *Repository) ListOrderImports(ctx context.Context, orgID int64, limit int) ([]erp.OrderImportRow, error) {
	var out []erp.OrderImportRow
	err := r.db.InReadTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT direction, order_id, order_number, erp_reference, acked_status, acked_at, ack_count
			FROM erp.order_imports WHERE organization_id = $1 ORDER BY id DESC LIMIT $2`, orgID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var o erp.OrderImportRow
			if err := rows.Scan(&o.Direction, &o.OrderID, &o.OrderNumber, &o.ErpReference, &o.AckedStatus, &o.AckedAt, &o.AckCount); err != nil {
				return err
			}
			out = append(out, o)
		}
		return rows.Err()
	})
	return out, err
}
