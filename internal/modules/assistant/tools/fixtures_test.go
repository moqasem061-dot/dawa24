package tools_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/actions"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/datasets"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/tools"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// auditLog captures decisions so a refusal can be checked for being recorded.
type auditLog struct {
	entries []assistant.ToolAudit
}

func (a *auditLog) RecordToolCall(_ context.Context, e assistant.ToolAudit) {
	a.entries = append(a.entries, e)
}

func (a *auditLog) last() assistant.ToolAudit {
	if len(a.entries) == 0 {
		return assistant.ToolAudit{}
	}
	return a.entries[len(a.entries)-1]
}

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

type fixture struct {
	reg    *tools.Registry
	reader *spyReader
	data   *spyRunner
	audit  *auditLog
	signer *handles.Signer
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	reader := &spyReader{}
	data := &spyRunner{}
	audit := &auditLog{}
	signer := handles.NewSigner(testSecret)
	reg := tools.NewRegistry(reader, signer, audit, nil)
	reg.SetDatasets(datasets.Default(), data, data)
	reg.SetActions(actions.NewFlow(nil, stubActions{}, assistant.CanAct, nil), nil)
	return &fixture{reg: reg, reader: reader, data: data, audit: audit, signer: signer}
}

// spyRunner records every compiled plan instead of running it, and answers
// with one row per plan so allowed calls have something to render.
type spyRunner struct {
	plans   []*datasets.Plan
	exports []assistant.ExportFile
}

func (s *spyRunner) RunDataset(_ context.Context, plan *datasets.Plan, _ time.Duration) (*datasets.Result, error) {
	s.plans = append(s.plans, plan)
	row := make([]any, len(plan.Columns))
	for i, c := range plan.Columns {
		switch c.Type {
		case datasets.Int, datasets.Money, datasets.Number, datasets.Ref:
			row[i] = json.Number("12")
		default:
			row[i] = "PO-7001"
		}
	}
	res := &datasets.Result{Columns: plan.Columns, Rows: [][]any{row}, Total: 1}
	if plan.HasKey {
		res.Keys = []int64{501}
	}
	return res, nil
}

func (s *spyRunner) SaveExport(_ context.Context, _ authctx.Actor, f assistant.ExportFile) (string, error) {
	s.exports = append(s.exports, f)
	return "tok_0123456789abcdef0123456789abcdef0123456", nil
}

// actor builds a caller with an explicit permission set, which is what a role
// resolved from the database produces.
func actor(scope rbac.Scope, orgID, userID int64, perms ...string) authctx.Actor {
	orgType := "customer"
	switch scope {
	case rbac.ScopeVendor:
		orgType = "vendor"
	case rbac.ScopeAdmin:
		orgType = ""
	}
	a := authctx.Actor{
		UserID:         userID,
		OrgID:          orgID,
		OrganizationID: orgID,
		OrgType:        orgType,
		OrgStatus:      "approved",
		Scope:          scope,
		IsStaff:        scope == rbac.ScopeAdmin,
	}
	a.Grants(perms)
	return a
}

// pharmacist is a fully-granted pharmacy user.
func pharmacist(orgID, userID int64) authctx.Actor {
	return actor(rbac.ScopePharmacy, orgID, userID,
		assistant.GatePharmacy, "pharmacy.order.view", "pharmacy.branch.view",
		"pharmacy.wallet.view", "pharmacy.subscription.view", "pharmacy.offer.view")
}

func vendor(orgID, userID int64) authctx.Actor {
	return actor(rbac.ScopeVendor, orgID, userID,
		assistant.GateVendor, "vendor.order.view", "vendor.product.view",
		"vendor.branch.view", "vendor.inventory.view", "vendor.offer.view")
}

func call(name, args string) gateway.ToolCall {
	return gateway.ToolCall{ID: "call_1", Name: name, Arguments: args}
}

// stubActions offers one action per dashboard so propose_action is reachable
// in schema tests. It never prepares or executes anything.
type stubActions struct{}

func (stubActions) Definitions(actor authctx.Actor) []actions.Definition {
	name := map[rbac.Scope]string{rbac.ScopePharmacy: "cart_add", rbac.ScopeVendor: "stock_adjust", rbac.ScopeAdmin: "issue_update"}[actor.DashboardScope()]
	if name == "" {
		return nil
	}
	return []actions.Definition{{Name: name, Label: name, Risk: actions.RiskLow, Description: "test"}}
}
func (stubActions) Permitted(authctx.Actor, string) bool { return true }
func (stubActions) Prepare(context.Context, authctx.Actor, string, actions.Args) (actions.Preview, error) {
	return actions.Preview{}, actions.ErrNotAllowed
}
func (stubActions) Execute(context.Context, authctx.Actor, string, actions.Args) (actions.Outcome, error) {
	return actions.Outcome{}, actions.ErrNotAllowed
}
