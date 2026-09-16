package queue

import (
	"time"

	"github.com/riverqueue/river"
)

// OrderNotificationArgs defines job parameters for dispatching order transition alerts.
type OrderNotificationArgs struct {
	OrderID    int64     `json:"order_id"`
	CustomerID int64     `json:"customer_id"`
	ToStatus   string    `json:"to_status"`
	At         time.Time `json:"at"`
}

func (OrderNotificationArgs) Kind() string { return "notifications.order_status" }
func (OrderNotificationArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: "notifications"}
}

// NotificationDeliverArgs defines job parameters for asynchronous notification dispatch.
type NotificationDeliverArgs struct {
	UserID             int64   `json:"user_id"`
	OrganizationID     *int64  `json:"organization_id,omitempty"`
	Channel            string  `json:"channel"`
	Recipient          string  `json:"recipient"`
	Title              string  `json:"title"`
	Body               string  `json:"body"`
	RequiredPermission string  `json:"required_permission,omitempty"`
	EventKey           string  `json:"event_key,omitempty"`
}

func (NotificationDeliverArgs) Kind() string { return "notifications.deliver" }
func (NotificationDeliverArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: "notifications", MaxAttempts: 3}
}


// IngestBatchArgs defines job parameters for chunked catalog row processing.
type IngestBatchArgs struct {
	SessionID      int64 `json:"session_id"`
	OrganizationID int64 `json:"organization_id"`
	Offset         int   `json:"offset"`
	BatchSize      int   `json:"batch_size"`
}

func (IngestBatchArgs) Kind() string { return "ingest.process_batch" }
func (IngestBatchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: "imports"}
}

// ExpirePromotionsArgs defines periodic maintenance parameters for expiring stale offers and sponsorships.
type ExpirePromotionsArgs struct {
	At time.Time `json:"at"`
}

func (ExpirePromotionsArgs) Kind() string { return "promo.expire_promotions" }
func (ExpirePromotionsArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: "maintenance"}
}

// ProductReindexArgs defines parameters for async catalog index synchronization.
type ProductReindexArgs struct {
	ProductID      int64 `json:"product_id,omitempty"` // 0 means full catalogue sweep
	OrganizationID int64 `json:"organization_id,omitempty"`
}

func (ProductReindexArgs) Kind() string { return "catalog.reindex" }
func (ProductReindexArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: "maintenance"}
}

// SmartOrderRunArgs defines job parameters for executing a smart ordering run.
//
// The job carries only identifiers. Everything else — the configuration, the
// staged rows — lives in the database, so a worker restart resumes from state
// rather than from a payload that may no longer be true.
type SmartOrderRunArgs struct {
	RunID          int64 `json:"run_id"`
	OrganizationID int64 `json:"organization_id"`
}

func (SmartOrderRunArgs) Kind() string { return "smartorder.run" }
func (SmartOrderRunArgs) InsertOpts() river.InsertOpts {
	// A ten-thousand-row import is minutes of work, so it gets its own queue
	// rather than blocking short jobs behind it. The pipeline is not yet
	// checkpointed per stage, so retrying this job would restart the entire AI
	// stage and multiply upstream requests after a transient failure.
	return river.InsertOpts{Queue: "smartorder", MaxAttempts: 1}
}

// ImportStageArgs defines job parameters for parsing and matching an uploaded
// file against the catalogue.  The job carries only identifiers; the file
// content, mapping choices and staged rows live in platform.import_runs /
// platform.import_run_rows.
//
// MaxAttempts is 1: a retry would re-run the AI matching stage and double-bill
// the tenant's credits.  A failed staging run is recoverable by re-uploading.
type ImportStageArgs struct {
	RunID          int64 `json:"run_id"`
	OrganizationID int64 `json:"organization_id"`
}

func (ImportStageArgs) Kind() string { return "imports.stage" }
func (ImportStageArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: "imports", MaxAttempts: 1}
}

// ImportCommitArgs defines job parameters for persisting the reviewed rows
// from a staged import run into their destination tables.
//
// MaxAttempts is 3: the commit uses ON CONFLICT (run_id, row_number) DO
// NOTHING on the child table and upserts on the target, so retries are
// idempotent.
type ImportCommitArgs struct {
	RunID          int64 `json:"run_id"`
	OrganizationID int64 `json:"organization_id"`
}

func (ImportCommitArgs) Kind() string { return "imports.commit" }
func (ImportCommitArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: "imports", MaxAttempts: 3}
}

// DocumentScanArgs defines job parameters for asynchronous file integrity and malware analysis.
type DocumentScanArgs struct {
	DocumentID     int64  `json:"document_id"`
	OrganizationID *int64 `json:"organization_id,omitempty"`
	StorageKey     string `json:"storage_key"`
	MimeType       string `json:"mime_type"`
}

func (DocumentScanArgs) Kind() string { return "security.document_scan" }
func (DocumentScanArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: "maintenance", MaxAttempts: 3}
}
