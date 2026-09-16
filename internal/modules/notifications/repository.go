package notifications

import (
	"context"
	"time"
)

// Repository defines storage operations for notification templates and message logs.
type Repository interface {
	CreateLog(ctx context.Context, log *NotificationLog) error
	GetTemplateBySlug(ctx context.Context, slug string) (*Template, error)
	ListUserNotifications(ctx context.Context, userID int64, limit, offset int) ([]*NotificationLog, error)
	MarkAsRead(ctx context.Context, id int64, userID int64) error
	// MarkAllAsRead clears the whole feed in one statement rather than making
	// the client issue one request per notification.
	MarkAllAsRead(ctx context.Context, userID int64) (int64, error)
	ListUnread(ctx context.Context, userID int64, limit, offset int) ([]*NotificationLog, error)
	GetUnreadCount(ctx context.Context, userID int64) (int, error)
	HasNotificationWithTitle(ctx context.Context, userID int64, title string, since time.Time) (bool, error)
}

