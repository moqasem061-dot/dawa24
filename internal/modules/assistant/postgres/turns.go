package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
)

// Turns, attachments and the audit trail.
//
// Every statement here goes through ownCtx: a tenant caller stays under
// row-level security, and a caller with no organisation — platform staff — is
// scoped by the user_id predicate each query carries instead. See support.go
// for why that distinction has to exist.

// CreateTurn opens a turn and fills in its identifiers.
func (r *Repository) CreateTurn(ctx context.Context, t *assistant.Turn) error {
	return r.db.InTx(ownCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var orgID *int64
		if t.OrganizationID > 0 {
			orgID = &t.OrganizationID
		}
		return tx.QueryRow(txCtx, `
			INSERT INTO assistant.turns
			    (conversation_id, organization_id, user_id, agent_role, status, question)
			VALUES ($1, $2, $3, $4, 'running', $5)
			RETURNING id, public_id, created_at;
		`, t.ConversationID, orgID, t.UserID, t.AgentRole, t.Question).
			Scan(&t.ID, &t.PublicID, &t.CreatedAt)
	})
}

// FinishTurn records the outcome.
//
// It is called on every exit path — success, model failure, cancellation, and
// the client walking away mid-answer — because the tokens were spent either
// way and an answer nobody can find is the same as an answer that was lost.
func (r *Repository) FinishTurn(ctx context.Context, t *assistant.Turn) error {
	return r.db.InTx(ownCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			UPDATE assistant.turns
			   SET status = $2, answer = $3, error_code = $4,
			       input_tokens = $5, output_tokens = $6, tool_calls = $7,
			       entities = $8, finished_at = now()
			 WHERE id = $1;
		`, t.ID, string(t.Status), t.Answer, t.ErrorCode,
			t.InputTokens, t.OutputTokens, t.ToolCalls, encodeEntities(t.Entities))
		return err
	})
}

// GetTurn fetches one turn for the caller who started it.
func (r *Repository) GetTurn(ctx context.Context, publicID string, orgID, userID int64) (*assistant.Turn, error) {
	var out *assistant.Turn
	err := r.db.InReadTx(ownCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var t assistant.Turn
		var org *int64
		var entsRaw []byte
		err := tx.QueryRow(txCtx, `
			SELECT id, public_id, conversation_id, organization_id, user_id, agent_role,
			       status, question, answer, error_code, input_tokens, output_tokens,
			       tool_calls, COALESCE(entities, '[]'::jsonb), created_at, finished_at
			  FROM assistant.turns
			 WHERE public_id = $1 AND user_id = $2
			   AND (organization_id IS NOT DISTINCT FROM $3);
		`, publicID, userID, nullableOrg(orgID)).Scan(
			&t.ID, &t.PublicID, &t.ConversationID, &org, &t.UserID, &t.AgentRole,
			&t.Status, &t.Question, &t.Answer, &t.ErrorCode, &t.InputTokens,
			&t.OutputTokens, &t.ToolCalls, &entsRaw, &t.CreatedAt, &t.FinishedAt)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		if org != nil {
			t.OrganizationID = *org
		}
		t.Entities = decodeEntities(entsRaw)
		out = &t
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("assistant: get turn: %w", err)
	}
	return out, nil
}

// LatestRunningTurn finds an answer still being written for this conversation.
//
// This is what lets a reopened drawer rejoin a turn that started before the
// page was closed, instead of showing a blank thread while the server is busy
// finishing the reply.
func (r *Repository) LatestRunningTurn(ctx context.Context, convID, userID int64) (*assistant.Turn, error) {
	var out *assistant.Turn
	err := r.db.InReadTx(ownCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var t assistant.Turn
		var org *int64
		var entsRaw []byte
		err := tx.QueryRow(txCtx, `
			SELECT id, public_id, conversation_id, organization_id, user_id, agent_role,
			       status, question, answer, error_code, input_tokens, output_tokens,
			       tool_calls, COALESCE(entities, '[]'::jsonb), created_at, finished_at
			  FROM assistant.turns
			 WHERE conversation_id = $1 AND user_id = $2 AND status = 'running'
			 ORDER BY id DESC
			 LIMIT 1;
		`, convID, userID).Scan(
			&t.ID, &t.PublicID, &t.ConversationID, &org, &t.UserID, &t.AgentRole,
			&t.Status, &t.Question, &t.Answer, &t.ErrorCode, &t.InputTokens,
			&t.OutputTokens, &t.ToolCalls, &entsRaw, &t.CreatedAt, &t.FinishedAt)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		if org != nil {
			t.OrganizationID = *org
		}
		t.Entities = decodeEntities(entsRaw)
		out = &t
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("assistant: latest running turn: %w", err)
	}
	return out, nil
}

// RecordToolCall appends to the audit trail.
//
// It swallows its error deliberately. The trail must not be able to fail a
// read the caller was entitled to make, and a caller has nothing useful to do
// about a failed audit write — the alternative is refusing a legitimate answer
// because bookkeeping was unavailable.
func (r *Repository) RecordToolCall(ctx context.Context, entry assistant.ToolAudit) {
	err := r.db.InTx(ownCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			INSERT INTO assistant.tool_audit
			    (turn_id, organization_id, user_id, agent_role, tool_name,
			     decision, permission, detail, latency_ms, row_count)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);
		`, nullableID(entry.TurnID), nullableOrg(entry.OrganizationID), entry.UserID,
			entry.AgentRole, entry.ToolName, entry.Decision, entry.Permission,
			entry.Detail, entry.LatencyMS, entry.RowCount)
		return err
	})
	if err != nil {
		r.logAuditFailure(ctx, entry, err)
	}
}

func nullableOrg(orgID int64) *int64 {
	if orgID <= 0 {
		return nil
	}
	return &orgID
}

func nullableID(id int64) *int64 {
	if id <= 0 {
		return nil
	}
	return &id
}

// ---------------------------------------------------------------------------
// Attachments
// ---------------------------------------------------------------------------

// CreateAttachment records an uploaded file. The bytes are already in object
// storage; this is the row that makes them findable and access-controlled.
func (r *Repository) CreateAttachment(ctx context.Context, a *assistant.AttachmentRow) error {
	return r.db.InTx(ownCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			INSERT INTO assistant.attachments
			    (organization_id, user_id, conversation_id, filename, mime_type,
			     size_bytes, content_hash, storage_key, digest)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id, public_id, created_at;
		`, nullableOrg(a.OrganizationID), a.UserID, a.ConversationID, a.Filename,
			a.MIMEType, a.SizeBytes, a.ContentHash, a.StorageKey, a.Digest).
			Scan(&a.ID, &a.PublicID, &a.CreatedAt)
	})
}

// GetAttachment fetches one attachment for its owner.
//
// Both the organisation and the user are in the WHERE clause, on top of the
// table's row-level security policy. Two of the three would do; all three cost
// nothing and mean no single mistake exposes somebody's uploaded invoice.
func (r *Repository) GetAttachment(
	ctx context.Context, publicID string, orgID, userID int64,
) (*assistant.AttachmentRow, error) {
	var out *assistant.AttachmentRow
	err := r.db.InReadTx(ownCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var a assistant.AttachmentRow
		var org *int64
		err := tx.QueryRow(txCtx, `
			SELECT id, public_id, organization_id, user_id, conversation_id,
			       filename, mime_type, size_bytes, content_hash, storage_key,
			       digest, referenced, created_at
			  FROM assistant.attachments
			 WHERE public_id = $1 AND user_id = $2
			   AND (organization_id IS NOT DISTINCT FROM $3);
		`, publicID, userID, nullableOrg(orgID)).Scan(
			&a.ID, &a.PublicID, &org, &a.UserID, &a.ConversationID,
			&a.Filename, &a.MIMEType, &a.SizeBytes, &a.ContentHash,
			&a.StorageKey, &a.Digest, &a.Referenced, &a.CreatedAt)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		if org != nil {
			a.OrganizationID = *org
		}
		out = &a
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("assistant: get attachment: %w", err)
	}
	return out, nil
}

// SetAttachmentDigest stores the model's reading of a file, so the same upload
// is never analysed twice.
func (r *Repository) SetAttachmentDigest(ctx context.Context, id int64, digest string) error {
	return r.db.InTx(ownCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx,
			`UPDATE assistant.attachments SET digest = $2 WHERE id = $1;`, id, digest)
		return err
	})
}

// MarkAttachmentsReferenced ties uploads to the conversation that used them, so
// the orphan sweep leaves them alone.
func (r *Repository) MarkAttachmentsReferenced(ctx context.Context, ids []int64, convID int64) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.InTx(ownCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			UPDATE assistant.attachments
			   SET referenced = true, conversation_id = $2
			 WHERE id = ANY($1);
		`, ids, convID)
		return err
	})
}

// ---------------------------------------------------------------------------
// Retention
// ---------------------------------------------------------------------------

// PurgeExpiredConversations deletes conversations past their six-month
// deadline, and everything hanging off them by cascade.
//
// Cross-tenant by nature — it sweeps every organisation — so it runs AsSystem,
// which is why it is a background job and not something a request can reach.
func (r *Repository) PurgeExpiredConversations(ctx context.Context, now time.Time) (int, error) {
	var deleted int
	err := r.db.InTx(systemCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			DELETE FROM assistant.conversations
			 WHERE expires_at IS NOT NULL AND expires_at <= $1;
		`, now)
		if err != nil {
			return err
		}
		deleted = int(tag.RowsAffected())
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("assistant: purge expired conversations: %w", err)
	}
	return deleted, nil
}

// PurgeOrphanAttachments removes uploads no saved message ever referenced and
// returns their storage keys so the caller can delete the objects too.
func (r *Repository) PurgeOrphanAttachments(ctx context.Context, olderThan time.Time) ([]string, error) {
	var keys []string
	err := r.db.InTx(systemCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			DELETE FROM assistant.attachments
			 WHERE referenced = false AND created_at <= $1
			 RETURNING storage_key;
		`, olderThan)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				return err
			}
			if key != "" {
				keys = append(keys, key)
			}
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("assistant: purge orphan attachments: %w", err)
	}
	return keys, nil
}
