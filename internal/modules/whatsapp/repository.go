package whatsapp

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/chatbridge"
)

// Repository is this module's persistence.
type Repository interface {
	chatbridge.Store

	// Link codes.
	CountLinkTokensSince(ctx context.Context, userID int64, since time.Time) (int, error)
	CreateLinkToken(ctx context.Context, userID int64, orgID *int64, hash []byte, expires time.Time) error
	ConsumeLinkToken(ctx context.Context, hash []byte) (userID int64, orgID *int64, err error)

	// Links.
	LiveLinkByWAID(ctx context.Context, waID string) (*Link, error)
	CurrentLinkForUser(ctx context.Context, userID int64) (*Link, error)
	CreatePendingLink(ctx context.Context, l *Link, confirmBy time.Time) error
	ConfirmPendingLink(ctx context.Context, userID int64, publicID string) (*Link, error)
	RevokeLink(ctx context.Context, userID, linkID int64) error
	SetLinkStatus(ctx context.Context, linkID int64, status LinkStatus) error
	// TouchLink records an inbound message: it refreshes the display name and
	// opens the 24-hour window.
	TouchLink(ctx context.Context, linkID int64, displayName string) error
	MarkMessageProcessed(ctx context.Context, messageID string) (first bool, err error)

	// Outbox.
	NotificationCandidates(ctx context.Context, since time.Time, limit int) ([]chatbridge.Candidate, error)
	RecordDecisions(ctx context.Context, decisions []chatbridge.Decision) error
	EnqueueSystemMessage(ctx context.Context, linkID int64, text string) error
	ClaimDeliveries(ctx context.Context, limit int, lease time.Duration) ([]LeasedDelivery, error)
	// DropDelivery gives up on a leased delivery that can never be sent.
	DropDelivery(ctx context.Context, id int64, reason string) error
	MarkDelivered(ctx context.Context, id int64) error
	// RetryDelivery requeues a leased delivery, or fails it once attempts
	// reach maxAttempts. A zero retryAfter backs off exponentially.
	RetryDelivery(ctx context.Context, id int64, errText string, retryAfter time.Duration, maxAttempts int) error
	// CloseWindowAndRetry records that WhatsApp says the 24-hour window is
	// closed, and requeues the delivery so the next claim sends a template.
	CloseWindowAndRetry(ctx context.Context, id int64, errText string, maxAttempts int) error
	FailDeliveryAndBlockLink(ctx context.Context, id int64, errText string) error
	PurgeProcessedMessages(ctx context.Context, olderThan time.Time) error
}

// LeasedDelivery is one claimed outbox row, before it becomes a payload.
type LeasedDelivery struct {
	ID   int64
	WAID string
	Text string
	// WindowOpen reports that the user wrote within the last 24 hours (less a
	// safety margin), so a free-form message is accepted.
	WindowOpen bool
}
