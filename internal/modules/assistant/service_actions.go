package assistant

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/muhiya/dawa24-store/internal/modules/assistant/actions"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// Confirming and cancelling proposals, shared by every interface.
//
// The web drawer and the Telegram bot both land here, so the wording a person
// reads after pressing a button, and the record left in the conversation, are
// the same whichever door they used.

// SetActionFlow installs the proposal flow.
func (s *Service) SetActionFlow(f *actions.Flow) { s.actions = f }

// ActionResult is what an interface shows after a decision.
type ActionResult struct {
	// Card is the proposal's current state, or nil when there is none to show.
	Card *ProposalCard
	// Message is one sentence for the person who pressed the button.
	Message string
	// Outcome classifies the result for the interface.
	Outcome ActionOutcome
}

// ActionOutcome classifies a decision.
type ActionOutcome string

const (
	ActionExecuted  ActionOutcome = "executed"
	ActionCancelled ActionOutcome = "cancelled"
	ActionRefused   ActionOutcome = "refused"
	ActionStale     ActionOutcome = "stale"
	ActionExpired   ActionOutcome = "expired"
	ActionNotFound  ActionOutcome = "not_found"
	ActionForbidden ActionOutcome = "forbidden"
	ActionFailed    ActionOutcome = "failed"
)

// ConfirmAction executes a proposal for its own user.
func (s *Service) ConfirmAction(ctx context.Context, actor authctx.Actor, publicID uuid.UUID) ActionResult {
	if s.actions == nil {
		return ActionResult{Outcome: ActionForbidden, Message: "تنفيذ الإجراءات غير متاح حالياً."}
	}
	p, err := s.actions.Confirm(ctx, actor, publicID)
	res := decisionResult(p, err)
	if p != nil {
		s.recordDecision(ctx, actor, p, res.Message)
	}
	return res
}

// CancelAction withdraws a proposal.
func (s *Service) CancelAction(ctx context.Context, actor authctx.Actor, publicID uuid.UUID) ActionResult {
	if s.actions == nil {
		return ActionResult{Outcome: ActionForbidden, Message: "تنفيذ الإجراءات غير متاح حالياً."}
	}
	p, err := s.actions.Cancel(ctx, actor, publicID)
	res := decisionResult(p, err)
	if p != nil && err == nil {
		s.recordDecision(ctx, actor, p, res.Message)
	}
	return res
}

// RefreshProposals brings proposal cards in stored entities up to date, so a
// reopened conversation shows "confirmed" rather than a live button.
func (s *Service) RefreshProposals(ctx context.Context, actor authctx.Actor, ents []Entity) {
	if s.actions == nil {
		return
	}
	for i := range ents {
		card := ents[i].Proposal
		if card == nil {
			continue
		}
		id, err := uuid.Parse(card.ID)
		if err != nil {
			continue
		}
		p, err := s.actions.Get(ctx, actor, id)
		if err != nil {
			card.Status = string(actions.StatusExpired)
			continue
		}
		card.Status, card.Outcome, card.Error = string(p.Status), p.Outcome, p.Error
	}
}

func decisionResult(p *actions.Pending, err error) ActionResult {
	res := ActionResult{}
	if p != nil {
		e := ProposalEntity(p)
		res.Card = e.Proposal
	}
	var stale *actions.ErrStale
	switch {
	case err == nil && p != nil && p.Status == actions.StatusCancelled:
		res.Outcome, res.Message = ActionCancelled, "أُلغي الإجراء."
	case err == nil && p != nil && p.Outcome != nil:
		res.Outcome, res.Message = ActionExecuted, p.Outcome.Message
	case err == nil:
		res.Outcome, res.Message = ActionExecuted, "تم التنفيذ."
	case errors.As(err, &stale):
		res.Outcome = ActionStale
		res.Message = "تغيّرت البيانات منذ تجهيز هذا الإجراء، فلم يُنفَّذ. اطلب تجهيزه من جديد لترى التفاصيل الحالية."
	case errors.Is(err, actions.ErrExpired):
		res.Outcome, res.Message = ActionExpired, "انتهت صلاحية هذا الإجراء. اطلب تجهيزه من جديد."
	case errors.Is(err, actions.ErrDecided):
		res.Outcome, res.Message = ActionRefused, "تم التعامل مع هذا الإجراء من قبل."
	case errors.Is(err, actions.ErrNotFound):
		res.Outcome, res.Message = ActionNotFound, "هذا الإجراء غير موجود."
	case errors.Is(err, actions.ErrNotAllowed):
		res.Outcome, res.Message = ActionForbidden, "لم تعد تملك صلاحية تنفيذ هذا الإجراء."
	default:
		if refusal, ok := actions.AsRefusal(err); ok {
			res.Outcome, res.Message = ActionRefused, refusal.Message
		} else {
			res.Outcome, res.Message = ActionFailed, "تعذّر تنفيذ الإجراء. لم يتغير شيء، حاول مرة أخرى."
		}
	}
	return res
}

// recordDecision appends the result to the conversation the proposal came
// from, so the next question in that thread knows what happened.
func (s *Service) recordDecision(ctx context.Context, actor authctx.Actor, p *actions.Pending, message string) {
	if s.repo == nil || p.ConversationID <= 0 || message == "" {
		return
	}
	entity := ProposalEntity(p)
	if err := s.repo.SaveMessage(context.WithoutCancel(ctx), &Message{
		ConversationID: p.ConversationID,
		OrganizationID: actor.OrgID,
		Role:           "assistant",
		Content:        message,
		PromptVersion:  SystemPromptVersion,
		Entities:       []Entity{entity},
	}); err != nil {
		s.log.ErrorContext(ctx, "assistant: record action decision", "error", err)
	}
}
