// Package chatbridge is what every chat channel bridge shares: Telegram and
// WhatsApp are transports over it.
//
// It holds the decisions that must not differ between channels — how a chat's
// user is rebuilt from the live RBAC resolver, which gates a question and a
// confirmation pass, and who is still owed a notification at delivery time —
// together with the port through which a bridge reaches the Capsule assistant.
// A channel package owns only its wire format, its link table and its
// delivery-error vocabulary.
package chatbridge

import (
	"context"
	"errors"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// ErrTokenInvalid reports a link code that is unknown, used or expired.
var ErrTokenInvalid = errors.New("chatbridge: link code is invalid, used or expired")

// GrantResolver is the platform's permission resolver. *rbac.Resolver
// satisfies it; nothing in a bridge computes a permission of its own.
type GrantResolver interface {
	Resolve(ctx context.Context, userID, orgID int64) (rbac.Grant, error)
}

// Answer is one finished assistant turn, in terms any chat channel can render.
type Answer struct {
	// Markdown is the answer text, or the part written before a failure.
	Markdown       string
	ConversationID int64
	// Links are dashboard records the answer names, as absolute URLs.
	Links []AnswerLink
	// Failure is the user-facing sentence when the turn did not finish; empty
	// on success.
	Failure string
	// Proposals are actions awaiting the user's confirmation.
	Proposals []AnswerProposal
	// Files are exports the turn produced.
	Files []AnswerFile
}

// AnswerLink is one record the answer referred to.
type AnswerLink struct {
	Title string
	URL   string
}

// AnswerProposal is an action awaiting confirmation.
type AnswerProposal struct {
	ID       string
	Title    string
	Summary  string
	Details  [][2]string
	Warnings []string
}

// AnswerFile is an export produced by the turn.
type AnswerFile struct {
	Name  string
	Token string
}

// ExportFile is a stored export's bytes, for a bridge to serve.
type ExportFile struct {
	UserID   int64
	Filename string
	MIMEType string
	Content  []byte
}

// ActionReply is the result of a decision, for the person who pressed.
type ActionReply struct {
	Message string
	// URL is a site-relative page showing the result.
	URL string
}

// Assistant is the Capsule assistant, seen from a chat channel.
//
// Implemented in cmd/server over assistant.Service, so no bridge imports that
// module. Allowed must be the very gate the browser endpoints use, and Ask
// re-applies it: an interface that forgot to ask still gets refused.
type Assistant interface {
	Allowed(actor authctx.Actor) bool
	// AllowQuestion is the per-user question rate limit, shared with the
	// browser drawer so no channel is a way around it.
	AllowQuestion(userID int64) bool
	Ask(ctx context.Context, actor authctx.Actor, conversationID int64, question string, attachmentRefs ...string) Answer
	// Decide confirms or cancels a proposal for the live actor, through the
	// same path the web drawer uses.
	Decide(ctx context.Context, actor authctx.Actor, confirm bool, proposalID string) ActionReply
	// Export loads a stored export by its download token.
	Export(ctx context.Context, token string) (*ExportFile, error)
	// IngestAttachment stores an uploaded or downloaded file and returns its reference handle.
	IngestAttachment(ctx context.Context, actor authctx.Actor, filename string, content []byte) (string, error)
}

// Membership is one organisation the user is an active member of.
type Membership struct {
	OrgID      int64
	Name       string
	Type       string
	Status     string
	BranchName string
}

// Candidate is one in-app notification that may be owed to a chat link.
type Candidate struct {
	LogID              int64
	LinkID             int64
	LinkActiveOrgID    int64
	MutedCategories    []string
	UserID             int64
	OrganizationID     int64
	OrganizationName   string
	Title              string
	Body               string
	RequiredPermission string
}

// Decision is the outcome recorded for one Candidate.
type Decision struct {
	LogID      int64
	LinkID     int64
	Category   Category
	Text       string
	DropReason string // empty: queue for delivery
}

// Store is the part of a channel's link persistence the shared flow uses.
// Every method is scoped by the link or user it names.
type Store interface {
	Memberships(ctx context.Context, userID int64) ([]Membership, error)
	OffersTopicEnabled(ctx context.Context, userID int64) (bool, error)
	SetActiveOrganization(ctx context.Context, linkID int64, orgID *int64) error
	SetConversation(ctx context.Context, linkID int64, conversationID *int64) error
	SetMutedCategories(ctx context.Context, linkID int64, muted []string) error
	AcquireBusy(ctx context.Context, linkID int64, until time.Time) (bool, error)
	ReleaseBusy(ctx context.Context, linkID int64) error
}

// Chat is the channel-neutral view of one confirmed link.
type Chat struct {
	LinkID          int64
	UserID          int64
	ActiveOrgID     *int64
	ConversationID  *int64
	MutedCategories []string
}

// OrgID returns the active organisation, zero for none.
func (c *Chat) OrgID() int64 {
	if c == nil || c.ActiveOrgID == nil {
		return 0
	}
	return *c.ActiveOrgID
}
