package assistant

import (
	"context"
	"time"
)

// Repository is the assistant's own persistence: conversations, turns,
// messages, attachments and the tool audit trail.
//
// Business data is NOT here. That lives behind Reader (access.go), which is
// read-only by construction. Keeping the two apart means the only writes the
// assistant can perform are writes about itself.
type Repository interface {
	// Conversations.
	CreateConversation(ctx context.Context, c *Conversation) error
	// GetConversation fetches by primary key with no ownership check. It exists
	// for the platform-admin conversation log, which is separately gated on an
	// admin permission. Request paths must use GetOwnedConversation instead.
	GetConversation(ctx context.Context, id int64) (*Conversation, error)
	// GetOwnedConversation fetches a conversation only when it belongs to this
	// caller AND was created by the same agent they are using now.
	//
	// This is the fix for the hole that let any signed-in user post another
	// tenant's conversation id and have that history read back to them. The
	// predicate is in the WHERE clause, so a foreign id returns nothing rather
	// than being fetched and then compared.
	GetOwnedConversation(ctx context.Context, id, orgID, userID int64, agentRole string) (*Conversation, error)
	GetConversationSummary(ctx context.Context, id int64) (*ConversationSummary, error)
	ListConversations(ctx context.Context, orgID, userID int64, limit, offset int) ([]*Conversation, error)
	ListAllConversations(ctx context.Context, search string, limit, offset int) ([]*ConversationSummary, int, error)
	ListAllConversationsFiltered(ctx context.Context, f AdminConversationFilter) ([]*ConversationSummary, int, error)
	GetAssistantStats(ctx context.Context) (*AssistantStats, error)
	DeleteConversation(ctx context.Context, id int64, orgID, userID int64) error

	// Messages.
	SaveMessage(ctx context.Context, m *Message) error
	ListMessages(ctx context.Context, convID int64, limit int) ([]*Message, error)
	// ListRecentMessages returns the NEWEST messages, oldest-first within the
	// window. ListMessages returns the oldest, which is right for showing a
	// conversation from the top and wrong for building a prompt: a long thread
	// was feeding the model its opening and dropping everything since.
	ListRecentMessages(ctx context.Context, convID int64, limit int) ([]*Message, error)

	// Turns.
	CreateTurn(ctx context.Context, t *Turn) error
	FinishTurn(ctx context.Context, t *Turn) error
	GetTurn(ctx context.Context, publicID string, orgID, userID int64) (*Turn, error)
	LatestRunningTurn(ctx context.Context, convID, userID int64) (*Turn, error)

	// Attachments.
	CreateAttachment(ctx context.Context, a *AttachmentRow) error
	GetAttachment(ctx context.Context, publicID string, orgID, userID int64) (*AttachmentRow, error)
	SetAttachmentDigest(ctx context.Context, id int64, digest string) error
	// SaveAttachmentContent and LoadAttachmentContent are the fallback for a
	// deployment with no reachable object storage. See postgres/blobs.go: an
	// unconfigured bucket used to make every upload fail, which from the user's
	// side was indistinguishable from the assistant refusing their photograph.
	SaveAttachmentContent(ctx context.Context, id int64, content []byte) error
	LoadAttachmentContent(ctx context.Context, id int64) ([]byte, error)
	MarkAttachmentsReferenced(ctx context.Context, ids []int64, convID int64) error

	// Audit.
	RecordToolCall(ctx context.Context, entry ToolAudit)

	// Retention. Conversations are deleted six months after they were created;
	// unreferenced attachments after a day.
	PurgeExpiredConversations(ctx context.Context, now time.Time) (int, error)
	PurgeOrphanAttachments(ctx context.Context, olderThan time.Time) ([]string, error)

	// Memories (Organization & User persistent knowledge).
	SaveMemory(ctx context.Context, m *Memory) error
	UpdateMemory(ctx context.Context, orgID, id int64, content string, category MemoryCategory) error
	DeleteMemory(ctx context.Context, orgID, id int64) error
	ListMemories(ctx context.Context, orgID int64, userID *int64, limit int) ([]*Memory, error)
	FindMemories(ctx context.Context, orgID int64, query string, limit int) ([]*Memory, error)
}
