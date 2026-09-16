package chat

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

type mockRepo struct {
	conversation *Conversation
}

func (m *mockRepo) CreateConversation(_ context.Context, c *Conversation) error {
	c.ID = 1
	m.conversation = c
	return nil
}
func (m *mockRepo) GetConversationByID(_ context.Context, _ int64) (*Conversation, error) {
	return m.conversation, nil
}
func (m *mockRepo) ListConversationsForOrg(_ context.Context, _ int64, _, _ int) ([]*Conversation, error) {
	return nil, nil
}
func (m *mockRepo) AddParticipant(_ context.Context, _, _, _ int64) error { return nil }
func (m *mockRepo) SendMessage(_ context.Context, msg *Message) error {
	msg.ID = 1
	return nil
}
func (m *mockRepo) ListMessages(_ context.Context, _ int64, _ int) ([]*Message, error) {
	return nil, nil
}
func (m *mockRepo) MarkConversationRead(_ context.Context, _, _ int64) error { return nil }
func (m *mockRepo) CountUnread(_ context.Context, _ int64) (int, error)      { return 0, nil }

func newService() *Service {
	return NewService(&mockRepo{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestStartConversationRejectsSameOrg(t *testing.T) {
	svc := newService()
	_, err := svc.StartConversation(context.Background(), 1, 1, 10, i18n.New("استفسار", "Inquiry"), ContextGeneral, nil)
	if err == nil {
		t.Fatal("expected error starting a conversation with the same organization")
	}
}

func TestStartConversationDefaultsContextType(t *testing.T) {
	svc := newService()
	c, err := svc.StartConversation(context.Background(), 1, 2, 10, i18n.New("استفسار", "Inquiry"), "", nil)
	if err != nil {
		t.Fatalf("StartConversation failed: %v", err)
	}
	if c.ContextType != ContextGeneral {
		t.Fatalf("ContextType = %q, want %q", c.ContextType, ContextGeneral)
	}
}

func TestSendMessageRejectsEmpty(t *testing.T) {
	svc := newService()
	_, err := svc.SendMessage(context.Background(), 1, 10, nil, "")
	if err == nil {
		t.Fatal("expected error sending an empty message")
	}
}
