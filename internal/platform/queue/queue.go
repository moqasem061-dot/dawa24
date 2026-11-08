// Package queue wraps River background job processing and transaction-aware
// job insertion.
//
// Jobs can be enqueued inside a database transaction via EnqueueTx so that
// background tasks commit atomically with domain state (the transactional outbox pattern).
package queue

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/riverqueue/river/rivertype"

	"github.com/moqasem061-dot/dawa24/internal/platform/config"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// DefaultJobTimeout is the ceiling for any background job execution.
const DefaultJobTimeout = 30 * time.Minute

// Client wraps a River queue client.
type Client struct {
	riverClient *river.Client[pgx.Tx]
	workers     *river.Workers
	log         *slog.Logger
	db          *database.DB
}

// Config holds queue client configuration.
type Config struct {
	Queues          map[string]int
	JobTimeout      time.Duration
	ShutdownTimeout time.Duration
}

// New creates a new queue Client using the shared database connection pool.
// If workers is nil, an empty worker registry is initialized.
func New(db *database.DB, workers *river.Workers, cfg config.Worker, log *slog.Logger, periodicJobs ...*river.PeriodicJob) (*Client, error) {
	if workers == nil {
		workers = river.NewWorkers()
	}

	driver := riverpgxv5.New(db.Pool())

	queues := make(map[string]river.QueueConfig, len(cfg.Queues))
	for name, count := range cfg.Queues {
		queues[name] = river.QueueConfig{MaxWorkers: count}
	}

	riverClient, err := river.NewClient(driver, &river.Config{
		Queues:       queues,
		Workers:      workers,
		PeriodicJobs: periodicJobs,
		Logger:       log,
		JobTimeout:   DefaultJobTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("queue: create river client: %w", err)
	}

	return &Client{
		riverClient: riverClient,
		workers:     workers,
		log:         log,
		db:          db,
	}, nil
}

// NewInsertOnly builds a client that can only enqueue. No queues, no workers,
// never started: the web process has no River workers (they run in cmd/worker),
// but it must still accept ERP batches and insert the chunk jobs in the same
// transaction as the batch rows (the transactional outbox).
//
// The database handle must be connected by the time this is called — the
// composition root wraps it in a lazy enqueuer, because the handle is not
// connected at mount time (plan trap 20). River accepts a config with nil
// Queues and nil Workers; that combination is an insert-only client (verify
// note V1).
func NewInsertOnly(db *database.DB, log *slog.Logger) (*Client, error) {
	driver := riverpgxv5.New(db.Pool())
	riverClient, err := river.NewClient(driver, &river.Config{
		Queues:     nil,
		Workers:    nil,
		Logger:     log,
		JobTimeout: DefaultJobTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("queue: create insert-only river client: %w", err)
	}
	return &Client{
		riverClient: riverClient,
		workers:     nil,
		log:         log,
		db:          db,
	}, nil
}

// MigrateSchema applies pending River schema migrations (River versions its own
// tables in river_migration). It runs from `cli migrate`, as the schema owner,
// alongside the application's migrations: the worker connects as the
// least-privilege role and cannot create tables.
func MigrateSchema(ctx context.Context, db *database.DB) error {
	migrator, err := rivermigrate.New(riverpgxv5.New(db.Pool()), nil)
	if err != nil {
		return fmt.Errorf("queue: river migrator: %w", err)
	}

	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("queue: river migrate: %w", err)
	}
	return nil
}

// Start begins processing jobs from configured queues and activates the stalled job reaper.
func (c *Client) Start(ctx context.Context) error {
	if err := c.riverClient.Start(ctx); err != nil {
		return fmt.Errorf("queue: start: %w", err)
	}
	c.StartStalledJobReaper(ctx, 3*time.Minute, 15*time.Minute)
	return nil
}

// ReapStalledJobs marks jobs in river_job that have been running longer than maxAge as cancelled.
func (c *Client) ReapStalledJobs(ctx context.Context, maxAge time.Duration) (int64, error) {
	if c.db == nil {
		return 0, nil
	}
	if maxAge <= 0 {
		maxAge = 15 * time.Minute
	}
	tag, err := c.db.Pool().Exec(ctx, `
		UPDATE river_job
		SET state = 'cancelled',
		    finalized_at = now()
		WHERE state = 'running'
		  AND attempted_at < now() - ($1 || ' seconds')::interval;
	`, fmt.Sprintf("%d", int(maxAge.Seconds())))
	if err != nil {
		return 0, fmt.Errorf("queue: reap stalled jobs: %w", err)
	}
	if tag.RowsAffected() > 0 {
		c.log.WarnContext(ctx, "reaped stalled river jobs", "count", tag.RowsAffected(), "max_age", maxAge)
	}
	return tag.RowsAffected(), nil
}

// StartStalledJobReaper runs a background goroutine that periodically calls ReapStalledJobs.
func (c *Client) StartStalledJobReaper(ctx context.Context, interval, maxAge time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = c.ReapStalledJobs(ctx, maxAge)
			}
		}
	}()
}

// Stop gracefully stops job processing, waiting for active jobs to complete up to timeout.
func (c *Client) Stop(ctx context.Context) error {
	if err := c.riverClient.Stop(ctx); err != nil {
		return fmt.Errorf("queue: stop: %w", err)
	}
	return nil
}

// Enqueue inserts a job outside of a transaction.
func (c *Client) Enqueue(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	res, err := c.riverClient.Insert(ctx, args, opts)
	if err != nil {
		return nil, fmt.Errorf("queue: enqueue: %w", err)
	}
	return res, nil
}

// EnqueueTx inserts a job inside a pgx transaction.
// The job will only be scheduled if the surrounding transaction commits.
func (c *Client) EnqueueTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	res, err := c.riverClient.InsertTx(ctx, tx, args, opts)
	if err != nil {
		return nil, fmt.Errorf("queue: enqueue tx: %w", err)
	}
	return res, nil
}

// Workers returns the underlying workers registry for adding new worker handlers.
func (c *Client) Workers() *river.Workers {
	return c.workers
}
