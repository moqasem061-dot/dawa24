package database

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// joinedTxKey marks a context whose InTx and InReadTx calls run on an existing
// transaction. See JoinTx.
type joinedTxKey struct{}

// JoinTx runs fn with a context in which every InTx and InReadTx call runs on
// tx instead of opening its own transaction. A guard and the write it protects
// then commit or roll back together. The caller owns tx and commits or rolls it
// back after JoinTx returns. Use it only for the duration of fn.
func JoinTx(ctx context.Context, tx pgx.Tx, fn func(context.Context) error) error {
	return fn(context.WithValue(ctx, joinedTxKey{}, tx))
}

func joinedTx(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(joinedTxKey{}).(pgx.Tx)
	return tx, ok && tx != nil
}
