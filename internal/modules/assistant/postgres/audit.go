package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/muhiya/dawa24-store/internal/modules/assistant"
)

// ListAllConversations returns all assistant sessions across organizations for administrative audit.
func (r *Repository) ListAllConversations(ctx context.Context, search string, limit, offset int) ([]*assistant.ConversationSummary, int, error) {
	return r.ListAllConversationsFiltered(ctx, assistant.AdminConversationFilter{
		Search: search, Limit: limit, Offset: offset,
	})
}

// ListAllConversationsFiltered is the same listing with the filters the audit
// screen needs. ListAllConversations is kept as the unfiltered call its other
// callers already make.
func (r *Repository) ListAllConversationsFiltered(
	ctx context.Context, f assistant.AdminConversationFilter,
) ([]*assistant.ConversationSummary, int, error) {
	f.Normalize()
	search, limit, offset := f.Search, f.Limit, f.Offset
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	whereClause := "WHERE c.deleted_at IS NULL"
	args := []any{}
	argIdx := 1

	if search != "" {
		whereClause += fmt.Sprintf(` AND (
			c.title ILIKE $%d OR
			COALESCE(u.name->>'ar', u.name->>'en', '') ILIKE $%d OR
			u.email ILIKE $%d OR
			u.phone ILIKE $%d OR
			COALESCE(o.name->>'ar', o.name->>'en', '') ILIKE $%d
		)`, argIdx, argIdx, argIdx, argIdx, argIdx)
		args = append(args, "%"+search+"%")
		argIdx++
	}

	// The separate user and organisation filters exist because the single
	// search box could not answer either question on its own: typing a
	// pharmacy's name matched conversations whose TITLE happened to contain it,
	// and there was no way to ask for one person's conversations at all.
	if f.UserQuery != "" {
		whereClause += fmt.Sprintf(` AND (
			COALESCE(u.name->>'ar', u.name->>'en', '') ILIKE $%d OR
			u.email::text ILIKE $%d OR
			COALESCE(u.phone, '') ILIKE $%d
		)`, argIdx, argIdx, argIdx)
		args = append(args, "%"+f.UserQuery+"%")
		argIdx++
	}
	if f.OrganizationQuery != "" {
		whereClause += fmt.Sprintf(` AND (
			COALESCE(o.name->>'ar', '') ILIKE $%d OR
			COALESCE(o.name->>'en', '') ILIKE $%d OR
			COALESCE(o.trade_name->>'ar', '') ILIKE $%d OR
			COALESCE(o.legal_name, '') ILIKE $%d
		)`, argIdx, argIdx, argIdx, argIdx)
		args = append(args, "%"+f.OrganizationQuery+"%")
		argIdx++
	}
	if f.DateFrom != nil {
		whereClause += fmt.Sprintf(" AND c.created_at >= $%d", argIdx)
		args = append(args, *f.DateFrom)
		argIdx++
	}
	if f.DateTo != nil {
		whereClause += fmt.Sprintf(" AND c.created_at < $%d", argIdx)
		args = append(args, *f.DateTo)
		argIdx++
	}
	// "Flagged" is a fact the platform already records, not a judgement.
	//
	// assistant.tool_audit writes a row for every tool call including the
	// denials -- denied_scope, denied_permission, denied_handle -- which are
	// the model reaching for data the caller may not have. A turn that ended in
	// an error counts too. Both are deterministic and work with the Gateway off,
	// which a model-scored "unusual" would not.
	if f.FlaggedOnly {
		whereClause += ` AND (
			EXISTS (
				SELECT 1 FROM assistant.turns t
				JOIN assistant.tool_audit ta ON ta.turn_id = t.id
				WHERE t.conversation_id = c.id AND ta.decision LIKE 'denied%'
			)
			OR EXISTS (
				SELECT 1 FROM assistant.turns t2
				WHERE t2.conversation_id = c.id AND COALESCE(t2.error_code, '') <> ''
			)
		)`
	}

	var total int
	var convs []*assistant.ConversationSummary

	err := r.db.InReadTx(systemCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		countQuery := fmt.Sprintf(`
			SELECT COUNT(DISTINCT c.id)
			FROM assistant.conversations c
			LEFT JOIN org.organizations o ON o.id = c.organization_id
			LEFT JOIN identity.users u ON u.id = c.user_id
			%s
		`, whereClause)

		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return fmt.Errorf("assistant: count conversations: %w", err)
		}

		query := fmt.Sprintf(`
			SELECT 
				c.id, c.public_id, c.organization_id, COALESCE(o.name->>'ar', o.name->>'en', 'منشأة #' || COALESCE(c.organization_id, 0)) AS org_name,
				COALESCE(o.type, '') AS org_type,
				c.user_id, COALESCE(u.name->>'ar', u.name->>'en', u.email, 'مستخدم #' || c.user_id) AS user_name,
				COALESCE(u.email, '') AS user_email, COALESCE(u.phone, '') AS user_phone,
				COALESCE(u.role, '') AS user_role,
				c.title, c.created_at, c.updated_at,
				COUNT(m.id) AS message_count,
				COALESCE(SUM(m.input_tokens), 0) AS total_input_tokens,
				COALESCE(SUM(m.output_tokens), 0) AS total_output_tokens,
				EXISTS (
					SELECT 1 FROM assistant.turns t
					JOIN assistant.tool_audit ta ON ta.turn_id = t.id
					WHERE t.conversation_id = c.id AND ta.decision LIKE 'denied%%'
				) OR EXISTS (
					SELECT 1 FROM assistant.turns t2
					WHERE t2.conversation_id = c.id AND COALESCE(t2.error_code, '') <> ''
				) AS is_flagged,
				(
					SELECT COUNT(*) FROM assistant.turns t3
					JOIN assistant.tool_audit ta3 ON ta3.turn_id = t3.id
					WHERE t3.conversation_id = c.id AND ta3.decision LIKE 'denied%%'
				) AS denied_tool_calls
			FROM assistant.conversations c
			LEFT JOIN org.organizations o ON o.id = c.organization_id
			LEFT JOIN identity.users u ON u.id = c.user_id
			LEFT JOIN assistant.messages m ON m.conversation_id = c.id
			%s
			GROUP BY c.id, c.public_id, c.organization_id, o.name, o.type, c.user_id, u.name, u.email, u.phone, u.role, c.title, c.created_at, c.updated_at
			ORDER BY c.updated_at DESC
			LIMIT $%d OFFSET $%d
		`, whereClause, argIdx, argIdx+1)

		pageArgs := append([]any{}, args...)
		pageArgs = append(pageArgs, limit, offset)
		rows, err := tx.Query(txCtx, query, pageArgs...)
		if err != nil {
			return fmt.Errorf("assistant: list all conversations: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var s assistant.ConversationSummary
			var orgID *int64
			if err := rows.Scan(
				&s.ID, &s.PublicID, &orgID, &s.OrganizationName, &s.OrganizationType,
				&s.UserID, &s.UserName, &s.UserEmail, &s.UserPhone, &s.UserRole,
				&s.Title, &s.CreatedAt, &s.UpdatedAt,
				&s.MessageCount, &s.TotalInputTokens, &s.TotalOutputTokens,
				&s.IsFlagged, &s.DeniedToolCalls,
			); err != nil {
				return fmt.Errorf("assistant: scan conversation summary: %w", err)
			}
			if orgID != nil {
				s.OrganizationID = *orgID
			}
			convs = append(convs, &s)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, 0, err
	}
	return convs, total, nil
}

// GetAssistantStats aggregates platform-wide assistant usage metrics.
func (r *Repository) GetAssistantStats(ctx context.Context) (*assistant.AssistantStats, error) {
	var stats assistant.AssistantStats
	err := r.db.InReadTx(systemCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT 
				(SELECT COUNT(*) FROM assistant.conversations WHERE deleted_at IS NULL),
				(SELECT COUNT(*) FROM assistant.messages),
				(SELECT COALESCE(SUM(input_tokens), 0) FROM assistant.messages),
				(SELECT COALESCE(SUM(output_tokens), 0) FROM assistant.messages),
				(SELECT COUNT(DISTINCT user_id) FROM assistant.conversations WHERE deleted_at IS NULL)
		`
		return tx.QueryRow(txCtx, query).Scan(
			&stats.TotalConversations,
			&stats.TotalMessages,
			&stats.TotalInputTokens,
			&stats.TotalOutputTokens,
			&stats.ActiveUsers,
		)
	})
	if err != nil {
		return nil, fmt.Errorf("assistant: get stats: %w", err)
	}
	return &stats, nil
}
