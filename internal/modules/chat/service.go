package chat

import (
	"context"
	"log/slog"

	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// Service coordinates messaging between organizations.
type Service struct {
	repo Repository
	log  *slog.Logger
}

// NewService creates a messaging service.
func NewService(repo Repository, log *slog.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// StartConversation opens a thread between the caller's organization and a
// counterparty, recording the creator as the first participant.
func (s *Service) StartConversation(ctx context.Context, orgID, counterpartyOrgID, creatorUserID int64, subject i18n.Text, contextType ContextType, contextID *int64) (*Conversation, error) {
	c := &Conversation{
		OrganizationID:    orgID,
		CounterpartyOrgID: counterpartyOrgID,
		Subject:           subject,
		ContextType:       contextType,
		ContextID:         contextID,
		Status:            StatusOpen,
		CreatedByUserID:   creatorUserID,
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if c.ContextType == "" {
		c.ContextType = ContextGeneral
	}
	if err := s.repo.CreateConversation(ctx, c); err != nil {
		return nil, err
	}
	_ = s.repo.AddParticipant(ctx, c.ID, creatorUserID, orgID)
	s.log.InfoContext(ctx, "conversation started", "conversation_id", c.ID, "org", orgID, "counterparty", counterpartyOrgID)
	return c, nil
}

// SendMessage appends a message to a conversation.
func (s *Service) SendMessage(ctx context.Context, conversationID, senderUserID int64, senderOrgID *int64, body string) (*Message, error) {
	m := &Message{
		ConversationID: conversationID,
		SenderUserID:   senderUserID,
		SenderOrgID:    senderOrgID,
		Body:           body,
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	// The conversation is addressed by an id from the URL. Only its two
	// organizations may post in it; otherwise anyone signed in could write
	// into another pair's thread, and row-level security is the only thing that
	// stopped it — which a connection that bypasses RLS does not apply.
	conv, err := s.repo.GetConversationByID(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if conv == nil || senderOrgID == nil ||
		(*senderOrgID != conv.OrganizationID && *senderOrgID != conv.CounterpartyOrgID) {
		return nil, ErrNoConversation
	}
	if err := s.repo.SendMessage(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

// ListConversations returns the caller's threads, newest first.
func (s *Service) ListConversations(ctx context.Context, orgID int64, limit, offset int) ([]*Conversation, error) {
	return s.repo.ListConversationsForOrg(ctx, orgID, limit, offset)
}

// GetThread returns a conversation's messages oldest-first.
func (s *Service) GetThread(ctx context.Context, conversationID int64, limit int) ([]*Message, error) {
	return s.repo.ListMessages(ctx, conversationID, limit)
}

// MarkRead clears unread state for the caller's side of a conversation.
func (s *Service) MarkRead(ctx context.Context, conversationID, orgID int64) error {
	return s.repo.MarkConversationRead(ctx, conversationID, orgID)
}

// UnreadCount returns how many unread conversations the org has.
func (s *Service) UnreadCount(ctx context.Context, orgID int64) (int, error) {
	if orgID <= 0 {
		return 0, nil
	}
	return s.repo.CountUnread(ctx, orgID)
}

// ErrNoConversation is returned when a referenced thread does not exist.
var ErrNoConversation = apperr.NotFound("conversation")
