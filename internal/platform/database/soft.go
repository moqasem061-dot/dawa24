package database

// Best-effort statements inside a transaction.
//
// `_, _ = tx.Exec(...)` reads as "try this, and never mind if it fails". In
// PostgreSQL it means something else: a failed statement ABORTS the whole
// transaction, and every statement after it fails with SQLSTATE 25P02
// ("current transaction is aborted, commands ignored until end of transaction
// block") — or the commit quietly rolls back everything the transaction did.
// So the ignored side-write does not fail alone; it takes the main operation
// with it, and the error the user sees names a statement that did nothing
// wrong. That is how editing an order came to fail on "update line 1673":
// a lookup two statements earlier had referenced a column that does not exist.
//
// Soft runs the statement inside a savepoint. If it fails, only it is undone
// and the transaction carries on, which is what the ignored error always
// meant to say.

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// softSavepoint is reused by every call. PostgreSQL stacks savepoints of the
// same name, and each call releases its own before returning.
const softSavepoint = "dawa_soft"

// Soft runs fn inside a savepoint on tx and returns its error. A failure rolls
// back to the savepoint, so the enclosing transaction stays usable.
//
// Where no savepoint can be opened — a read that runs outside a transaction
// block — nothing can be aborted either, and fn simply runs.
func Soft(ctx context.Context, tx pgx.Tx, fn func() error) error {
	if _, err := tx.Exec(ctx, "SAVEPOINT "+softSavepoint); err != nil {
		return fn()
	}
	if err := fn(); err != nil {
		// Failing to roll back to our own savepoint leaves the transaction
		// aborted whatever we do next; the caller's next statement reports it.
		_, _ = tx.Exec(ctx, "ROLLBACK TO SAVEPOINT "+softSavepoint)
		_, _ = tx.Exec(ctx, "RELEASE SAVEPOINT "+softSavepoint)
		return err
	}
	_, err := tx.Exec(ctx, "RELEASE SAVEPOINT "+softSavepoint)
	return err
}

// SoftExec is Soft for a single statement.
func SoftExec(ctx context.Context, tx pgx.Tx, sql string, args ...any) error {
	return Soft(ctx, tx, func() error {
		_, err := tx.Exec(ctx, sql, args...)
		return err
	})
}
