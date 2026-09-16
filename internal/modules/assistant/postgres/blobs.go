package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Where an attachment's bytes live when object storage does not.
//
// The assistant's uploads used to require an S3 or MinIO bucket to be
// configured and reachable, with no path if it was not: the handler saw a nil
// storage client, answered 503, and the drawer said "تعذّر رفع الملف". On a
// deployment without STORAGE_BUCKET set — which is every developer machine and
// more than one small installation — attaching a photograph simply never worked
// and nothing said why.
//
// So the bytes fall back to here. This is a bounded fallback and not a file
// store: assistant uploads are capped at 10 MB, unreferenced ones are swept
// after a day, and the rest go with their conversation after six months. A
// deployment with object storage still uses it and never writes a byte to this
// column.

// SaveAttachmentContent stores an attachment's bytes in the database.
func (r *Repository) SaveAttachmentContent(ctx context.Context, id int64, content []byte) error {
	if id <= 0 || len(content) == 0 {
		return errors.New("assistant: attachment content requires an id and bytes")
	}
	err := r.db.InTx(ownCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx,
			`UPDATE assistant.attachments SET content = $2 WHERE id = $1;`, id, content)
		return err
	})
	if err != nil {
		return fmt.Errorf("assistant: save attachment content: %w", err)
	}
	return nil
}

// LoadAttachmentContent reads bytes kept in the database.
//
// It takes an id and not a public reference on purpose: the caller has already
// fetched the row through GetAttachment, which is where ownership is checked.
// Row-level security applies here as well, so an id from another tenant returns
// nothing rather than bytes.
func (r *Repository) LoadAttachmentContent(ctx context.Context, id int64) ([]byte, error) {
	var out []byte
	err := r.db.InReadTx(ownCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(txCtx,
			`SELECT content FROM assistant.attachments WHERE id = $1;`, id).Scan(&out)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("assistant: load attachment content: %w", err)
	}
	if len(out) == 0 {
		return nil, errors.New("assistant: attachment has no stored content")
	}
	return out, nil
}
