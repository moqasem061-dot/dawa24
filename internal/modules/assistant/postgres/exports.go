package postgres

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// SaveExport stores a generated file and returns the download token. Only the
// token's hash is written, so a database read cannot be turned into a link.
func (r *Repository) SaveExport(ctx context.Context, actor authctx.Actor, file assistant.ExportFile) (string, error) {
	if actor.UserID <= 0 || len(file.Content) == 0 {
		return "", errors.New("assistant export: owner and content are required")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("assistant export: token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))

	var orgID *int64
	if actor.OrgID > 0 {
		orgID = &actor.OrgID
	}
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			INSERT INTO assistant.exports
			       (token_hash, organization_id, user_id, filename, mime_type, row_count, content, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, now() + make_interval(secs => $8));`,
			hash[:], orgID, actor.UserID, file.Filename, file.MIMEType, file.Rows, file.Content, assistant.ExportTTL)
		return err
	})
	if err != nil {
		return "", fmt.Errorf("assistant export: save: %w", err)
	}
	return token, nil
}

// LoadExport returns an unexpired export by token. Ownership is checked by the
// caller against the returned owner ids; a missing, expired or unknown token
// returns nil.
func (r *Repository) LoadExport(ctx context.Context, token string) (*assistant.Export, error) {
	if len(token) < 40 || len(token) > 64 {
		return nil, nil
	}
	hash := sha256.Sum256([]byte(token))
	var (
		e     assistant.Export
		orgID *int64
	)
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			SELECT id, organization_id, user_id, filename, mime_type, row_count, content
			  FROM assistant.exports
			 WHERE token_hash = $1 AND expires_at > now();`, hash[:]).
			Scan(&e.ID, &orgID, &e.UserID, &e.Filename, &e.MIMEType, &e.Rows, &e.Content)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("assistant export: load: %w", err)
	}
	if orgID != nil {
		e.OrganizationID = *orgID
	}
	return &e, nil
}

// PurgeExpiredExports deletes files past their expiry.
func (r *Repository) PurgeExpiredExports(ctx context.Context, now time.Time) (int, error) {
	var n int64
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `DELETE FROM assistant.exports WHERE expires_at <= $1;`, now)
		n = tag.RowsAffected()
		return err
	})
	return int(n), err
}
