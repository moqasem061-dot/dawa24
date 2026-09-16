package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
)

// GetOwnedConversation fetches a conversation only for the caller who owns it.
//
// This closes the hole that mattered most. The streaming endpoint used to take
// conversation_id straight from the request body and load its history with no
// check at all, so any signed-in user could post another tenant's id and have
// that conversation read back to them by the model — and then have their own
// turns written into it.
//
// Three predicates, all in SQL so a foreign id returns no rows rather than
// being fetched and compared afterwards:
//
//	organization_id — the tenant, on top of the table's RLS policy
//	user_id         — the individual; a colleague's thread is not yours either
//	agent_role      — the dashboard it was created under, so a user whose role
//	                  changed cannot resume a thread built with another agent's
//	                  tools and another agent's data in its context
func (r *Repository) GetOwnedConversation(
	ctx context.Context, id, orgID, userID int64, agentRole string,
) (*assistant.Conversation, error) {
	var out *assistant.Conversation
	err := r.db.InReadTx(ownCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var c assistant.Conversation
		var org *int64
		err := tx.QueryRow(txCtx, `
			SELECT id, public_id, organization_id, user_id, title,
			       COALESCE(agent_role,''), created_at, updated_at, expires_at, deleted_at
			  FROM assistant.conversations
			 WHERE id = $1
			   AND user_id = $2
			   AND (organization_id IS NOT DISTINCT FROM $3)
			   AND (COALESCE(agent_role,'') = $4 OR COALESCE(agent_role,'') = '')
			   AND deleted_at IS NULL;
		`, id, userID, nullableOrg(orgID), agentRole).Scan(
			&c.ID, &c.PublicID, &org, &c.UserID, &c.Title, &c.AgentRole,
			&c.CreatedAt, &c.UpdatedAt, &c.ExpiresAt, &c.DeletedAt)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		if org != nil {
			c.OrganizationID = *org
		}
		out = &c
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("assistant: get owned conversation: %w", err)
	}
	return out, nil
}

// ListRecentMessages returns the NEWEST turns of a conversation, in
// chronological order within that window.
//
// ListMessages orders by id ASC, which is right for rendering a thread from the
// top and wrong for building a prompt: on a long conversation it fed the model
// the opening exchange and dropped everything that had happened since. This one
// takes the tail and then reverses it.
func (r *Repository) ListRecentMessages(
	ctx context.Context, convID int64, limit int,
) ([]*assistant.Message, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}

	var msgs []*assistant.Message
	err := r.db.InReadTx(ownCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT id, conversation_id, organization_id, role, content,
			       attachments, prompt_version, model_role, input_tokens,
			       output_tokens, gateway_request_id, created_at
			  FROM assistant.messages
			 WHERE conversation_id = $1
			 ORDER BY id DESC
			 LIMIT $2;
		`, convID, limit)
		if err != nil {
			return fmt.Errorf("assistant: list recent messages: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var m assistant.Message
			var orgID *int64
			var attsRaw []byte
			if err := rows.Scan(
				&m.ID, &m.ConversationID, &orgID, &m.Role, &m.Content,
				&attsRaw, &m.PromptVersion, &m.ModelRole, &m.InputTokens,
				&m.OutputTokens, &m.GatewayRequestID, &m.CreatedAt,
			); err != nil {
				return fmt.Errorf("assistant: scan message: %w", err)
			}
			if orgID != nil {
				m.OrganizationID = *orgID
			}
			if len(attsRaw) > 0 {
				_ = json.Unmarshal(attsRaw, &m.Attachments)
			}
			msgs = append(msgs, &m)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}

	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}
