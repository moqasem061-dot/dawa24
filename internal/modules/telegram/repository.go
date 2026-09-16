package telegram

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
	LiveLinkByTelegramUser(ctx context.Context, telegramUserID int64) (*Link, error)
	CurrentLinkForUser(ctx context.Context, userID int64) (*Link, error)
	CreatePendingLink(ctx context.Context, l *Link, confirmBy time.Time) error
	ConfirmPendingLink(ctx context.Context, userID int64, publicID string) (*Link, error)
	RevokeLink(ctx context.Context, userID, linkID int64) error
	SetLinkStatus(ctx context.Context, linkID int64, status LinkStatus) error
	TouchLink(ctx context.Context, linkID, chatID int64, username, displayName string) error
	MarkUpdateProcessed(ctx context.Context, updateID int64) (first bool, err error)

	// Outbox.
	NotificationCandidates(ctx context.Context, since time.Time, limit int) ([]chatbridge.Candidate, error)
	RecordDecisions(ctx context.Context, decisions []chatbridge.Decision) error
	EnqueueSystemMessage(ctx context.Context, linkID int64, text string) error
	ClaimDeliveries(ctx context.Context, limit int, lease time.Duration) ([]Outgoing, error)
	MarkDelivered(ctx context.Context, id int64) error
	// RetryDelivery requeues a leased delivery, or fails it once attempts
	// reach maxAttempts. A zero retryAfter backs off exponentially.
	RetryDelivery(ctx context.Context, id int64, errText string, retryAfter time.Duration, maxAttempts int) error
	FailDeliveryAndBlockLink(ctx context.Context, id int64, errText string) error
	PurgeProcessedUpdates(ctx context.Context, olderThan time.Time) error
}
