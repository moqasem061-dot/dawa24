package chat

import (
	"context"
)

// Repository defines the storage contract for messaging.
type Repository interface {
	CreateConversation(ctx context.Context, c *Conversation) error
	GetConversationByID(ctx context.Context, id int64) (*Conversation, error)
	ListConversationsForOrg(ctx context.Context, orgID int64, limit, offset int) ([]*Conversation, error)
	AddParticipant(ctx context.Context, conversationID, userID, orgID int64) error
	SendMessage(ctx context.Context, m *Message) error
	ListMessages(ctx context.Context, conversationID int64, limit int) ([]*Message, error)
	MarkConversationRead(ctx context.Context, conversationID, orgID int64) error
	CountUnread(ctx context.Context, orgID int64) (int, error)
}
