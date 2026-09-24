package tools_test

import (
	"regexp"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/assistant"
	"github.com/moqasem061-dot/dawa24/internal/modules/assistant/datasets"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/rbac"
)

// A prompt that names a tool or dataset that does not exist sends the model to
// call something that will be refused. The names in each dashboard's prompt
// must be tools that dashboard is offered or datasets it can read.
func TestPromptsNameOnlyRealToolsAndDatasets(t *testing.T) {
	f := newFixture(t)
	snake := regexp.MustCompile(`\b[a-z]+(?:_[a-z]+)+\b`)
	// Action names are arguments to propose_action, declared by the dashboard
	// (internal/ui), which TestEveryCommandMatchesItsRouteGuard holds to its
	// routes.
	words := map[string]bool{"group_by": true, "cart_add": true, "offer_add": true, "place_order": true}

	for _, scope := range []rbac.Scope{rbac.ScopePharmacy, rbac.ScopeVendor, rbac.ScopeAdmin} {
		a := authctx.Actor{UserID: 1, OrgID: 1, Scope: scope, IsStaff: scope == rbac.ScopeAdmin}
		keys := rbac.Default().KeysFor(scope)
		a.Grants(keys)
		cfg, ok := assistant.AgentFor(a)
		if !ok {
			t.Fatalf("no agent for %s", scope)
		}
		offered := map[string]bool{}
		for _, spec := range f.reg.Schemas(a) {
			offered[spec.Name] = true
		}
		for _, name := range snake.FindAllString(cfg.SystemPrompt, -1) {
			if words[name] || offered[name] {
				continue
			}
			if _, ok := datasets.Default().For(a, name); ok {
				continue
			}
			t.Errorf("%s prompt names %q, which is neither an offered tool nor a readable dataset", scope, name)
		}
	}
}
