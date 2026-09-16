package main

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/actions"
	"github.com/muhiya/dawa24-store/internal/modules/chatbridge"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Chat channels as further doors to Capsule.
//
// The channel modules may not import the assistant module, so they meet it
// here. capsuleBridge hands every channel the very service the browser drawer
// uses — the same instance, with the same tool registry, gateway key resolver
// and rate limiter — rather than a second assistant assembled for a bot.

// capsuleBridge holds the mounted assistant; forChannel gives a channel its
// chatbridge.Assistant.
//
// It is bound after construction because the assistant is mounted inside the
// authenticated API group while the bridge routes sit outside it. Until it is
// bound, it refuses everything.
type capsuleBridge struct {
	mu            sync.RWMutex
	svc           *assistant.Service
	exports       exportLoader
	allowQuestion func(userID int64) bool
	baseURL       string
}

// capsuleChannel is the assistant as one chat channel reaches it. The channel
// is recorded on every proposal the channel's turns make.
type capsuleChannel struct {
	*capsuleBridge
	channel actions.Channel
}

var _ chatbridge.Assistant = capsuleChannel{}

func (c *capsuleBridge) forChannel(channel actions.Channel) capsuleChannel {
	return capsuleChannel{capsuleBridge: c, channel: channel}
}

func newCapsuleBridge(baseURL string) *capsuleBridge {
	return &capsuleBridge{baseURL: strings.TrimRight(baseURL, "/")}
}

// exportLoader reads stored exports; *assistant/postgres.Repository has it.
type exportLoader interface {
	LoadExport(ctx context.Context, token string) (*assistant.Export, error)
}

func (c *capsuleBridge) bind(svc *assistant.Service, exports exportLoader, allowQuestion func(int64) bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.svc, c.exports, c.allowQuestion = svc, exports, allowQuestion
	c.mu.Unlock()
}

func (c *capsuleBridge) service() (*assistant.Service, func(int64) bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.svc, c.allowQuestion
}

// Allowed is assistant.Allowed — the gate requireAssistant applies to every
// browser assistant route.
func (c *capsuleBridge) Allowed(actor authctx.Actor) bool {
	svc, _ := c.service()
	_, ok := assistant.Allowed(actor)
	return svc != nil && ok
}

func (c *capsuleBridge) AllowQuestion(userID int64) bool {
	_, allow := c.service()
	return allow != nil && allow(userID)
}

func (c capsuleChannel) Ask(ctx context.Context, actor authctx.Actor, conversationID int64, question string, attachmentRefs ...string) chatbridge.Answer {
	svc, _ := c.service()
	if svc == nil {
		return chatbridge.Answer{Failure: assistant.Fail(assistant.CodeGatewayUnavailable).Message}
	}
	res := svc.Ask(ctx, actor, c.channel, conversationID, question, attachmentRefs...)
	ans := chatbridge.Answer{Markdown: res.Answer, ConversationID: res.ConversationID}
	if res.Code != "" {
		ans.Failure = assistant.Fail(res.Code).Message
	}
	// Entity URLs are built by the assistant from rows it read, for the
	// caller's own dashboard. Only site-relative paths are accepted, so
	// nothing can turn a reference into a link to somewhere else.
	ans.Proposals, ans.Files = chatExtras(res.Entities)
	for _, e := range res.Entities {
		if e.Kind == assistant.EntityProposal || e.Kind == assistant.EntityExport {
			continue
		}
		if !strings.HasPrefix(e.URL, "/") || strings.HasPrefix(e.URL, "//") || c.baseURL == "" {
			continue
		}
		title := e.Title
		if title == "" {
			title = e.Label
		}
		ans.Links = append(ans.Links, chatbridge.AnswerLink{Title: title, URL: c.baseURL + e.URL})
	}
	return ans
}

// Decide confirms or cancels a proposal through the drawer's own path.
func (c *capsuleBridge) Decide(ctx context.Context, actor authctx.Actor, confirm bool, proposalID string) chatbridge.ActionReply {
	svc, _ := c.service()
	id, err := uuid.Parse(proposalID)
	if svc == nil || err != nil {
		return chatbridge.ActionReply{Message: "هذا الإجراء غير موجود."}
	}
	var res assistant.ActionResult
	if confirm {
		res = svc.ConfirmAction(ctx, actor, id)
	} else {
		res = svc.CancelAction(ctx, actor, id)
	}
	out := chatbridge.ActionReply{Message: res.Message}
	if res.Card != nil && res.Card.Outcome != nil && strings.HasPrefix(res.Card.Outcome.URL, "/") && !strings.HasPrefix(res.Card.Outcome.URL, "//") {
		out.URL = res.Card.Outcome.URL
	}
	return out
}

// Export loads a stored export for the bridge.
func (c *capsuleBridge) Export(ctx context.Context, token string) (*chatbridge.ExportFile, error) {
	c.mu.RLock()
	exports := c.exports
	c.mu.RUnlock()
	if exports == nil {
		return nil, nil
	}
	f, err := exports.LoadExport(database.AsSystem(ctx), token)
	if err != nil || f == nil {
		return nil, err
	}
	return &chatbridge.ExportFile{UserID: f.UserID, Filename: f.Filename, MIMEType: f.MIMEType, Content: f.Content}, nil
}

// IngestAttachment stores an uploaded or downloaded file and returns its reference handle.
func (c *capsuleBridge) IngestAttachment(ctx context.Context, actor authctx.Actor, filename string, content []byte) (string, error) {
	svc, _ := c.service()
	if svc == nil {
		return "", errors.New("assistant service unavailable")
	}
	row, err := svc.IngestAttachment(ctx, actor, filename, content)
	if err != nil {
		return "", err
	}
	return row.PublicID.String(), nil
}

// chatExtras splits the non-record references of an answer into what a chat
// channel sends as their own messages: confirmation cards and files.
func chatExtras(ents []assistant.Entity) ([]chatbridge.AnswerProposal, []chatbridge.AnswerFile) {
	var proposals []chatbridge.AnswerProposal
	var files []chatbridge.AnswerFile
	for _, e := range ents {
		switch {
		case e.Kind == assistant.EntityProposal && e.Proposal != nil && e.Proposal.Status == "pending":
			p := chatbridge.AnswerProposal{
				ID: e.Proposal.ID, Title: e.Proposal.Preview.Title, Summary: e.Proposal.Preview.Summary,
				Warnings: e.Proposal.Preview.Warnings,
			}
			for _, d := range e.Proposal.Preview.Details {
				p.Details = append(p.Details, [2]string{d.Label, d.Value})
			}
			proposals = append(proposals, p)
		case e.Kind == assistant.EntityExport && strings.HasPrefix(e.URL, assistant.ExportPath("")):
			files = append(files, chatbridge.AnswerFile{Name: e.Title, Token: strings.TrimPrefix(e.URL, assistant.ExportPath(""))})
		}
	}
	return proposals, files
}
