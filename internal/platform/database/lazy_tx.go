package database

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// A read-only "transaction" that only becomes one when it has to.
//
// InReadTx wrapped every read in BEGIN READ ONLY ... COMMIT. Under the default
// READ COMMITTED isolation each statement already takes its own snapshot, so
// the wrapper bought no consistency; it bought two network round trips per
// read, which on the pharmacy dashboard was 71 of its 149. When the connecting
// role bypasses row-level security there is also no tenant setting to scope.
//
// lazyTx therefore runs statements directly on a pooled connection and opens a
// real read-only transaction the first time a statement needs one: SET LOCAL
// (the role switches the dataset executor and SQL console rely on, trigram
// thresholds), set_config, transaction-scoped advisory locks and row locks.
// Callers keep writing ordinary InReadTx code and lose nothing that depends on
// transaction scope.
type lazyTx struct {
	conn *pgxpool.Conn
	tx   pgx.Tx
}

var _ pgx.Tx = (*lazyTx)(nil)

// needsTransaction reports whether a statement only has its effect inside a
// transaction.
func needsTransaction(sql string) bool {
	s := strings.ToLower(sql)
	return strings.Contains(s, "set local") ||
		strings.Contains(s, "set_config(") ||
		strings.Contains(s, "_xact_") ||
		strings.Contains(s, "for update") ||
		strings.Contains(s, "for share") ||
		strings.Contains(s, "for no key update") ||
		strings.Contains(s, "for key share")
}

func (t *lazyTx) ensure(ctx context.Context, sql string) error {
	if t.tx != nil || !needsTransaction(sql) {
		return nil
	}
	tx, err := t.conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	t.tx = tx
	return nil
}

func (t *lazyTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if err := t.ensure(ctx, sql); err != nil {
		return pgconn.CommandTag{}, err
	}
	if t.tx != nil {
		return t.tx.Exec(ctx, sql, args...)
	}
	return t.conn.Exec(ctx, sql, args...)
}

func (t *lazyTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if err := t.ensure(ctx, sql); err != nil {
		return nil, err
	}
	if t.tx != nil {
		return t.tx.Query(ctx, sql, args...)
	}
	return t.conn.Query(ctx, sql, args...)
}

func (t *lazyTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if err := t.ensure(ctx, sql); err != nil {
		return errRow{err}
	}
	if t.tx != nil {
		return t.tx.QueryRow(ctx, sql, args...)
	}
	return t.conn.QueryRow(ctx, sql, args...)
}

func (t *lazyTx) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	for _, q := range b.QueuedQueries {
		if err := t.ensure(ctx, q.SQL); err != nil {
			return errBatch{err}
		}
	}
	if t.tx != nil {
		return t.tx.SendBatch(ctx, b)
	}
	return t.conn.SendBatch(ctx, b)
}

func (t *lazyTx) CopyFrom(ctx context.Context, table pgx.Identifier, columns []string, src pgx.CopyFromSource) (int64, error) {
	if t.tx != nil {
		return t.tx.CopyFrom(ctx, table, columns, src)
	}
	return t.conn.CopyFrom(ctx, table, columns, src)
}

func (t *lazyTx) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	return t.conn.Conn().Prepare(ctx, name, sql)
}

// LargeObjects need a transaction, so asking for them opens one.
func (t *lazyTx) LargeObjects() pgx.LargeObjects {
	if t.tx == nil {
		if tx, err := t.conn.BeginTx(context.Background(), pgx.TxOptions{AccessMode: pgx.ReadOnly}); err == nil {
			t.tx = tx
		}
	}
	if t.tx == nil {
		return pgx.LargeObjects{}
	}
	return t.tx.LargeObjects()
}

// Begin opens a real transaction (or a savepoint inside one already open).
func (t *lazyTx) Begin(ctx context.Context) (pgx.Tx, error) {
	if t.tx != nil {
		return t.tx.Begin(ctx)
	}
	tx, err := t.conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	t.tx = tx
	return tx, nil
}

func (t *lazyTx) Commit(ctx context.Context) error {
	if t.tx == nil {
		return nil
	}
	return t.tx.Commit(ctx)
}

func (t *lazyTx) Rollback(ctx context.Context) error {
	if t.tx == nil {
		return nil
	}
	return t.tx.Rollback(ctx)
}

func (t *lazyTx) Conn() *pgx.Conn { return t.conn.Conn() }

// errBatch reports a failure to start the transaction a batch needed.
type errBatch struct{ err error }

func (b errBatch) Exec() (pgconn.CommandTag, error) { return pgconn.CommandTag{}, b.err }
func (b errBatch) Query() (pgx.Rows, error)         { return nil, b.err }
func (b errBatch) QueryRow() pgx.Row                { return errRow{b.err} }
func (b errBatch) Close() error                     { return b.err }

// readLazily runs a read-only callback on a pooled connection through lazyTx.
func (db *DB) readLazily(ctx context.Context, pool *pgxpool.Pool, fn func(context.Context, pgx.Tx) error) (err error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return diagnose(ctx, pool, err)
	}
	defer conn.Release()

	t := &lazyTx{conn: conn}
	defer func() {
		if p := recover(); p != nil {
			_ = t.Rollback(context.WithoutCancel(ctx))
			panic(p)
		}
		if err != nil {
			_ = t.Rollback(context.WithoutCancel(ctx))
		}
	}()
	if err = fn(ctx, t); err != nil {
		return err
	}
	return t.Commit(ctx)
}
