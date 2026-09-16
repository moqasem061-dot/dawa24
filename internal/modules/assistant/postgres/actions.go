package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/assistant/actions"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

var _ actions.Store = (*Repository)(nil)

// CreatePendingAction stores a proposal.
func (r *Repository) CreatePendingAction(ctx context.Context, p *actions.Pending) error {
	args, err := p.Args.Marshal()
	if err != nil {
		return err
	}
	preview, err := json.Marshal(p.Preview)
	if err != nil {
		return err
	}
	var orgID, convID *int64
	if p.OrganizationID > 0 {
		orgID = &p.OrganizationID
	}
	if p.ConversationID > 0 {
		convID = &p.ConversationID
	}
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			INSERT INTO assistant.pending_actions
			       (organization_id, user_id, scope, conversation_id, channel, action, risk,
			        args, preview, preview_hash, status, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'pending', $11)
			RETURNING id, public_id, created_at;`,
			orgID, p.UserID, p.Scope, convID, string(p.Channel), p.Action, string(p.Risk),
			args, preview, p.PreviewHash, p.ExpiresAt).Scan(&p.ID, &p.PublicID, &p.CreatedAt)
	})
}

// GetPendingAction loads a proposal for its own user and organisation. Any
// other combination returns nil, the same as an id that never existed.
func (r *Repository) GetPendingAction(ctx context.Context, publicID uuid.UUID, userID, orgID int64) (*actions.Pending, error) {
	var (
		p                     actions.Pending
		org, conv             *int64
		channel, risk, status string
		args, preview         []byte
		outcome               []byte
	)
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			SELECT id, public_id, organization_id, user_id, scope, conversation_id, channel, action, risk,
			       args, preview, preview_hash, status, outcome, error_message, expires_at, created_at
			  FROM assistant.pending_actions
			 WHERE public_id = $1 AND user_id = $2 AND organization_id IS NOT DISTINCT FROM $3;`,
			publicID, userID, nullableID(orgID)).
			Scan(&p.ID, &p.PublicID, &org, &p.UserID, &p.Scope, &conv, &channel, &p.Action, &risk,
				&args, &preview, &p.PreviewHash, &status, &outcome, &p.Error, &p.ExpiresAt, &p.CreatedAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("assistant action: load: %w", err)
	}
	if org != nil {
		p.OrganizationID = *org
	}
	if conv != nil {
		p.ConversationID = *conv
	}
	p.Channel, p.Risk, p.Status = actions.Channel(channel), actions.Risk(risk), actions.Status(status)
	if p.Args, err = actions.UnmarshalArgs(args); err != nil {
		return nil, fmt.Errorf("assistant action: args: %w", err)
	}
	if err := json.Unmarshal(preview, &p.Preview); err != nil {
		return nil, fmt.Errorf("assistant action: preview: %w", err)
	}
	if len(outcome) > 0 {
		p.Outcome = &actions.Outcome{}
		if err := json.Unmarshal(outcome, p.Outcome); err != nil {
			return nil, fmt.Errorf("assistant action: outcome: %w", err)
		}
	}
	return &p, nil
}

// ClaimPendingAction is the single-use guarantee: of two confirms racing, one
// moves the row and the other finds nothing to move.
func (r *Repository) ClaimPendingAction(ctx context.Context, id int64) (bool, error) {
	var n int64
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE assistant.pending_actions
			   SET status = 'executing', decided_at = now()
			 WHERE id = $1 AND status = 'pending' AND expires_at > now();`, id)
		n = tag.RowsAffected()
		return err
	})
	return n == 1, err
}

// FinishPendingAction records the result of an executing action.
func (r *Repository) FinishPendingAction(ctx context.Context, id int64, status actions.Status, outcome *actions.Outcome, errText string) error {
	var body []byte
	if outcome != nil {
		b, err := json.Marshal(outcome)
		if err != nil {
			return err
		}
		body = b
	}
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			UPDATE assistant.pending_actions
			   SET status = $2, outcome = $3, error_message = $4,
			       executed_at = CASE WHEN $2 = 'executed' THEN now() ELSE executed_at END
			 WHERE id = $1 AND status = 'executing';`, id, string(status), body, errText)
		return err
	})
}

// DecidePendingAction closes a proposal that will not run.
func (r *Repository) DecidePendingAction(ctx context.Context, id int64, status actions.Status) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			UPDATE assistant.pending_actions SET status = $2, decided_at = now()
			 WHERE id = $1 AND status = 'pending';`, id, string(status))
		return err
	})
}
