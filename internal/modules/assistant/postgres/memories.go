package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
)

// SaveMemory stores a persistent organization or user memory.
func (r *Repository) SaveMemory(ctx context.Context, m *assistant.Memory) error {
	if m == nil {
		return errors.New("assistant: nil memory")
	}
	if m.OrganizationID <= 0 {
		return errors.New("assistant: organization_id required for memory")
	}
	m.Content = strings.TrimSpace(m.Content)
	if m.Content == "" {
		return errors.New("assistant: memory content cannot be empty")
	}
	if m.Category == "" {
		m.Category = assistant.MemoryCategoryGeneral
	}
	if m.Scope == "" {
		m.Scope = assistant.MemoryScopeOrganization
	}
	if m.Source == "" {
		m.Source = assistant.MemorySourceUserExplicit
	}
	if m.Confidence <= 0 {
		m.Confidence = 1.0
	}

	return r.db.InTx(ownCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// If key is provided, deactivate previous memory with the same key in this org
		if m.Key != "" {
			_, err := tx.Exec(txCtx, `
				UPDATE assistant.organization_memories
				   SET is_active = false, updated_at = now()
				 WHERE organization_id = $1
				   AND key = $2
				   AND is_active = true;
			`, m.OrganizationID, m.Key)
			if err != nil {
				return err
			}
		}

		query := `
			INSERT INTO assistant.organization_memories (
				organization_id, user_id, scope, category, key, content,
				confidence, source, source_conversation_id, source_turn_id, is_active
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, true)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			m.OrganizationID,
			m.UserID,
			string(m.Scope),
			string(m.Category),
			m.Key,
			m.Content,
			m.Confidence,
			string(m.Source),
			m.SourceConvID,
			m.SourceTurnID,
		).Scan(&m.ID, &m.PublicID, &m.CreatedAt, &m.UpdatedAt)
	})
}

// UpdateMemory updates the content or category of an active memory.
func (r *Repository) UpdateMemory(ctx context.Context, orgID, id int64, content string, category assistant.MemoryCategory) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return errors.New("assistant: content cannot be empty")
	}
	if category == "" {
		category = assistant.MemoryCategoryGeneral
	}

	return r.db.InTx(ownCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE assistant.organization_memories
			   SET content = $1, category = $2, updated_at = now()
			 WHERE id = $3
			   AND organization_id = $4
			   AND is_active = true;
		`, content, string(category), id, orgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errors.New("assistant: memory not found or not owned by organization")
		}
		return nil
	})
}

// DeleteMemory deactivates a memory belonging to the organization.
func (r *Repository) DeleteMemory(ctx context.Context, orgID, id int64) error {
	return r.db.InTx(ownCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE assistant.organization_memories
			   SET is_active = false, updated_at = now()
			 WHERE id = $1
			   AND organization_id = $2
			   AND is_active = true;
		`, id, orgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errors.New("assistant: memory not found or already deleted")
		}
		return nil
	})
}

// ListMemories fetches all active memories for an organization, optionally including user-scoped preferences.
func (r *Repository) ListMemories(ctx context.Context, orgID int64, userID *int64, limit int) ([]*assistant.Memory, error) {
	if orgID <= 0 {
		return nil, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	var list []*assistant.Memory
	err := r.db.InReadTx(ownCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var query string
		var args []any

		if userID != nil && *userID > 0 {
			query = `
				SELECT id, public_id, organization_id, user_id, scope, category, key, content,
				       confidence, source, source_conversation_id, source_turn_id, is_active,
				       created_at, updated_at
				  FROM assistant.organization_memories
				 WHERE organization_id = $1
				   AND is_active = true
				   AND (scope = 'organization' OR user_id = $2)
				 ORDER BY category ASC, updated_at DESC
				 LIMIT $3;
			`
			args = []any{orgID, *userID, limit}
		} else {
			query = `
				SELECT id, public_id, organization_id, user_id, scope, category, key, content,
				       confidence, source, source_conversation_id, source_turn_id, is_active,
				       created_at, updated_at
				  FROM assistant.organization_memories
				 WHERE organization_id = $1
				   AND is_active = true
				 ORDER BY category ASC, updated_at DESC
				 LIMIT $2;
			`
			args = []any{orgID, limit}
		}

		rows, err := tx.Query(txCtx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var m assistant.Memory
			var scope, category, source string
			if err := rows.Scan(
				&m.ID, &m.PublicID, &m.OrganizationID, &m.UserID,
				&scope, &category, &m.Key, &m.Content,
				&m.Confidence, &source, &m.SourceConvID, &m.SourceTurnID,
				&m.IsActive, &m.CreatedAt, &m.UpdatedAt,
			); err != nil {
				return err
			}
			m.Scope = assistant.MemoryScope(scope)
			m.Category = assistant.MemoryCategory(category)
			m.Source = assistant.MemorySource(source)
			list = append(list, &m)
		}
		return rows.Err()
	})
	return list, err
}

// FindMemories searches active memories using text matching or trigram search.
func (r *Repository) FindMemories(ctx context.Context, orgID int64, query string, limit int) ([]*assistant.Memory, error) {
	if orgID <= 0 {
		return nil, nil
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return r.ListMemories(ctx, orgID, nil, limit)
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var list []*assistant.Memory
	err := r.db.InReadTx(ownCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		likeTerm := "%" + query + "%"
		sql := `
			SELECT id, public_id, organization_id, user_id, scope, category, key, content,
			       confidence, source, source_conversation_id, source_turn_id, is_active,
			       created_at, updated_at
			  FROM assistant.organization_memories
			 WHERE organization_id = $1
			   AND is_active = true
			   AND (content ILIKE $2 OR key ILIKE $2)
			 ORDER BY updated_at DESC
			 LIMIT $3;
		`
		rows, err := tx.Query(txCtx, sql, orgID, likeTerm, limit)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var m assistant.Memory
			var scope, category, source string
			if err := rows.Scan(
				&m.ID, &m.PublicID, &m.OrganizationID, &m.UserID,
				&scope, &category, &m.Key, &m.Content,
				&m.Confidence, &source, &m.SourceConvID, &m.SourceTurnID,
				&m.IsActive, &m.CreatedAt, &m.UpdatedAt,
			); err != nil {
				return err
			}
			m.Scope = assistant.MemoryScope(scope)
			m.Category = assistant.MemoryCategory(category)
			m.Source = assistant.MemorySource(source)
			list = append(list, &m)
		}
		return rows.Err()
	})
	return list, err
}
