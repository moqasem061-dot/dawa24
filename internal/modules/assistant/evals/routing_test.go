package evals_test

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/actions"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/datasets"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/evals"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/tools"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// The offline half of the eval: can the question be answered at all, by
// somebody who is allowed to ask it?
//
// It builds a registry with no reader behind it, because nothing here runs a
// query — it asks the registry what it offers, which is a pure function of the
// declarations and the caller's grants.

// grantedActor is a user of one dashboard holding every permission that
// dashboard's tools ask for. Using a maximally-granted caller is deliberate: a
// tool missing from THIS actor's list is missing because it does not exist or
// is mis-scoped, never because of a permission the corpus forgot to grant.
func grantedActor(t *testing.T, scope rbac.Scope, reg *tools.Registry) authctx.Actor {
	t.Helper()

	perms := map[rbac.Scope][]string{
		rbac.ScopePharmacy: {assistant.GatePharmacy},
		rbac.ScopeVendor:   {assistant.GateVendor},
		rbac.ScopeAdmin:    {assistant.GateAdmin},
	}[scope]

	for _, name := range reg.Names() {
		tool, ok := reg.Lookup(name)
		if !ok {
			continue
		}
		perms = append(perms, tool.Permissions...)
	}

	orgType := "customer"
	switch scope {
	case rbac.ScopeVendor:
		orgType = "vendor"
	case rbac.ScopeAdmin:
		orgType = ""
	}
	a := authctx.Actor{
		UserID: 1, OrgID: 1, OrganizationID: 1,
		OrgType: orgType, OrgStatus: "approved",
		Scope: scope, IsStaff: scope == rbac.ScopeAdmin,
	}
	a.Grants(perms)
	return a
}

func newRegistry() *tools.Registry {
	reg := tools.NewRegistry(nil, handles.NewSigner("eval-secret-not-used-for-anything"), nil, nil)
	reg.SetActions(actions.NewFlow(nil, evalActions{}, assistant.CanAct, nil), nil)
	return reg
}

// evalActions offers one action per dashboard, so routing can see that
// propose_action is reachable for a granted user.
type evalActions struct{}

func (evalActions) Definitions(authctx.Actor) []actions.Definition {
	return []actions.Definition{{Name: "noop", Label: "noop", Risk: actions.RiskLow, Description: "eval"}}
}
func (evalActions) Permitted(authctx.Actor, string) bool { return true }
func (evalActions) Prepare(context.Context, authctx.Actor, string, actions.Args) (actions.Preview, error) {
	return actions.Preview{}, actions.ErrNotAllowed
}
func (evalActions) Execute(context.Context, authctx.Actor, string, actions.Args) (actions.Outcome, error) {
	return actions.Outcome{}, actions.ErrNotAllowed
}

// TestCorpusLoads is the guard on the corpus itself. A malformed line, a
// duplicate id or an unknown scope fails here rather than silently shrinking
// the measurement.
func TestCorpusLoads(t *testing.T) {
	cases, err := evals.Load()
	if err != nil {
		t.Fatal(err)
	}
	byScope := evals.ByScope(cases)
	for _, scope := range []rbac.Scope{rbac.ScopePharmacy, rbac.ScopeVendor, rbac.ScopeAdmin} {
		if n := len(byScope[scope]); n < 60 {
			t.Errorf("%s: %d questions, the work order asks for at least 60", scope, n)
		}
	}
	t.Logf("corpus: %d questions across %d dashboards", len(cases), len(byScope))
}

// TestRoutingCoverage reports, per dashboard, how many corpus questions have a
// tool that exists and is offered to a granted user of that dashboard.
//
// It does not fail on a gap. The gaps are the remaining work, they are listed
// in the work order, and a red build on "a planned tool is not written yet"
// teaches everyone to ignore the build. What it fails on is a REGRESSION: a
// tool that the corpus expects, that exists, and that has stopped being offered
// to the dashboard it belongs to. That is a mis-scoped or mis-permissioned
// tool, and it is invisible from every other angle.
func TestRoutingCoverage(t *testing.T) {
	cases, err := evals.Load()
	if err != nil {
		t.Fatal(err)
	}
	reg := newRegistry()

	type report struct {
		total, covered, refuse int
		gaps                   map[string][]string // tool -> question ids
	}
	reports := map[rbac.Scope]*report{}

	for scope, group := range evals.ByScope(cases) {
		actor := grantedActor(t, scope, reg)

		offered := map[string]bool{}
		for _, spec := range reg.Schemas(actor) {
			offered[spec.Name] = true
		}

		rep := &report{gaps: map[string][]string{}}
		for _, c := range group {
			rep.total++
			if c.MustRefuse() {
				rep.refuse++
				// A refusal case is "covered" by there being no tool that
				// would satisfy it. If one ever appears, that is a finding.
				if offered[c.Tool] {
					t.Errorf("%s: question %s expects a refusal but tool %q is offered",
						scope, c.ID, c.Tool)
				}
				continue
			}
			if offered[c.Tool] {
				rep.covered++
				continue
			}
			// Declared but not offered to this scope is a real defect; not
			// declared at all is the known backlog.
			if _, declared := reg.Lookup(c.Tool); declared {
				t.Errorf("%s: tool %q exists but is not offered to a fully-granted %s user (question %s)",
					scope, c.Tool, scope, c.ID)
			}
			rep.gaps[c.Tool] = append(rep.gaps[c.Tool], c.ID)
		}
		reports[scope] = rep
	}

	var overallTotal, overallCovered int
	for _, scope := range []rbac.Scope{rbac.ScopePharmacy, rbac.ScopeVendor, rbac.ScopeAdmin} {
		rep := reports[scope]
		if rep == nil {
			continue
		}
		answerable := rep.total - rep.refuse
		overallTotal += answerable
		overallCovered += rep.covered
		t.Logf("%-9s routing coverage %2d/%2d (%3.0f%%) — %d refusal cases, %d tools missing",
			scope, rep.covered, answerable, pct(rep.covered, answerable), rep.refuse, len(rep.gaps))

		missing := make([]string, 0, len(rep.gaps))
		for tool := range rep.gaps {
			missing = append(missing, tool)
		}
		sort.Strings(missing)
		for _, tool := range missing {
			t.Logf("    missing %-26s wanted by %s", tool, joinIDs(rep.gaps[tool]))
		}
	}
	t.Logf("TOTAL routing coverage %d/%d (%.0f%%)",
		overallCovered, overallTotal, pct(overallCovered, overallTotal))
}

// TestCorpusDatasetsAreReadable holds every dataset the corpus expects to a
// dataset that exists for that dashboard. A renamed or mis-scoped dataset fails
// here rather than as a silently worse answer.
func TestCorpusDatasetsAreReadable(t *testing.T) {
	cases, err := evals.Load()
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, c := range cases {
		if c.Dataset == "" {
			continue
		}
		scope := c.DashboardScope()
		a := authctx.Actor{UserID: 1, OrgID: 1, OrganizationID: 1, Scope: scope, IsStaff: scope == rbac.ScopeAdmin}
		a.Grants(rbac.Default().KeysFor(scope))
		if _, ok := datasets.Default().For(a, c.Dataset); !ok {
			t.Errorf("%s: dataset %q is not readable by a granted %s user", c.ID, c.Dataset, scope)
		}
		checked++
	}
	t.Logf("%d dataset expectations checked", checked)
}

// TestEveryDeclaredToolIsReachable is the inverse check: a tool nobody can be
// offered is dead weight that still appears in reviews and audits as though it
// worked.
func TestEveryDeclaredToolIsReachable(t *testing.T) {
	reg := newRegistry()

	reachable := map[string]bool{}
	for _, scope := range []rbac.Scope{rbac.ScopePharmacy, rbac.ScopeVendor, rbac.ScopeAdmin} {
		for _, spec := range reg.Schemas(grantedActor(t, scope, reg)) {
			reachable[spec.Name] = true
		}
	}
	for _, name := range reg.Names() {
		if !reachable[name] {
			t.Errorf("tool %q is declared but reachable from no dashboard", name)
		}
	}
	t.Logf("%d tools declared, all reachable", len(reg.Names()))
}

// TestGateClosesEverything is the security property the corpus depends on: with
// the assistant grant withheld, no tool is offered at all, whatever else the
// user may open.
func TestGateClosesEverything(t *testing.T) {
	reg := newRegistry()
	for _, scope := range []rbac.Scope{rbac.ScopePharmacy, rbac.ScopeVendor, rbac.ScopeAdmin} {
		granted := grantedActor(t, scope, reg)

		ungated := authctx.Actor{
			UserID: granted.UserID, OrgID: granted.OrgID,
			OrganizationID: granted.OrganizationID, OrgType: granted.OrgType,
			OrgStatus: granted.OrgStatus, Scope: granted.Scope, IsStaff: granted.IsStaff,
		}
		// Everything EXCEPT the assistant gate. The gate keys have to be
		// filtered out explicitly rather than simply not added: the memory
		// tools declare the gate as their own permission — there is no
		// separate "may use memory" key, and using the gate is the deliberate
		// choice — so collecting every tool's permissions hands it back.
		gates := map[string]bool{
			assistant.GatePharmacy: true,
			assistant.GateVendor:   true,
			assistant.GateAdmin:    true,
		}
		var perms []string
		for _, name := range reg.Names() {
			tool, ok := reg.Lookup(name)
			if !ok {
				continue
			}
			for _, p := range tool.Permissions {
				if !gates[p] {
					perms = append(perms, p)
				}
			}
		}
		ungated.Grants(perms)

		if specs := reg.Schemas(ungated); len(specs) != 0 {
			t.Errorf("%s: %d tools offered to a user without the assistant grant", scope, len(specs))
		}
	}
}

// TestDispatchRefusesWithoutGate proves the offer filter is not the boundary:
// calling a tool by name, gate withheld, is refused and recorded.
func TestDispatchRefusesWithoutGate(t *testing.T) {
	audit := &recordingAudit{}
	reg := tools.NewRegistry(nil, handles.NewSigner("eval-secret-not-used-for-anything"), audit, nil)

	a := authctx.Actor{
		UserID: 1, OrgID: 1, OrganizationID: 1, OrgType: "customer",
		OrgStatus: "approved", Scope: rbac.ScopePharmacy,
	}
	a.Grants([]string{"pharmacy.order.view"}) // no assistant gate

	out := reg.Dispatch(context.Background(), a, 1, toolCall("orders_list"))
	if out.Decision != string(tools.DecisionGate) {
		t.Errorf("decision was %q, want %q", out.Decision, tools.DecisionGate)
	}
	if len(audit.entries) != 1 || audit.entries[0].Decision != string(tools.DecisionGate) {
		t.Errorf("the refusal was not recorded in the audit trail: %+v", audit.entries)
	}
}

func pct(n, of int) float64 {
	if of == 0 {
		return 0
	}
	return float64(n) * 100 / float64(of)
}

func joinIDs(ids []string) string {
	if len(ids) <= 4 {
		return fmt.Sprint(ids)
	}
	return fmt.Sprintf("%v +%d more", ids[:4], len(ids)-4)
}

type recordingAudit struct{ entries []assistant.ToolAudit }

func (r *recordingAudit) RecordToolCall(_ context.Context, e assistant.ToolAudit) {
	r.entries = append(r.entries, e)
}

func toolCall(name string) gateway.ToolCall {
	return gateway.ToolCall{ID: "eval_call", Name: name, Arguments: "{}"}
}
