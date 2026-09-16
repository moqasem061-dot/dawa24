// Package notifications manages multi-channel communications (SMS, WhatsApp, Email, In-App),
// message templates, and user notification inboxes.
package notifications

import (
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Channel specifies the delivery pathway.
type Channel string

const (
	ChannelSMS      Channel = "sms"
	ChannelWhatsApp Channel = "whatsapp"
	ChannelEmail    Channel = "email"
	ChannelInApp    Channel = "in_app"
)

// DeliveryStatus tracks delivery state.
type DeliveryStatus string

const (
	StatusPending   DeliveryStatus = "pending"
	StatusSent      DeliveryStatus = "sent"
	StatusDelivered DeliveryStatus = "delivered"
	StatusFailed    DeliveryStatus = "failed"
)

// Template represents a parameterized notification message definition.
type Template struct {
	ID        int64     `json:"id"`
	PublicID  string    `json:"public_id"`
	Slug      string    `json:"slug"`
	Channel   Channel   `json:"channel"`
	Title     i18n.Text `json:"title"`
	Body      i18n.Text `json:"body"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NotificationLog is a dispatched notification record.
type NotificationLog struct {
	ID                 int64          `json:"id"`
	PublicID           string         `json:"public_id"`
	UserID             int64          `json:"user_id"`
	OrganizationID     *int64         `json:"organization_id,omitempty"`
	BranchID           *int64         `json:"branch_id,omitempty"`
	Channel            Channel        `json:"channel"`
	Recipient          string         `json:"recipient"`
	Title              string         `json:"title"`
	Body               string         `json:"body"`
	RequiredPermission string         `json:"required_permission,omitempty"`
	Status             DeliveryStatus `json:"status"`
	ErrorMessage       string         `json:"error_message,omitempty"`
	IsRead             bool           `json:"is_read"`
	ReadAt             *time.Time     `json:"read_at,omitempty"`
	SentAt             *time.Time     `json:"sent_at,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
}

// InterpolateTemplate replaces {variable} tokens in text templates.
func InterpolateTemplate(text string, vars map[string]string) string {
	result := text
	for k, v := range vars {
		placeholder := "{" + k + "}"
		result = strings.ReplaceAll(result, placeholder, v)
	}
	return result
}

// Validate ensures non-empty required fields.
func (n *NotificationLog) Validate() error {
	if n.UserID <= 0 {
		return apperr.Validation("notification.user_required", "User ID is required.", nil)
	}
	if n.Recipient == "" {
		return apperr.Validation("notification.recipient_required", "Recipient address or phone number is required.", nil)
	}
	if n.Body == "" {
		return apperr.Validation("notification.body_required", "Notification message body is required.", nil)
	}
	return nil
}
