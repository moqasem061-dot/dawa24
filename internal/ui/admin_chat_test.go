package ui_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui"
)

type mockUIAssistantRepo struct {
	convs []*assistant.ConversationSummary
	msgs  []*assistant.Message
}

func (m *mockUIAssistantRepo) CreateConversation(_ context.Context, _ *assistant.Conversation) error {
	return nil
}
func (m *mockUIAssistantRepo) GetConversation(_ context.Context, _ int64) (*assistant.Conversation, error) {
	return nil, nil
}
func (m *mockUIAssistantRepo) GetConversationSummary(_ context.Context, id int64) (*assistant.ConversationSummary, error) {
	for _, c := range m.convs {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, nil
}
func (m *mockUIAssistantRepo) ListConversations(_ context.Context, _, _ int64, _, _ int) ([]*assistant.Conversation, error) {
	return nil, nil
}
func (m *mockUIAssistantRepo) ListAllConversationsFiltered(
	ctx context.Context, f assistant.AdminConversationFilter,
) ([]*assistant.ConversationSummary, int, error) {
	return m.ListAllConversations(ctx, f.Search, f.Limit, f.Offset)
}

func (m *mockUIAssistantRepo) ListAllConversations(_ context.Context, search string, limit, offset int) ([]*assistant.ConversationSummary, int, error) {
	return m.convs, len(m.convs), nil
}
func (m *mockUIAssistantRepo) GetAssistantStats(_ context.Context) (*assistant.AssistantStats, error) {
	return &assistant.AssistantStats{
		TotalConversations: len(m.convs),
		TotalMessages:      len(m.msgs),
		ActiveUsers:        1,
	}, nil
}
func (m *mockUIAssistantRepo) DeleteConversation(_ context.Context, _ int64, _, _ int64) error {
	return nil
}
func (m *mockUIAssistantRepo) SaveMessage(_ context.Context, _ *assistant.Message) error { return nil }
func (m *mockUIAssistantRepo) ListMessages(_ context.Context, _ int64, _ int) ([]*assistant.Message, error) {
	return m.msgs, nil
}
func (m *mockUIAssistantRepo) GetOwnedConversation(_ context.Context, _, _, _ int64, _ string) (*assistant.Conversation, error) {
	return nil, nil
}
func (m *mockUIAssistantRepo) ListRecentMessages(_ context.Context, _ int64, _ int) ([]*assistant.Message, error) {
	return nil, nil
}
func (m *mockUIAssistantRepo) CreateTurn(_ context.Context, _ *assistant.Turn) error { return nil }
func (m *mockUIAssistantRepo) FinishTurn(_ context.Context, _ *assistant.Turn) error { return nil }
func (m *mockUIAssistantRepo) GetTurn(_ context.Context, _ string, _, _ int64) (*assistant.Turn, error) {
	return nil, nil
}
func (m *mockUIAssistantRepo) LatestRunningTurn(_ context.Context, _, _ int64) (*assistant.Turn, error) {
	return nil, nil
}
func (m *mockUIAssistantRepo) CreateAttachment(_ context.Context, _ *assistant.AttachmentRow) error {
	return nil
}
func (m *mockUIAssistantRepo) GetAttachment(_ context.Context, _ string, _, _ int64) (*assistant.AttachmentRow, error) {
	return nil, nil
}
func (m *mockUIAssistantRepo) SetAttachmentDigest(_ context.Context, _ int64, _ string) error {
	return nil
}
func (m *mockUIAssistantRepo) SaveAttachmentContent(_ context.Context, _ int64, _ []byte) error {
	return nil
}
func (m *mockUIAssistantRepo) LoadAttachmentContent(_ context.Context, _ int64) ([]byte, error) {
	return nil, nil
}
func (m *mockUIAssistantRepo) MarkAttachmentsReferenced(_ context.Context, _ []int64, _ int64) error {
	return nil
}
func (m *mockUIAssistantRepo) RecordToolCall(_ context.Context, _ assistant.ToolAudit) {}
func (m *mockUIAssistantRepo) PurgeExpiredConversations(_ context.Context, _ time.Time) (int, error) {
	return 0, nil
}
func (m *mockUIAssistantRepo) PurgeOrphanAttachments(_ context.Context, _ time.Time) ([]string, error) {
	return nil, nil
}
func (m *mockUIAssistantRepo) SaveMemory(_ context.Context, _ *assistant.Memory) error { return nil }
func (m *mockUIAssistantRepo) UpdateMemory(_ context.Context, _, _ int64, _ string, _ assistant.MemoryCategory) error {
	return nil
}
func (m *mockUIAssistantRepo) DeleteMemory(_ context.Context, _, _ int64) error { return nil }
func (m *mockUIAssistantRepo) ListMemories(_ context.Context, _ int64, _ *int64, _ int) ([]*assistant.Memory, error) {
	return nil, nil
}
func (m *mockUIAssistantRepo) FindMemories(_ context.Context, _ int64, _ string, _ int) ([]*assistant.Memory, error) {
	return nil, nil
}

func TestAdminChatTreeAndHistoryRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	mockRepo := &mockUIAssistantRepo{
		convs: []*assistant.ConversationSummary{
			{
				ID:                10,
				PublicID:          uuid.New(),
				OrganizationID:    1,
				OrganizationName:  "صيدلية الأمل",
				OrganizationType:  "customer",
				UserID:            5,
				UserName:          "د. أحمد صيدلي",
				UserEmail:         "ahmed@amal.com",
				Title:             "استفسار عن توافر الأدوية",
				MessageCount:      4,
				TotalInputTokens:  120,
				TotalOutputTokens: 250,
				CreatedAt:         time.Now().Add(-1 * time.Hour),
				UpdatedAt:         time.Now(),
			},
		},
		msgs: []*assistant.Message{
			{
				ID:             1,
				ConversationID: 10,
				Role:           "user",
				Content:        "هل يوجد بدائل لبنادول إكسترا؟",
				CreatedAt:      time.Now().Add(-55 * time.Minute),
			},
			{
				ID:             2,
				ConversationID: 10,
				Role:           "assistant",
				Content:        "نعم، تتوفر بدائل مثل أدول إكسترا وبارامول بلس.",
				CreatedAt:      time.Now().Add(-54 * time.Minute),
			},
		},
	}
	handler.SetAssistantRepository(mockRepo)

	r := chi.NewRouter()
	handler.RegisterAdminRoutes(r)

	tests := []struct {
		name       string
		path       string
		method     string
		actor      *authctx.Actor
		wantStatus int
	}{
		{
			name:       "Anonymous GET /admin/chat/history redirects to login",
			path:       "/admin/chat/history",
			method:     "GET",
			actor:      nil,
			wantStatus: http.StatusSeeOther,
		},
		{
			name:   "Super admin GET /admin/chat/history returns 200",
			path:   "/admin/chat/history",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/chat/history with search query returns 200",
			path:   "/admin/chat/history?q=panadol",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/chat/ai/10 returns 200",
			path:   "/admin/chat/ai/10",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Legacy GET /admin/chat/history/10 redirects to /admin/chat/ai/10",
			path:   "/admin/chat/history/10",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusMovedPermanently,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.actor != nil {
				ctx = authctx.WithActor(ctx, *tt.actor)
			}

			req, _ := http.NewRequestWithContext(ctx, tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("path %s expected status %d, got %d", tt.path, tt.wantStatus, rr.Code)
			}
		})
	}
}
