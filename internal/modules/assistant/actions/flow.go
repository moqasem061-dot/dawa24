package actions

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// Status is a pending action's lifecycle.
type Status string

const (
	StatusPending   Status = "pending"
	StatusExecuting Status = "executing"
	StatusExecuted  Status = "executed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
	StatusExpired   Status = "expired"
	StatusStale     Status = "stale"
)

// Channel is where an action was proposed, and so where it is confirmed.
type Channel string

const (
	ChannelWeb      Channel = "web"
	ChannelTelegram Channel = "telegram"
	ChannelWhatsApp Channel = "whatsapp"
)

// Turn is where a proposal comes from: the conversation and the interface.
type Turn struct {
	ConversationID int64
	Channel        Channel
}

type turnKey struct{}

// WithTurn marks a turn's context so a proposal made inside it records its
// conversation and the interface it must be confirmed on.
func WithTurn(ctx context.Context, t Turn) context.Context {
	return context.WithValue(ctx, turnKey{}, t)
}

// TurnFrom reads the turn a context belongs to.
func TurnFrom(ctx context.Context) Turn {
	t, _ := ctx.Value(turnKey{}).(Turn)
	return t
}

// TTL is how long a proposal waits for its confirmation.
const TTL = 10 * time.Minute

// Pending is a stored proposal.
type Pending struct {
	ID             int64
	PublicID       uuid.UUID
	OrganizationID int64
	UserID         int64
	Scope          string
	ConversationID int64
	Channel        Channel
	Action         string
	Risk           Risk
	Args           Args
	Preview        Preview
	PreviewHash    []byte
	Status         Status
	Outcome        *Outcome
	Error          string
	ExpiresAt      time.Time
	CreatedAt      time.Time
}

// Store persists proposals. Implemented by assistant/postgres.
type Store interface {
	CreatePendingAction(ctx context.Context, p *Pending) error
	// GetPendingAction loads a proposal only for its own user and organisation.
	GetPendingAction(ctx context.Context, publicID uuid.UUID, userID, orgID int64) (*Pending, error)
	// ClaimPendingAction moves pending → executing once, and only before expiry.
	ClaimPendingAction(ctx context.Context, id int64) (bool, error)
	FinishPendingAction(ctx context.Context, id int64, status Status, outcome *Outcome, errText string) error
	// DecidePendingAction moves pending → cancelled/expired/stale.
	DecidePendingAction(ctx context.Context, id int64, status Status) error
}

// Gate reports whether the caller may act through the assistant at all: the
// assistant grant and the owner's separate "act" grant.
type Gate func(actor authctx.Actor) bool

// Flow runs propose, confirm and cancel.
type Flow struct {
	store Store
	exec  Executor
	gate  Gate
	now   func() time.Time
	log   *slog.Logger
}

// NewFlow wires the action flow.
func NewFlow(store Store, exec Executor, gate Gate, log *slog.Logger) *Flow {
	if log == nil {
		log = slog.Default()
	}
	return &Flow{store: store, exec: exec, gate: gate, now: time.Now, log: log.With("component", "capsule_actions")}
}

// Errors the interfaces map to user-facing answers.
var (
	ErrNotAllowed = errors.New("actions: not allowed")
	ErrNotFound   = errors.New("actions: not found")
	ErrExpired    = errors.New("actions: expired")
	ErrDecided    = errors.New("actions: already decided")
)

// ErrStale is returned when the data changed between proposal and confirm.
type ErrStale struct{ Preview Preview }

func (e *ErrStale) Error() string { return "actions: preview changed" }

// Available lists the actions this caller may propose now.
func (f *Flow) Available(actor authctx.Actor) []Definition {
	if f == nil || f.exec == nil || !f.gate(actor) {
		return nil
	}
	var out []Definition
	for _, d := range f.exec.Definitions(actor) {
		if f.exec.Permitted(actor, d.Name) {
			out = append(out, d)
		}
	}
	return out
}

func (f *Flow) definition(actor authctx.Actor, name string) (Definition, bool) {
	for _, d := range f.Available(actor) {
		if d.Name == name {
			return d, true
		}
	}
	return Definition{}, false
}

// ProposeRequest is one proposal from a conversation.
type ProposeRequest struct {
	Action         string
	Args           json.RawMessage
	ConversationID int64
	Channel        Channel
	Resolve        func(handles.Kind, string) (int64, error)
}

// Propose validates, authorizes and previews an action, and stores it. It
// changes no business data.
func (f *Flow) Propose(ctx context.Context, actor authctx.Actor, req ProposeRequest) (*Pending, error) {
	def, ok := f.definition(actor, req.Action)
	if !ok {
		return nil, ErrNotAllowed
	}
	args, err := Decode(def, req.Args, req.Resolve)
	if err != nil {
		return nil, err
	}
	preview, err := f.exec.Prepare(ctx, actor, def.Name, args)
	if err != nil {
		return nil, err
	}
	hash, err := previewHash(preview)
	if err != nil {
		return nil, err
	}
	channel := req.Channel
	if channel == "" {
		channel = ChannelWeb
	}
	p := &Pending{
		OrganizationID: actor.OrgID,
		UserID:         actor.UserID,
		Scope:          string(actor.DashboardScope()),
		ConversationID: req.ConversationID,
		Channel:        channel,
		Action:         def.Name,
		Risk:           def.Risk,
		Args:           args,
		Preview:        preview,
		PreviewHash:    hash,
		Status:         StatusPending,
		ExpiresAt:      f.now().Add(TTL),
	}
	if err := f.store.CreatePendingAction(ctx, p); err != nil {
		return nil, fmt.Errorf("actions: store proposal: %w", err)
	}
	return p, nil
}

// Confirm executes a proposal for the user it belongs to.
//
// Every check is repeated against the live actor, because the person who
// confirms is authenticated by a different request than the one that proposed:
// the assistant gate, the act grant, the action's own permission, the
// executor's full authorization, and the preview itself.
func (f *Flow) Confirm(ctx context.Context, actor authctx.Actor, publicID uuid.UUID) (*Pending, error) {
	p, err := f.load(ctx, actor, publicID)
	if err != nil {
		return nil, err
	}
	if p.Scope != string(actor.DashboardScope()) {
		return nil, ErrNotFound
	}
	if _, ok := f.definition(actor, p.Action); !ok {
		return nil, ErrNotAllowed
	}

	claimed, err := f.store.ClaimPendingAction(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	if !claimed {
		return nil, ErrDecided
	}

	preview, err := f.exec.Prepare(ctx, actor, p.Action, p.Args)
	if err != nil {
		f.finish(ctx, p, StatusFailed, nil, err)
		return p, err
	}
	hash, err := previewHash(preview)
	if err != nil {
		f.finish(ctx, p, StatusFailed, nil, err)
		return p, err
	}
	if string(hash) != string(p.PreviewHash) {
		f.finish(ctx, p, StatusStale, nil, nil)
		return p, &ErrStale{Preview: preview}
	}

	outcome, err := f.exec.Execute(ctx, actor, p.Action, p.Args)
	if err != nil {
		f.finish(ctx, p, StatusFailed, nil, err)
		return p, err
	}
	f.finish(ctx, p, StatusExecuted, &outcome, nil)
	return p, nil
}

// Cancel withdraws a proposal.
func (f *Flow) Cancel(ctx context.Context, actor authctx.Actor, publicID uuid.UUID) (*Pending, error) {
	p, err := f.load(ctx, actor, publicID)
	if err != nil {
		return nil, err
	}
	if err := f.store.DecidePendingAction(ctx, p.ID, StatusCancelled); err != nil {
		return nil, err
	}
	p.Status = StatusCancelled
	return p, nil
}

// Get returns a proposal for its own user, with expiry applied.
func (f *Flow) Get(ctx context.Context, actor authctx.Actor, publicID uuid.UUID) (*Pending, error) {
	p, err := f.store.GetPendingAction(ctx, publicID, actor.UserID, actor.OrgID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrNotFound
	}
	if p.Status == StatusPending && !f.now().Before(p.ExpiresAt) {
		p.Status = StatusExpired
	}
	return p, nil
}

func (f *Flow) load(ctx context.Context, actor authctx.Actor, publicID uuid.UUID) (*Pending, error) {
	if f == nil || actor.UserID <= 0 || !f.gate(actor) {
		return nil, ErrNotAllowed
	}
	p, err := f.Get(ctx, actor, publicID)
	if err != nil {
		return nil, err
	}
	switch p.Status {
	case StatusPending:
		return p, nil
	case StatusExpired:
		_ = f.store.DecidePendingAction(ctx, p.ID, StatusExpired)
		return nil, ErrExpired
	}
	return nil, ErrDecided
}

func (f *Flow) finish(ctx context.Context, p *Pending, status Status, outcome *Outcome, cause error) {
	errText := ""
	if cause != nil {
		if r, ok := AsRefusal(cause); ok {
			errText = r.Message
		} else {
			errText = "execution failed"
			f.log.ErrorContext(ctx, "assistant action failed", "action", p.Action, "user_id", p.UserID, "error", cause)
		}
	}
	p.Status, p.Outcome, p.Error = status, outcome, errText
	// Detached: the outcome must be recorded even when the request that
	// confirmed it has gone.
	if err := f.store.FinishPendingAction(context.WithoutCancel(ctx), p.ID, status, outcome, errText); err != nil {
		f.log.ErrorContext(ctx, "assistant action outcome not recorded", "action", p.Action, "id", p.ID, "error", err)
	}
	f.log.InfoContext(ctx, "assistant action decided", "action", p.Action, "status", status,
		"user_id", p.UserID, "org_id", p.OrganizationID, "channel", p.Channel)
}

func previewHash(p Preview) ([]byte, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(b)
	return sum[:], nil
}
