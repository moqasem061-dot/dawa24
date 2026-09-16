package database

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pipelinedTx wraps a pooled connection whose transaction and tenant context
// were initialized in a single batched network round trip.
type pipelinedTx struct {
	conn   *pgxpool.Conn
	closed bool
}

var _ pgx.Tx = (*pipelinedTx)(nil)

func beginSQL(opts pgx.TxOptions) string {
	if opts.AccessMode == pgx.ReadOnly {
		return "BEGIN READ ONLY"
	}
	return "BEGIN"
}

func tenantConfigSQL(ctx context.Context) (string, []any) {
	if isSystem(ctx) {
		return "SELECT set_config('app.is_system', 'on', true)", nil
	}
	orgID, ok := TenantFrom(ctx)
	if !ok {
		return "SELECT set_config('app.current_org_id', '', true)", nil
	}
	return "SELECT set_config('app.current_org_id', $1, true)", []any{strconv.FormatInt(orgID, 10)}
}

// startPipelinedTx acquires a connection and pipelines BEGIN + set_config in one round trip.
func (db *DB) startPipelinedTx(ctx context.Context, pool *pgxpool.Pool, opts pgx.TxOptions) (*pipelinedTx, error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, diagnose(ctx, pool, err)
	}

	batch := &pgx.Batch{}
	batch.Queue(beginSQL(opts))

	sql, args := tenantConfigSQL(ctx)
	if len(args) > 0 {
		batch.Queue(sql, args[0])
	} else {
		batch.Queue(sql)
	}

	br := conn.SendBatch(ctx, batch)
	if _, err := br.Exec(); err != nil {
		_ = br.Close()
		conn.Release()
		return nil, fmt.Errorf("database: pipelined begin: %w", err)
	}
	if _, err := br.Exec(); err != nil {
		_ = br.Close()
		conn.Release()
		return nil, fmt.Errorf("database: pipelined tenant: %w", err)
	}
	if err := br.Close(); err != nil {
		conn.Release()
		return nil, fmt.Errorf("database: pipelined close: %w", err)
	}

	return &pipelinedTx{conn: conn}, nil
}

func (p *pipelinedTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return p.conn.Exec(ctx, sql, args...)
}

func (p *pipelinedTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return p.conn.Query(ctx, sql, args...)
}

func (p *pipelinedTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return p.conn.QueryRow(ctx, sql, args...)
}

func (p *pipelinedTx) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	return p.conn.SendBatch(ctx, b)
}

func (p *pipelinedTx) CopyFrom(ctx context.Context, table pgx.Identifier, columns []string, src pgx.CopyFromSource) (int64, error) {
	return p.conn.CopyFrom(ctx, table, columns, src)
}

func (p *pipelinedTx) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	return p.conn.Conn().Prepare(ctx, name, sql)
}

func (p *pipelinedTx) LargeObjects() pgx.LargeObjects {
	return pgx.LargeObjects{}
}

func (p *pipelinedTx) Begin(ctx context.Context) (pgx.Tx, error) {
	return nil, fmt.Errorf("database: nested transactions not supported in pipelined tx")
}

func (p *pipelinedTx) Commit(ctx context.Context) error {
	if p.closed {
		return nil
	}
	p.closed = true
	_, err := p.conn.Exec(ctx, "COMMIT")
	return err
}

func (p *pipelinedTx) Rollback(ctx context.Context) error {
	if p.closed {
		return nil
	}
	p.closed = true
	_, err := p.conn.Exec(ctx, "ROLLBACK")
	return err
}

func (p *pipelinedTx) Conn() *pgx.Conn {
	return p.conn.Conn()
}

func (p *pipelinedTx) Release() {
	if p.conn != nil {
		p.conn.Release()
		p.conn = nil
	}
}
