package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Repository provides PostgreSQL persistence for the assistant module.
type Repository struct {
	db *database.DB
}

// NewRepository constructs a PostgreSQL assistant repository.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

var _ assistant.Repository = (*Repository)(nil)

// CreateConversation inserts a new conversation row.
func (r *Repository) CreateConversation(ctx context.Context, c *assistant.Conversation) error {
	return r.db.InTx(ownCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var orgID *int64
		if c.OrganizationID > 0 {
			orgID = &c.OrganizationID
		}
		query := `
			INSERT INTO assistant.conversations (
				organization_id, user_id, title, agent_role
			) VALUES ($1, $2, $3, $4)
			RETURNING id, public_id, created_at, updated_at, expires_at
		`
		return tx.QueryRow(txCtx, query, orgID, c.UserID, c.Title, c.AgentRole).
			Scan(&c.ID, &c.PublicID, &c.CreatedAt, &c.UpdatedAt, &c.ExpiresAt)
	})
}

// GetConversation fetches one conversation by primary key.
func (r *Repository) GetConversation(ctx context.Context, id int64) (*assistant.Conversation, error) {
	var c assistant.Conversation
	var orgID *int64
	err := r.db.InReadTx(ownCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, user_id, title,
			       COALESCE(agent_role,''), created_at, updated_at, expires_at, deleted_at
			FROM assistant.conversations
			WHERE id = $1 AND deleted_at IS NULL
		`
		err := tx.QueryRow(txCtx, query, id).Scan(
			&c.ID, &c.PublicID, &orgID, &c.UserID, &c.Title, &c.AgentRole,
			&c.CreatedAt, &c.UpdatedAt, &c.ExpiresAt, &c.DeletedAt,
		)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		if orgID != nil {
			c.OrganizationID = *orgID
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("assistant: get conversation: %w", err)
	}
	if c.ID == 0 {
		return nil, nil
	}
	return &c, nil
}

// GetConversationSummary fetches enriched metadata for one conversation.
func (r *Repository) GetConversationSummary(ctx context.Context, id int64) (*assistant.ConversationSummary, error) {
	var s assistant.ConversationSummary
	var orgID *int64
	err := r.db.InReadTx(ownCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT 
				c.id, c.public_id, c.organization_id, COALESCE(o.name->>'ar', o.name->>'en', 'منشأة #' || COALESCE(c.organization_id, 0)) AS org_name,
				COALESCE(o.type, '') AS org_type,
				c.user_id, COALESCE(u.name->>'ar', u.name->>'en', u.email, 'مستخدم #' || c.user_id) AS user_name,
				COALESCE(u.email, '') AS user_email, COALESCE(u.phone, '') AS user_phone,
				COALESCE(u.role, '') AS user_role,
				c.title, c.created_at, c.updated_at,
				COUNT(m.id) AS message_count,
				COALESCE(SUM(m.input_tokens), 0) AS total_input_tokens,
				COALESCE(SUM(m.output_tokens), 0) AS total_output_tokens
			FROM assistant.conversations c
			LEFT JOIN org.organizations o ON o.id = c.organization_id
			LEFT JOIN identity.users u ON u.id = c.user_id
			LEFT JOIN assistant.messages m ON m.conversation_id = c.id
			WHERE c.id = $1 AND c.deleted_at IS NULL
			GROUP BY c.id, c.public_id, c.organization_id, o.name, o.type, c.user_id, u.name, u.email, u.phone, u.role, c.title, c.created_at, c.updated_at
		`
		err := tx.QueryRow(txCtx, query, id).Scan(
			&s.ID, &s.PublicID, &orgID, &s.OrganizationName, &s.OrganizationType,
			&s.UserID, &s.UserName, &s.UserEmail, &s.UserPhone, &s.UserRole,
			&s.Title, &s.CreatedAt, &s.UpdatedAt,
			&s.MessageCount, &s.TotalInputTokens, &s.TotalOutputTokens,
		)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		if orgID != nil {
			s.OrganizationID = *orgID
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("assistant: get conversation summary: %w", err)
	}
	if s.ID == 0 {
		return nil, nil
	}
	return &s, nil
}

// DeleteConversation marks a conversation as deleted for a user.
func (r *Repository) DeleteConversation(ctx context.Context, id int64, orgID, userID int64) error {
	return r.db.InTx(ownCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var query string
		var args []any
		if orgID > 0 {
			query = `
				UPDATE assistant.conversations
				SET deleted_at = now()
				WHERE id = $1 AND organization_id = $2 AND user_id = $3 AND deleted_at IS NULL
			`
			args = []any{id, orgID, userID}
		} else {
			query = `
				UPDATE assistant.conversations
				SET deleted_at = now()
				WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
			`
			args = []any{id, userID}
		}
		_, err := tx.Exec(txCtx, query, args...)
		return err
	})
}

// ListConversations returns recent active conversations for a user.
func (r *Repository) ListConversations(ctx context.Context, orgID, userID int64, limit, offset int) ([]*assistant.Conversation, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}

	var convs []*assistant.Conversation
	err := r.db.InReadTx(ownCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var query string
		var args []any
		if orgID > 0 {
			query = `
				SELECT id, public_id, organization_id, user_id, title,
				       COALESCE(agent_role,''), created_at, updated_at, expires_at, deleted_at
				FROM assistant.conversations
				WHERE organization_id = $1 AND user_id = $2 AND deleted_at IS NULL
				ORDER BY updated_at DESC
				LIMIT $3 OFFSET $4
			`
			args = []any{orgID, userID, limit, offset}
		} else {
			query = `
				SELECT id, public_id, organization_id, user_id, title,
				       COALESCE(agent_role,''), created_at, updated_at, expires_at, deleted_at
				FROM assistant.conversations
				WHERE user_id = $1 AND deleted_at IS NULL
				ORDER BY updated_at DESC
				LIMIT $2 OFFSET $3
			`
			args = []any{userID, limit, offset}
		}
		rows, err := tx.Query(txCtx, query, args...)
		if err != nil {
			return fmt.Errorf("assistant: list conversations: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var c assistant.Conversation
			var oID *int64
			if err := rows.Scan(
				&c.ID, &c.PublicID, &oID, &c.UserID, &c.Title, &c.AgentRole,
				&c.CreatedAt, &c.UpdatedAt, &c.ExpiresAt, &c.DeletedAt,
			); err != nil {
				return fmt.Errorf("assistant: scan conversation: %w", err)
			}
			if oID != nil {
				c.OrganizationID = *oID
			}
			convs = append(convs, &c)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return convs, nil
}

// SaveMessage records a turn in a conversation.
func (r *Repository) SaveMessage(ctx context.Context, m *assistant.Message) error {
	attsJSON, err := json.Marshal(m.Attachments)
	if err != nil {
		attsJSON = []byte("[]")
	}

	return r.db.InTx(ownCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var orgID *int64
		if m.OrganizationID > 0 {
			orgID = &m.OrganizationID
		}
		query := `
			INSERT INTO assistant.messages (
				conversation_id, organization_id, role, content,
				attachments, prompt_version, model_role, input_tokens,
				output_tokens, gateway_request_id, entities
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			RETURNING id, created_at
		`
		err = tx.QueryRow(
			txCtx, query,
			m.ConversationID, orgID, m.Role, m.Content,
			attsJSON, m.PromptVersion, m.ModelRole, m.InputTokens,
			m.OutputTokens, m.GatewayRequestID, encodeEntities(m.Entities),
		).Scan(&m.ID, &m.CreatedAt)
		if err != nil {
			return fmt.Errorf("assistant: save message: %w", err)
		}

		// Update conversation updated_at
		_, _ = tx.Exec(txCtx, `UPDATE assistant.conversations SET updated_at = now() WHERE id = $1`, m.ConversationID)
		return nil
	})
}

// ListMessages returns turn history for a conversation in chronological order.
func (r *Repository) ListMessages(ctx context.Context, convID int64, limit int) ([]*assistant.Message, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var msgs []*assistant.Message
	err := r.db.InReadTx(ownCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, conversation_id, organization_id, role, content,
			       attachments, prompt_version, model_role, input_tokens,
			       output_tokens, gateway_request_id, created_at,
			       COALESCE(entities, '[]'::jsonb)
			FROM assistant.messages
			WHERE conversation_id = $1
			ORDER BY id ASC
			LIMIT $2
		`
		rows, err := tx.Query(txCtx, query, convID, limit)
		if err != nil {
			return fmt.Errorf("assistant: list messages: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var m assistant.Message
			var orgID *int64
			var attsRaw, entsRaw []byte
			if err := rows.Scan(
				&m.ID, &m.ConversationID, &orgID, &m.Role, &m.Content,
				&attsRaw, &m.PromptVersion, &m.ModelRole, &m.InputTokens,
				&m.OutputTokens, &m.GatewayRequestID, &m.CreatedAt, &entsRaw,
			); err != nil {
				return fmt.Errorf("assistant: scan message: %w", err)
			}
			if orgID != nil {
				m.OrganizationID = *orgID
			}
			if len(attsRaw) > 0 {
				_ = json.Unmarshal(attsRaw, &m.Attachments)
			}
			m.Entities = decodeEntities(entsRaw)
			msgs = append(msgs, &m)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return msgs, nil
}
