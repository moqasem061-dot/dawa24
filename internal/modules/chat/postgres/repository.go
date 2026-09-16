package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/chat"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Repository implements chat.Repository using PostgreSQL.
type Repository struct {
	db *database.DB
}

// NewRepository creates a chat repository.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

// CreateConversation inserts a conversation row.
//
// The live schema (migration 049) has: title, context_type, context_id,
// organization_id, created_by. It does NOT have counterparty_org_id, subject,
// status, or last_message_at — those were in the v1 schema (036) which was
// dropped by migration 045.
func (r *Repository) CreateConversation(ctx context.Context, c *chat.Conversation) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			INSERT INTO chat.conversations (organization_id, title, context_type, context_id, created_by)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, public_id, created_at, updated_at;
		`
		// Map domain Subject (i18n.Text) to the flat title column.
		title := c.Subject.Get(i18n.AR)
		if title == "" {
			title = c.Subject.Get(i18n.EN)
		}
		return tx.QueryRow(txCtx, query,
			c.OrganizationID, title, string(c.ContextType), c.ContextID, c.CreatedByUserID,
		).Scan(&c.ID, &c.PublicID, &c.CreatedAt, &c.UpdatedAt)
	})
}

// GetConversationByID fetches one conversation.
func (r *Repository) GetConversationByID(ctx context.Context, id int64) (*chat.Conversation, error) {
	var c chat.Conversation
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT id, public_id, COALESCE(organization_id, 0), title,
			       context_type, context_id, COALESCE(created_by, 0),
			       created_at, updated_at
			FROM chat.conversations WHERE id = $1 AND deleted_at IS NULL;
		`
		var title, ctxType string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&c.ID, &c.PublicID, &c.OrganizationID, &title,
			&ctxType, &c.ContextID, &c.CreatedByUserID,
			&c.CreatedAt, &c.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return chat.ErrNoConversation
			}
			return err
		}
		c.ContextType = chat.ContextType(ctxType)
		// Map the flat title back into the domain's Subject field.
		c.Subject = i18n.Text{i18n.AR: title, i18n.EN: title}
		// The live schema has no status column; treat all as open.
		c.Status = chat.StatusOpen
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListConversationsForOrg returns the org's threads newest-first.
//
// The v2 schema has no counterparty_org_id. Conversations for an org are found
// through the participants table, or by organization_id on the conversation.
func (r *Repository) ListConversationsForOrg(ctx context.Context, orgID int64, limit, offset int) ([]*chat.Conversation, error) {
	var list []*chat.Conversation
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT c.id, c.public_id, COALESCE(c.organization_id, 0), c.title,
			       c.context_type, c.context_id, COALESCE(c.created_by, 0),
			       c.created_at, c.updated_at
			FROM chat.conversations c
			WHERE c.deleted_at IS NULL
			  AND (c.organization_id = $1
			       OR EXISTS (SELECT 1 FROM chat.participants p WHERE p.conversation_id = c.id AND p.organization_id = $1))
			ORDER BY c.updated_at DESC
			LIMIT $2 OFFSET $3;
		`
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		rows, err := tx.Query(txCtx, query, orgID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c chat.Conversation
			var title, ctxType string
			if err := rows.Scan(
				&c.ID, &c.PublicID, &c.OrganizationID, &title,
				&ctxType, &c.ContextID, &c.CreatedByUserID,
				&c.CreatedAt, &c.UpdatedAt,
			); err != nil {
				return err
			}
			c.ContextType = chat.ContextType(ctxType)
			c.Subject = i18n.Text{i18n.AR: title, i18n.EN: title}
			c.Status = chat.StatusOpen
			list = append(list, &c)
		}
		return rows.Err()
	})
	return list, err
}

// AddParticipant registers a user on a conversation.
func (r *Repository) AddParticipant(ctx context.Context, conversationID, userID, orgID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `INSERT INTO chat.participants (conversation_id, user_id, organization_id) VALUES ($1, $2, $3) ON CONFLICT ON CONSTRAINT chat_participant_unique DO NOTHING;`
		_, err := tx.Exec(txCtx, query, conversationID, userID, orgID)
		return err
	})
}

// SendMessage inserts a message.
//
// The v2 schema uses sender_id (not sender_user_id), attachment_url and
// attachment_type (not a jsonb attachments column), and has no read_at.
func (r *Repository) SendMessage(ctx context.Context, m *chat.Message) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		// Extract the first attachment URL if present.
		var attachURL, attachType *string
		if len(m.Attachments) > 0 {
			if u, ok := m.Attachments[0]["url"].(string); ok {
				attachURL = &u
			}
			if t, ok := m.Attachments[0]["type"].(string); ok {
				attachType = &t
			}
		}

		const query = `
			INSERT INTO chat.messages (conversation_id, sender_id, sender_org_id, body, attachment_url, attachment_type)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, created_at;
		`
		if err := tx.QueryRow(txCtx, query, m.ConversationID, m.SenderUserID, m.SenderOrgID, m.Body, attachURL, attachType).
			Scan(&m.ID, &m.CreatedAt); err != nil {
			return fmt.Errorf("chat postgres: send message: %w", err)
		}
		// Bump the conversation's updated_at via the touch trigger.
		const touch = `UPDATE chat.conversations SET updated_at = now() WHERE id = $1;`
		_, err := tx.Exec(txCtx, touch, m.ConversationID)
		return err
	})
}

// ListMessages returns a conversation's messages oldest-first.
//
// The v2 schema uses sender_id, attachment_url/attachment_type, and has no
// read_at on messages.
func (r *Repository) ListMessages(ctx context.Context, conversationID int64, limit int) ([]*chat.Message, error) {
	var list []*chat.Message
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT id, conversation_id, sender_id, sender_org_id, body,
			       attachment_url, attachment_type, created_at
			FROM chat.messages
			WHERE conversation_id = $1 AND deleted_at IS NULL
			ORDER BY created_at ASC, id ASC
			LIMIT $2;
		`
		if limit <= 0 || limit > 500 {
			limit = 100
		}
		rows, err := tx.Query(txCtx, query, conversationID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m chat.Message
			var attachURL, attachType *string
			if err := rows.Scan(
				&m.ID, &m.ConversationID, &m.SenderUserID, &m.SenderOrgID, &m.Body,
				&attachURL, &attachType, &m.CreatedAt,
			); err != nil {
				return err
			}
			// Map attachment columns back into the domain's Attachments slice.
			if attachURL != nil && *attachURL != "" {
				att := map[string]any{"url": *attachURL}
				if attachType != nil {
					att["type"] = *attachType
				}
				m.Attachments = []map[string]any{att}
			}
			list = append(list, &m)
		}
		return rows.Err()
	})
	return list, err
}

// MarkConversationRead updates the participant's last_read_at timestamp.
//
// The v2 schema tracks read state on participants.last_read_at, not on
// individual messages.
func (r *Repository) MarkConversationRead(ctx context.Context, conversationID, orgID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			UPDATE chat.participants SET last_read_at = now()
			WHERE conversation_id = $1 AND organization_id = $2;
		`
		_, err := tx.Exec(txCtx, query, conversationID, orgID)
		return err
	})
}

// CountUnread returns the number of conversations with unread incoming messages.
//
// A conversation is unread for an org when it has messages created after that
// org's participant last_read_at.
func (r *Repository) CountUnread(ctx context.Context, orgID int64) (int, error) {
	var count int
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT COUNT(DISTINCT m.conversation_id)
			FROM chat.messages m
			JOIN chat.participants p ON p.conversation_id = m.conversation_id AND p.organization_id = $1
			WHERE m.sender_org_id IS DISTINCT FROM $1
			  AND m.deleted_at IS NULL
			  AND (p.last_read_at IS NULL OR m.created_at > p.last_read_at);
		`
		return tx.QueryRow(txCtx, query, orgID).Scan(&count)
	})
	return count, err
}
