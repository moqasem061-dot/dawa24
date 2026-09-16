package main

import (
	"context"
	"sync"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/actions"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/tools"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui"
)

// Capsule acts through the dashboard's own code, which lives on the UI
// handler. The assistant is mounted inside the API group before that handler
// exists, so it is handed this proxy, bound once the handler is built. Until
// then it offers no actions and refuses every request.
type lateActions struct {
	mu     sync.RWMutex
	target *ui.AssistantActions
}

var (
	_ actions.Executor  = (*lateActions)(nil)
	_ tools.OfferFinder = (*lateActions)(nil)
)

func (l *lateActions) bind(t *ui.AssistantActions) {
	l.mu.Lock()
	l.target = t
	l.mu.Unlock()
}

func (l *lateActions) get() *ui.AssistantActions {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.target
}

func (l *lateActions) Definitions(actor authctx.Actor) []actions.Definition {
	if t := l.get(); t != nil {
		return t.Definitions(actor)
	}
	return nil
}

func (l *lateActions) Permitted(actor authctx.Actor, name string) bool {
	t := l.get()
	return t != nil && t.Permitted(actor, name)
}

func (l *lateActions) Prepare(ctx context.Context, actor authctx.Actor, name string, args actions.Args) (actions.Preview, error) {
	if t := l.get(); t != nil {
		return t.Prepare(ctx, actor, name, args)
	}
	return actions.Preview{}, actions.ErrNotAllowed
}

func (l *lateActions) Execute(ctx context.Context, actor authctx.Actor, name string, args actions.Args) (actions.Outcome, error) {
	if t := l.get(); t != nil {
		return t.Execute(ctx, actor, name, args)
	}
	return actions.Outcome{}, actions.ErrNotAllowed
}

func (l *lateActions) FindOffers(ctx context.Context, actor authctx.Actor, q assistant.OfferQuery) (*assistant.OfferResult, error) {
	if t := l.get(); t != nil {
		return t.FindOffers(ctx, actor, q)
	}
	return nil, actions.Refuse("البحث في العروض غير متاح حالياً.")
}

func (l *lateActions) FindPromotions(ctx context.Context, actor authctx.Actor, q assistant.PromotionQuery) (*assistant.PromotionResult, error) {
	if t := l.get(); t != nil {
		return t.FindPromotions(ctx, actor, q)
	}
	return nil, actions.Refuse("العروض غير متاحة حالياً.")
}

func (l *lateActions) PromotionDetail(ctx context.Context, actor authctx.Actor, offerID, branchID int64) (*assistant.PromotionDetail, error) {
	if t := l.get(); t != nil {
		return t.PromotionDetail(ctx, actor, offerID, branchID)
	}
	return nil, actions.Refuse("العروض غير متاحة حالياً.")
}
