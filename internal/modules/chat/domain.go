// Package chat implements buyer<->supplier messaging threads.
package chat

import (
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// ContextType classifies what a conversation is about.
type ContextType string

const (
	ContextOrder   ContextType = "order"
	ContextQuote   ContextType = "quote"
	ContextProduct ContextType = "product"
	ContextGeneral ContextType = "general"
)

// ConversationStatus is the lifecycle state of a thread.
type ConversationStatus string

const (
	StatusOpen   ConversationStatus = "open"
	StatusClosed ConversationStatus = "closed"
)

// Conversation is a thread between two organizations.
type Conversation struct {
	ID                int64              `json:"id"`
	PublicID          string             `json:"public_id"`
	OrganizationID    int64              `json:"organization_id"`
	CounterpartyOrgID int64              `json:"counterparty_org_id"`
	Subject           i18n.Text          `json:"subject"`
	ContextType       ContextType        `json:"context_type"`
	ContextID         *int64             `json:"context_id,omitempty"`
	Status            ConversationStatus `json:"status"`
	LastMessageAt     *time.Time         `json:"last_message_at,omitempty"`
	CreatedByUserID   int64              `json:"created_by_user_id"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
}

// Message is one message in a conversation.
type Message struct {
	ID             int64            `json:"id"`
	ConversationID int64            `json:"conversation_id"`
	SenderUserID   int64            `json:"sender_user_id"`
	SenderOrgID    *int64           `json:"sender_org_id,omitempty"`
	Body           string           `json:"body"`
	Attachments    []map[string]any `json:"attachments,omitempty"`
	ReadAt         *time.Time       `json:"read_at,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
}

// Validate ensures a message has content and a sender.
func (m *Message) Validate() error {
	if m.SenderUserID <= 0 {
		return apperr.Validation("chat.sender_required", "A sender is required.", nil)
	}
	if m.Body == "" && len(m.Attachments) == 0 {
		return apperr.Validation("chat.body_required", "A message needs text or an attachment.", nil)
	}
	return nil
}

// Validate ensures a conversation names both sides.
func (c *Conversation) Validate() error {
	if c.OrganizationID <= 0 || c.CounterpartyOrgID <= 0 {
		return apperr.Validation("chat.org_required", "Both conversation parties are required.", nil)
	}
	if c.OrganizationID == c.CounterpartyOrgID {
		return apperr.Validation("chat.same_org", "Cannot start a conversation with your own organization.", nil)
	}
	return nil
}
