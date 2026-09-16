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

	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// DefaultJobTimeout is the ceiling for any background job execution.
const DefaultJobTimeout = 30 * time.Minute

// Client wraps a River queue client.
type Client struct {
	riverClient *river.Client[pgx.Tx]
	driver      *riverpgxv5.Driver
	workers     *river.Workers
	log         *slog.Logger
}

// Config holds queue client configuration.
type Config struct {
	Queues          map[string]int
	JobTimeout      time.Duration
	ShutdownTimeout time.Duration
}

// New creates a new queue Client using the shared database connection pool.
// If workers is nil, an empty worker registry is initialized.
func New(db *database.DB, workers *river.Workers, cfg config.Worker, log *slog.Logger) (*Client, error) {
	if workers == nil {
		workers = river.NewWorkers()
	}

	driver := riverpgxv5.New(db.Pool())

	queues := make(map[string]river.QueueConfig, len(cfg.Queues))
	for name, count := range cfg.Queues {
		queues[name] = river.QueueConfig{MaxWorkers: count}
	}

	riverClient, err := river.NewClient(driver, &river.Config{
		Queues:     queues,
		Workers:    workers,
		Logger:     log,
		JobTimeout: DefaultJobTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("queue: create river client: %w", err)
	}

	return &Client{
		riverClient: riverClient,
		driver:      driver,
		workers:     workers,
		log:         log,
	}, nil
}

// Migrate applies pending River schema migrations.
// River maintains its own migration versioning in river_migration tables.
func (c *Client) Migrate(ctx context.Context) error {
	migrator, err := rivermigrate.New(c.driver, nil)
	if err != nil {
		return fmt.Errorf("queue: river migrator: %w", err)
	}

	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("queue: river migrate: %w", err)
	}
	return nil
}

// Start begins processing jobs from configured queues.
func (c *Client) Start(ctx context.Context) error {
	if err := c.riverClient.Start(ctx); err != nil {
		return fmt.Errorf("queue: start: %w", err)
	}
	return nil
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
