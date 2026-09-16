package database

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	_ "github.com/muhiya/dawa24-store/internal/shared/timeutil"
)

// ErrNoTenant is returned when tenant-scoped work is attempted without an
// organisation in context.
//
// This is usually a legitimate request from a user who simply has no active
// organisation — a customer hitting a vendor endpoint, or a member who has not
// selected one yet. Classifying it as an internal error turned that into a 500
// reading "something went wrong on our side", which is both wrong and
// unactionable. It is a forbidden request with a message that says what to do.
var ErrNoTenant = apperr.Forbidden("tenant.required",
	"No active organization. Select one, or ask to be added to a supplier account.")

type ctxKey int

const (
	ctxKeyOrgID ctxKey = iota
	ctxKeySystem
)

// WithTenant marks the context as belonging to one organisation. HTTP middleware
// calls this after resolving the authenticated user's active organisation.
func WithTenant(ctx context.Context, orgID int64) context.Context {
	return context.WithValue(ctx, ctxKeyOrgID, orgID)
}

// TenantFrom returns the organisation bound to this context.
func TenantFrom(ctx context.Context) (int64, bool) {
	orgID, ok := ctx.Value(ctxKeyOrgID).(int64)
	return orgID, ok && orgID > 0
}

// AsSystem marks a context as exempt from row-level security.
//
// Use it only for platform-admin screens, migrations, and background jobs that
// legitimately span tenants — and log why. Every call site is a deliberate hole
// in the isolation guarantee and should read like one.
func AsSystem(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKeySystem, true)
}

// IsSystem checks if the context was marked as system-exempt from tenant isolation.
func IsSystem(ctx context.Context) bool {
	v, _ := ctx.Value(ctxKeySystem).(bool)
	return v
}

func isSystem(ctx context.Context) bool {
	return IsSystem(ctx)
}

// UnscopedTables is every table this package will read outside a transaction.
//
// A read through QueryUnscoped costs one network round trip. The same read
// through InReadTx costs four — BEGIN, the set_config that arms row-level
// security, the query itself, and COMMIT — because the isolation contract at
// the top of this file requires the GUC to be set inside the transaction that
// reads. On a database reached over a network that is three round trips of
// latency bought for nothing, and it is bought on the hottest reads in the
// platform: the session's user row and the RBAC version counter are read on
// every authenticated request, and pg_stat_user_tables recorded 225,611
// sequential scans of org.organizations, a table with three rows in it.
//
// None of these tables has a row-level security policy, so there is nothing for
// the GUC to arm and nothing the transaction protects. That is a fact about the
// schema rather than a judgement about the query, which is why it is written
// down as data here and checked against the live catalogue by
// TestUnscopedTablesHaveNoRLS rather than left to reviewers to remember.
//
// Adding a table here is only correct if `\d+` shows no policy on it. Enabling
// RLS on a table already listed here MUST remove it from this list in the same
// change; the test fails loudly if it does not.
var UnscopedTables = map[string]bool{
	"identity.users":                 true,
	"identity.roles":                 true,
	"identity.permissions":           true,
	"identity.role_permissions":      true,
	"identity.rbac_version":          true,
	"org.organizations":              true,
	"catalog.brands":                 true,
	"catalog.categories":             true,
	"platform_admin.cities":          true,
	"platform_admin.managed_pages":   true,
	"platform_admin.system_settings": true,
}

// QueryUnscoped runs a read with no transaction and no tenant GUC.
//
// One round trip instead of four. See UnscopedTables for when this is legal:
// the SQL must touch only tables listed there. A query that reaches a
// tenant-owned table through this method bypasses nothing — row-level security
// still applies — but with no organisation set it will return zero rows, which
// is a silent wrong answer rather than an error. Read the list before using it.
func (db *DB) QueryUnscoped(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	pool, err := db.getPool()
	if err != nil {
		return nil, err
	}
	return pool.Query(ctx, sql, args...)
}

// QueryRowUnscoped is QueryUnscoped for a single row. The same rules apply.
func (db *DB) QueryRowUnscoped(ctx context.Context, sql string, args ...any) pgx.Row {
	pool, err := db.getPool()
	if err != nil {
		return errRow{err}
	}
	return pool.QueryRow(ctx, sql, args...)
}

// errRow lets QueryRowUnscoped report a pool that is not connected yet through
// the pgx.Row it must return, rather than panicking on a nil pool.
type errRow struct{ err error }

func (r errRow) Scan(...any) error { return r.err }

// InTx runs fn inside a read-write transaction with tenant isolation applied.
//
// The transaction commits if fn returns nil and rolls back otherwise, including
// on panic — a panic mid-order must never leave a half-written basket behind.
func (db *DB) InTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	return db.transact(ctx, pgx.TxOptions{AccessMode: pgx.ReadWrite}, fn)
}

// InReadTx runs fn inside a read-only transaction with tenant isolation applied.
//
// Read-only is not just a hint: it makes PostgreSQL reject writes, so a query
// helper that accidentally mutates state fails loudly in development.
func (db *DB) InReadTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	return db.transact(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly}, fn)
}

func (db *DB) transact(ctx context.Context, opts pgx.TxOptions, fn func(context.Context, pgx.Tx) error) (err error) {
	pool, err := db.getPool()
	if err != nil {
		return err
	}

	// Reads under a role that bypasses row-level security need neither the
	// tenant setting nor BEGIN/COMMIT around them; see lazy_tx.go.
	if opts.AccessMode == pgx.ReadOnly && db.rlsBypassed.Load() {
		return db.readLazily(ctx, pool, fn)
	}

	// When RLS is active, pipeline BEGIN + set_config into a single network
	// round trip via pgx.Batch rather than two sequential round trips.
	if !db.rlsBypassed.Load() {
		ptx, pErr := db.startPipelinedTx(ctx, pool, opts)
		if pErr != nil {
			return pErr
		}
		defer ptx.Release()
		defer func() {
			if p := recover(); p != nil {
				_ = ptx.Rollback(context.WithoutCancel(ctx))
				panic(p)
			}
			if err != nil {
				_ = ptx.Rollback(context.WithoutCancel(ctx))
			}
		}()

		if err = fn(ctx, ptx); err != nil {
			return err
		}
		if err = ptx.Commit(ctx); err != nil {
			return fmt.Errorf("database: commit: %w", err)
		}
		return nil
	}

	tx, err := pool.BeginTx(ctx, opts)
	if err != nil {
		// BeginTx waits for a free connection and returns the context's error
		// when it runs out of patience, so a saturated pool and a client that
		// closed its tab are indistinguishable here. diagnose separates them.
		return diagnose(ctx, pool, err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
			panic(p)
		}
		if err != nil {
			// Use a detached context so rollback still runs when the request
			// context is already cancelled — otherwise a client disconnect
			// leaves the transaction open until the server times it out.
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()

	if err = fn(ctx, tx); err != nil {
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("database: commit: %w", err)
	}
	return nil
}

// applyTenant sets the GUC that row-level security policies read.
//
// SET LOCAL scopes the setting to this transaction, so it is discarded when the
// connection returns to the pool. Session-level SET would leak one tenant's id
// into the next request that borrowed the same connection — which is exactly the
// bug this whole mechanism exists to prevent.
func applyTenant(ctx context.Context, tx pgx.Tx) error {
	if isSystem(ctx) {
		// Explicit cross-tenant access. RLS policies grant the bypass role;
		// we still clear any inherited org so nothing is silently scoped.
		if _, err := tx.Exec(ctx, "SELECT set_config('app.is_system', 'on', true)"); err != nil {
			return fmt.Errorf("database: set system context: %w", err)
		}
		return nil
	}

	orgID, ok := TenantFrom(ctx)
	if !ok {
		// Not every query is tenant-scoped: login, the public catalogue, and
		// reference data are legitimately tenant-free. Those tables have no RLS
		// policy, so leaving the GUC unset is correct. Tenant-owned tables will
		// return zero rows, which is the safe direction to fail.
		if _, err := tx.Exec(ctx, "SELECT set_config('app.current_org_id', '', true)"); err != nil {
			return fmt.Errorf("database: clear tenant context: %w", err)
		}
		return nil
	}

	// The value is formatted in Go rather than cast in SQL.
	//
	// `$1::text` tells PostgreSQL the parameter's type is text, so pgx must
	// encode an int64 as text and has no plan for that — every tenant-scoped
	// transaction failed with "cannot find encode plan". set_config's second
	// argument is text by signature, so the conversion has to happen on this
	// side of the wire.
	if _, err := tx.Exec(ctx,
		"SELECT set_config('app.current_org_id', $1, true)",
		strconv.FormatInt(orgID, 10),
	); err != nil {
		return fmt.Errorf("database: set tenant context: %w", err)
	}
	return nil
}

// IsNotFound reports the no-rows case.
func IsNotFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// IsUniqueViolation reports whether err is a PostgreSQL unique constraint
// violation, so services can turn it into a domain conflict instead of a 500.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// IsForeignKeyViolation reports a referential integrity failure.
func IsForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

// ConstraintName returns the violated constraint, letting a service map a
// specific unique index to a specific field error.
func ConstraintName(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}
	return ""
}
