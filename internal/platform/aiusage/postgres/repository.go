// Package postgres stores AI usage events in ai.usage_events.
package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/aiusage"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Repository reads and writes the AI usage ledger.
type Repository struct {
	db *database.DB
}

// NewRepository constructs the ledger store.
func NewRepository(db *database.DB) *Repository { return &Repository{db: db} }

// Insert writes one recorded call.
//
// The write runs as system rather than under the caller's tenant context, and
// that is deliberate: usage is recorded from wherever the AI call happened —
// a background import worker, a River job, a request whose transaction has
// already committed — and those contexts do not reliably carry the tenant GUC
// the row-level security policy reads. The organisation is supplied explicitly
// on the row, so nothing is being widened; only the write path is.
//
// Reads go the other way, under tenant context, so a pharmacy sees its own
// consumption and nobody else's.
func (r *Repository) Insert(ctx context.Context, e aiusage.Entry) error {
	if e.OrganizationID <= 0 {
		return fmt.Errorf("aiusage: organisation id required")
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			INSERT INTO ai.usage_events (
				organization_id, user_id, capability, feature, model,
				gateway_request_id, input_tokens, output_tokens,
				cost_nano_usd, cost_known, duration_ms,
				status, finish_reason, error_message,
				from_cache, fallback, created_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17);`,
			e.OrganizationID, e.UserID, e.Capability, e.Feature, e.Model,
			e.RequestID, e.InputTokens, e.OutputTokens,
			e.CostNanoUSD, e.CostKnown, e.DurationMS,
			e.Status, e.FinishReason, truncateError(e.ErrorMessage),
			e.FromCache, e.Fallback, e.CreatedAt)
		if err != nil {
			return fmt.Errorf("aiusage postgres: insert usage event: %w", err)
		}
		return nil
	})
}

// List returns matching entries newest first, with the unpaged total.
func (r *Repository) List(ctx context.Context, f aiusage.Filter) ([]aiusage.Entry, int, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	if f.Offset < 0 {
		f.Offset = 0
	}

	var where []string
	var args []any

	if f.OrganizationID > 0 {
		args = append(args, f.OrganizationID)
		where = append(where, fmt.Sprintf("organization_id = $%d", len(args)))
	}
	if !f.Since.IsZero() {
		args = append(args, f.Since)
		where = append(where, fmt.Sprintf("created_at >= $%d", len(args)))
	}
	if !f.Until.IsZero() {
		args = append(args, f.Until)
		where = append(where, fmt.Sprintf("created_at <= $%d", len(args)))
	}
	if strings.TrimSpace(f.Feature) != "" && f.Feature != "all" {
		feat := strings.TrimSpace(f.Feature)
		switch feat {
		case "smart_order":
			where = append(where, "(feature = 'smart_order' OR feature = 'smartorder')")
		case "savings":
			where = append(where, "(feature = 'savings' OR feature = 'saving_products')")
		case "variant_match":
			where = append(where, "(feature = 'variant_match' OR feature = 'variants')")
		case "compare_match":
			where = append(where, "(feature = 'compare_match' OR feature = 'compare' OR feature = 'compare_discounts')")
		case "catalog_import":
			where = append(where, "(feature = 'catalog_import' OR feature = 'vendor_import')")
		default:
			args = append(args, feat)
			where = append(where, fmt.Sprintf("feature = $%d", len(args)))
		}
	}
	if strings.TrimSpace(f.Model) != "" && f.Model != "all" {
		args = append(args, strings.TrimSpace(f.Model))
		where = append(where, fmt.Sprintf("model = $%d", len(args)))
	}
	if strings.TrimSpace(f.Status) != "" && f.Status != "all" {
		args = append(args, strings.TrimSpace(f.Status))
		where = append(where, fmt.Sprintf("status = $%d", len(args)))
	}
	if strings.TrimSpace(f.Search) != "" {
		p := "%" + strings.TrimSpace(f.Search) + "%"
		args = append(args, p)
		idx := len(args)
		where = append(where, fmt.Sprintf("(gateway_request_id ILIKE $%d OR error_message ILIKE $%d OR feature ILIKE $%d OR model ILIKE $%d)", idx, idx, idx, idx))
	}

	clause := "1=1"
	if len(where) > 0 {
		clause = strings.Join(where, " AND ")
	}

	var out []aiusage.Entry
	var total int
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(txCtx,
			"SELECT COUNT(*) FROM ai.usage_events WHERE "+clause, args...).Scan(&total); err != nil {
			return fmt.Errorf("aiusage postgres: count usage events: %w", err)
		}

		paged := append(append([]any{}, args...), f.Limit, f.Offset)
		rows, err := tx.Query(txCtx, fmt.Sprintf(`
			SELECT id, organization_id, user_id, capability, feature, model,
			       gateway_request_id, input_tokens, output_tokens,
			       cost_nano_usd, cost_known, duration_ms,
			       status, finish_reason, error_message,
			       from_cache, fallback, created_at
			FROM ai.usage_events
			WHERE %s
			ORDER BY created_at DESC, id DESC
			LIMIT $%d OFFSET $%d`, clause, len(args)+1, len(args)+2), paged...)
		if err != nil {
			return fmt.Errorf("aiusage postgres: list usage events: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var e aiusage.Entry
			if err := rows.Scan(&e.ID, &e.OrganizationID, &e.UserID, &e.Capability,
				&e.Feature, &e.Model, &e.RequestID, &e.InputTokens, &e.OutputTokens,
				&e.CostNanoUSD, &e.CostKnown, &e.DurationMS,
				&e.Status, &e.FinishReason, &e.ErrorMessage,
				&e.FromCache, &e.Fallback, &e.CreatedAt); err != nil {
				return fmt.Errorf("aiusage postgres: scan usage event: %w", err)
			}
			out = append(out, e)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// Summarize aggregates one organisation's window, or all organisations when orgID <= 0.
//
// Costs are summed only over the rows that carried a published price, and the
// count of those rows comes back with the total. A caller can then say "at
// least this much" rather than presenting a partial sum as complete — which is
// the distinction the previous screens erased by treating an absent price as
// zero.
func (r *Repository) Summarize(ctx context.Context, orgID int64, since time.Time) (aiusage.Summary, error) {
	s := aiusage.Summary{Since: since, Until: time.Now().UTC()}
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			SELECT COUNT(*),
			       COUNT(*) FILTER (WHERE status = 'success' AND NOT fallback),
			       COUNT(*) FILTER (WHERE status <> 'success'),
			       COUNT(*) FILTER (WHERE from_cache),
			       COUNT(*) FILTER (WHERE fallback),
			       COALESCE(SUM(input_tokens), 0),
			       COALESCE(SUM(output_tokens), 0),
			       COALESCE(SUM(cost_nano_usd) FILTER (WHERE cost_known), 0),
			       COUNT(*) FILTER (WHERE cost_known)
			FROM ai.usage_events
			WHERE ($1::bigint <= 0 OR organization_id = $1) AND ($2::timestamptz IS NULL OR created_at >= $2);`,
			orgID, nullableTime(since),
		).Scan(&s.Requests, &s.Succeeded, &s.Failed, &s.Cached, &s.FellBack,
			&s.InputTokens, &s.OutputTokens, &s.CostNanoUSD, &s.PricedRequests)
	})
	if err != nil {
		return aiusage.Summary{}, fmt.Errorf("aiusage postgres: summarize: %w", err)
	}
	return s, nil
}

// ByFeature breaks a window down by the screen that spent it.
func (r *Repository) ByFeature(ctx context.Context, orgID int64, since time.Time) ([]aiusage.FeatureUsage, error) {
	var out []aiusage.FeatureUsage
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT COALESCE(NULLIF(feature, ''), capability) AS label,
			       COUNT(*),
			       COALESCE(SUM(input_tokens + output_tokens), 0),
			       COALESCE(SUM(cost_nano_usd) FILTER (WHERE cost_known), 0)
			FROM ai.usage_events
			WHERE ($1::bigint <= 0 OR organization_id = $1) AND ($2::timestamptz IS NULL OR created_at >= $2)
			GROUP BY 1
			ORDER BY 2 DESC, 1;`, orgID, nullableTime(since))
		if err != nil {
			return fmt.Errorf("aiusage postgres: by feature: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var f aiusage.FeatureUsage
			if err := rows.Scan(&f.Feature, &f.Requests, &f.TotalTokens, &f.CostNanoUSD); err != nil {
				return fmt.Errorf("aiusage postgres: scan feature usage: %w", err)
			}
			out = append(out, f)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// nullableTime turns a zero time into a SQL NULL, so one query serves both
// "since this date" and "everything retained".
func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

// truncateError bounds what an upstream message can put in the ledger. A
// provider that returns a megabyte of HTML on an error should not be able to
// make one row dominate the table.
func truncateError(msg string) string {
	const limit = 500
	if len(msg) <= limit {
		return msg
	}
	return msg[:limit]
}

var _ aiusage.Repository = (*Repository)(nil)
