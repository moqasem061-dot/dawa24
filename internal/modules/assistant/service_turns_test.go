package assistant

import (
	"context"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/assistant/actions"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/rbac"
)

// Ask is reachable from interfaces other than the browser, so it must apply the
// assistant gate itself. The service here has no repository and no gateway:
// reaching either would panic, which is the proof that a refused caller never
// gets past the gate.
func TestAskRefusesCallersTheBrowserWouldRefuse(t *testing.T) {
	s := &Service{}
	noGate := authctx.Actor{UserID: 1, OrgID: 2, Scope: rbac.ScopePharmacy}
	noGate.Grants([]string{"pharmacy.order.view"})

	for name, actor := range map[string]authctx.Actor{
		"no dashboard":     {UserID: 1},
		"no assistant key": noGate,
		"no user":          {Scope: rbac.ScopePharmacy},
	} {
		if res := s.Ask(context.Background(), actor, actions.ChannelTelegram, 0, "كم طلب؟"); res.Code != CodeForbidden {
			t.Errorf("%s: code %q, want forbidden", name, res.Code)
		}
	}
}
