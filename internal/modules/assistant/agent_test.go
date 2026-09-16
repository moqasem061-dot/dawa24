package assistant_test

import (
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// Agent selection is a pure function of the authenticated caller. These tests
// say so, and say that no request field can influence it — because the only
// input is an Actor built by authentication.

func withGrants(a authctx.Actor, perms ...string) authctx.Actor {
	a.Grants(perms)
	return a
}

func TestAgentFollowsDashboard(t *testing.T) {
	cases := []struct {
		name  string
		actor authctx.Actor
		want  rbac.Scope
	}{
		{"pharmacy member", authctx.Actor{UserID: 1, OrgID: 1, OrgType: "customer"}, rbac.ScopePharmacy},
		{"legacy pharmacy spelling", authctx.Actor{UserID: 1, OrgID: 1, OrgType: "chain_pharmacy"}, rbac.ScopePharmacy},
		{"vendor member", authctx.Actor{UserID: 2, OrgID: 2, OrgType: "vendor"}, rbac.ScopeVendor},
		{"legacy vendor spelling", authctx.Actor{UserID: 2, OrgID: 2, OrgType: "supplier"}, rbac.ScopeVendor},
		{"platform staff", authctx.Actor{UserID: 3, IsStaff: true}, rbac.ScopeAdmin},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, ok := assistant.AgentFor(tc.actor)
			if !ok {
				t.Fatal("no agent selected")
			}
			if cfg.Role != tc.want {
				t.Fatalf("role = %q, want %q", cfg.Role, tc.want)
			}
			if strings.TrimSpace(cfg.SystemPrompt) == "" {
				t.Fatal("agent has no instructions")
			}
			if cfg.Gate == "" {
				t.Fatal("agent has no gate permission")
			}
		})
	}
}

// Each agent's prompt must be its own. A shared prompt is how a supplier came
// to be addressed as a pharmacy.
func TestAgentPromptsAreDistinct(t *testing.T) {
	seen := map[string]string{}
	for _, a := range []authctx.Actor{
		{UserID: 1, OrgID: 1, OrgType: "customer"},
		{UserID: 2, OrgID: 2, OrgType: "vendor"},
		{UserID: 3, IsStaff: true},
	} {
		cfg, _ := assistant.AgentFor(a)
		if prior, dup := seen[cfg.SystemPrompt]; dup {
			t.Fatalf("agents %q and %q share a prompt", prior, cfg.Role)
		}
		seen[cfg.SystemPrompt] = string(cfg.Role)
	}
}

// Every prompt must state the rules the safety of the whole design leans on:
// numbers come from tools, actions are only proposed, fenced content is data.
func TestEveryPromptStatesTheSafetyRules(t *testing.T) {
	for _, a := range []authctx.Actor{
		{UserID: 1, OrgID: 1, OrgType: "customer"},
		{UserID: 2, OrgID: 2, OrgType: "vendor"},
		{UserID: 3, IsStaff: true},
	} {
		cfg, _ := assistant.AgentFor(a)
		for _, rule := range []string{
			"must come from a tool result",
			"You never carry them out",
			"Never say it is done",
		} {
			if !strings.Contains(cfg.SystemPrompt, rule) {
				t.Errorf("agent %q prompt is missing the rule %q", cfg.Role, rule)
			}
		}
		if !strings.Contains(cfg.SystemPrompt, "UNTRUSTED_CONTENT") {
			t.Errorf("agent %q does not describe the untrusted-content fence", cfg.Role)
		}
	}
}

// A user with no dashboard gets no agent, rather than falling through to
// somebody else's.
func TestNoDashboardMeansNoAgent(t *testing.T) {
	orphan := authctx.Actor{UserID: 9, Role: "job_seeker"}
	orphan.Scope = ""
	orphan.OrgType = "unknown_type"

	if _, ok := assistant.AgentFor(orphan); ok {
		t.Fatal("a user with no dashboard was given an agent")
	}
}

// Allowed is the gate the owner controls.
func TestAllowedRequiresTheGrant(t *testing.T) {
	base := authctx.Actor{UserID: 1, OrgID: 1, OrgType: "customer"}

	if _, ok := assistant.Allowed(withGrants(base, "pharmacy.order.view")); ok {
		t.Fatal("assistant allowed without its permission")
	}
	if _, ok := assistant.Allowed(withGrants(base, assistant.GatePharmacy)); !ok {
		t.Fatal("assistant refused to a user holding the grant")
	}
	// A vendor's grant must not admit a pharmacy user, even if somehow held.
	if _, ok := assistant.Allowed(withGrants(base, assistant.GateVendor)); ok {
		t.Fatal("the vendor grant admitted a pharmacy user")
	}
}

// A wildcard holder — a company owner — passes without an explicit grant,
// because owners hold their whole scope.
func TestOwnerWildcardPasses(t *testing.T) {
	owner := withGrants(authctx.Actor{UserID: 1, OrgID: 1, OrgType: "vendor", IsOwner: true}, "vendor.*")
	if _, ok := assistant.Allowed(owner); !ok {
		t.Fatal("a vendor owner was refused the assistant")
	}
}
