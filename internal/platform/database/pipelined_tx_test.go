package database

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestBeginSQL(t *testing.T) {
	if got := beginSQL(pgx.TxOptions{AccessMode: pgx.ReadOnly}); got != "BEGIN READ ONLY" {
		t.Errorf("beginSQL(ReadOnly) = %q, want %q", got, "BEGIN READ ONLY")
	}
	if got := beginSQL(pgx.TxOptions{AccessMode: pgx.ReadWrite}); got != "BEGIN" {
		t.Errorf("beginSQL(ReadWrite) = %q, want %q", got, "BEGIN")
	}
}

func TestTenantConfigSQL(t *testing.T) {
	// System context
	sysCtx := AsSystem(context.Background())
	sql, args := tenantConfigSQL(sysCtx)
	if sql != "SELECT set_config('app.is_system', 'on', true)" || len(args) != 0 {
		t.Errorf("tenantConfigSQL(sysCtx) = %q, %v", sql, args)
	}

	// Tenant context
	tenantCtx := WithTenant(context.Background(), 123)
	sql, args = tenantConfigSQL(tenantCtx)
	if sql != "SELECT set_config('app.current_org_id', $1, true)" || len(args) != 1 || args[0] != "123" {
		t.Errorf("tenantConfigSQL(tenantCtx) = %q, %v", sql, args)
	}

	// Empty context
	sql, args = tenantConfigSQL(context.Background())
	if sql != "SELECT set_config('app.current_org_id', '', true)" || len(args) != 0 {
		t.Errorf("tenantConfigSQL(emptyCtx) = %q, %v", sql, args)
	}
}
