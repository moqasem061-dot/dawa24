package queue_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/platform/config"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/queue"
)

// The ERP sync runs as chunks on the erpsync queue so a 50,000-item batch
// advances without blocking short jobs, and the reaper's 15-minute ceiling can
// never outlive a chunk. Cleanup is a nightly maintenance job: one attempt,
// idempotent deletes.
func TestERPSyncArgs(t *testing.T) {
	chunk := queue.ERPSyncChunkArgs{BatchID: 1, OrganizationID: 2, ChunkNo: 3}
	if chunk.Kind() != "erp.sync_chunk" {
		t.Errorf("chunk Kind = %q", chunk.Kind())
	}
	opts := chunk.InsertOpts()
	if opts.Queue != "erpsync" {
		t.Errorf("chunk queue = %q; want erpsync (configured in config.Worker.Queues)", opts.Queue)
	}
	if opts.MaxAttempts != 3 {
		t.Errorf("chunk MaxAttempts = %d; want 3", opts.MaxAttempts)
	}

	cleanup := queue.ERPCleanupArgs{}
	if cleanup.Kind() != "erp.cleanup" {
		t.Errorf("cleanup Kind = %q", cleanup.Kind())
	}
	cOpts := cleanup.InsertOpts()
	if cOpts.Queue != "maintenance" {
		t.Errorf("cleanup queue = %q; want maintenance", cOpts.Queue)
	}
	if cOpts.MaxAttempts != 1 {
		t.Errorf("cleanup MaxAttempts = %d; want 1", cOpts.MaxAttempts)
	}
}

// TestNewInsertOnly proves the insert-only client: a River client built from
// an existing database handle with no queues and no workers, never started.
// The composition root calls it lazily on first enqueue, because the database
// handle is not connected at mount time (plan trap 20). Needs a reachable
// database to build the driver, so it runs in the sandbox like the other
// real-Postgres tests; a host run without a DSN skips.
func TestNewInsertOnly(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("TEST_DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("no database URL; the insert-only client needs a real PostgreSQL driver")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{URL: dsn})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(db.Close)
	client, err := queue.NewInsertOnly(db, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err != nil {
		t.Fatalf("NewInsertOnly: %v", err)
	}
	if client == nil {
		t.Fatal("NewInsertOnly returned a nil client")
	}
	if client.Workers() != nil {
		t.Error("insert-only client has a worker registry; it must have none")
	}
}
