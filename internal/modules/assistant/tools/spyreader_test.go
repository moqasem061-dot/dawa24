package tools_test

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// spyReader records every read that reaches the data layer. A denied call must
// leave calls empty: refusing after fetching is not refusing.
type spyReader struct {
	calls []string
}

func (s *spyReader) note(name string) { s.calls = append(s.calls, name) }

func (s *spyReader) Branches(context.Context, authctx.Actor) ([]assistant.BranchRow, error) {
	s.note("Branches")
	return []assistant.BranchRow{{ID: 11, Name: "الفرع الرئيسي", IsMain: true, Status: "active"}}, nil
}

func (s *spyReader) Wallet(context.Context, authctx.Actor) (*assistant.WalletSummary, error) {
	s.note("Wallet")
	return &assistant.WalletSummary{Currency: "EGP"}, nil
}

func (s *spyReader) Subscription(context.Context, authctx.Actor) (*assistant.SubscriptionSummary, error) {
	s.note("Subscription")
	return &assistant.SubscriptionSummary{PlanName: "الباقة الفضية", Status: "active"}, nil
}

func (s *spyReader) PlatformOverview(context.Context, authctx.Actor, assistant.DateRange) (*assistant.PlatformSummary, error) {
	s.note("PlatformOverview")
	return &assistant.PlatformSummary{}, nil
}
