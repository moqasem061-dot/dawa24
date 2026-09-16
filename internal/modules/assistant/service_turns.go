package assistant

import (
	"context"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/assistant/actions"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// Opening a turn, shared by every interface.
//
// The browser drawer and the Telegram bot are two front doors to one
// assistant. Which conversation a question belongs to, and the turn record
// that audits it, are decided here once, so neither door can be looser about
// ownership than the other.

// MaxQuestionBytes bounds one question on every interface. Long enough to
// paste a shortage list, short enough that nobody can push a megabyte of text
// into a prompt.
const MaxQuestionBytes = 8000

// OpenConversation returns the conversation a question belongs to, creating
// one when conversationID is zero.
//
// A supplied id is accepted only when it belongs to this caller AND was
// created by the agent they are using now. Deleted, expired, someone else's
// and another dashboard's all come back as CodeNotFound — the caller is told
// the same thing in every case.
func (s *Service) OpenConversation(
	ctx context.Context, actor authctx.Actor, cfg AgentConfig, conversationID int64, question string,
) (*Conversation, *Failure) {
	if conversationID > 0 {
		conv, err := s.repo.GetOwnedConversation(
			ctx, conversationID, actor.OrgID, actor.UserID, string(cfg.Role))
		if err != nil {
			s.log.ErrorContext(ctx, "assistant: load conversation", "error", err)
			f := Fail(CodeInternal)
			return nil, &f
		}
		if conv == nil {
			f := Fail(CodeNotFound)
			return nil, &f
		}
		return conv, nil
	}

	conv := &Conversation{
		OrganizationID: actor.OrgID,
		UserID:         actor.UserID,
		Title:          TitleFor(question),
		AgentRole:      string(cfg.Role),
	}
	if err := s.repo.CreateConversation(ctx, conv); err != nil {
		s.log.ErrorContext(ctx, "assistant: create conversation", "error", err)
		f := Fail(CodeInternal)
		return nil, &f
	}
	return conv, nil
}

// BeginTurn records a running turn in a conversation the caller owns.
func (s *Service) BeginTurn(
	ctx context.Context, actor authctx.Actor, cfg AgentConfig, conv *Conversation, question string,
) (*Turn, error) {
	turn := &Turn{
		ConversationID: conv.ID,
		OrganizationID: actor.OrgID,
		UserID:         actor.UserID,
		AgentRole:      string(cfg.Role),
		Status:         TurnRunning,
		Question:       question,
	}
	if err := s.repo.CreateTurn(ctx, turn); err != nil {
		return nil, err
	}
	return turn, nil
}

// AskResult is one finished turn for an interface that cannot stream.
type AskResult struct {
	Answer         string
	ConversationID int64
	Entities       []Entity
	// Code is empty on success. On failure Answer may still hold the part of
	// the answer written before it stopped.
	Code Code
}

// Ask answers one question to completion and returns the result.
//
// It is RunTurn — the same loop, the same tools, the same persistence — with a
// collector in place of the browser's stream. The gate is re-applied here
// rather than trusted from the caller, so an interface that forgot to check
// still cannot reach the loop for an actor the browser would refuse.
//
// A conversationID that no longer resolves (expired, deleted, or created under
// a role the user no longer holds) starts a fresh conversation instead of
// failing: a chat interface has no drawer in which to pick another thread.
func (s *Service) Ask(ctx context.Context, actor authctx.Actor, channel actions.Channel, conversationID int64, question string, attachmentRefs ...string) AskResult {
	cfg, ok := Allowed(actor)
	if !ok || actor.UserID <= 0 {
		return AskResult{Code: CodeForbidden}
	}
	question = strings.TrimSpace(question)
	if len(question) > MaxQuestionBytes {
		question = strings.ToValidUTF8(question[:MaxQuestionBytes], "")
	}
	if question == "" && len(attachmentRefs) == 0 {
		return AskResult{Code: CodeInvalidRequest}
	}

	convTitle := question
	if convTitle == "" && len(attachmentRefs) > 0 {
		convTitle = defaultAttachmentAsk
	}

	conv, failure := s.OpenConversation(ctx, actor, cfg, conversationID, convTitle)
	if failure != nil && failure.Code == CodeNotFound {
		conv, failure = s.OpenConversation(ctx, actor, cfg, 0, convTitle)
	}
	if failure != nil {
		return AskResult{Code: failure.Code}
	}

	turn, err := s.BeginTurn(ctx, actor, cfg, conv, convTitle)
	if err != nil {
		s.log.ErrorContext(ctx, "assistant: create turn", "error", err)
		return AskResult{Code: CodeInternal, ConversationID: conv.ID}
	}

	atts, digests, parts := s.ResolveAttachments(ctx, actor, attachmentRefs)

	c := &collector{}
	s.RunTurn(ctx, actor, cfg, turn, TurnInput{
		Text:        question,
		Attachments: atts,
		Digests:     digests,
		Parts:       parts,
		Channel:     channel,
	}, c)
	return AskResult{
		Answer:         c.answer,
		ConversationID: conv.ID,
		Entities:       c.entities,
		Code:           c.code,
	}
}

// collector is an Emitter that keeps only what a finished answer needs.
type collector struct {
	answer   string
	entities []Entity
	code     Code
}

func (c *collector) Delta(string)                  {}
func (c *collector) Reasoning(string)              {}
func (c *collector) Status(string, map[string]any) {}
func (c *collector) Usage(int, int)                {}
func (c *collector) Entities(list []Entity)        { c.entities = list }
func (c *collector) Done(answer string, _ int64)   { c.answer = answer }
func (c *collector) Failed(code Code, partial string, _ int64) {
	c.code = code
	c.answer = partial
}
