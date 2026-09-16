package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/tools"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// fakeProbe stands in for the platform's coverage service. It records what it
// was asked so the test can assert that the branch and day reaching the rule
// are the ones the caller is entitled to, not the ones the model wrote.
type fakeProbe struct {
	gotBranch int64
	gotDay    time.Weekday
	answer    assistant.CoverageAnswer
	err       error
}

func (f *fakeProbe) VendorsServingBranch(
	_ context.Context, branchID int64, day time.Weekday,
) (assistant.CoverageAnswer, error) {
	f.gotBranch, f.gotDay = branchID, day
	return f.answer, f.err
}

func coverageFixture(t *testing.T, probe assistant.CoverageProbe) *fixture {
	t.Helper()
	f := newFixture(t)
	f.reg.SetCoverageProbe(probe)
	return f
}

// A pharmacy with a selected branch gets the suppliers that cover it, and the
// rule is asked about THAT branch — never about anything in the arguments.
func TestCoverageCheckUsesTheSessionBranch(t *testing.T) {
	probe := &fakeProbe{answer: assistant.CoverageAnswer{
		Evaluated:  true,
		BranchName: "فرع أسوان",
		Vendors: []assistant.CoverageVendorRow{
			{ID: 192, Name: "شركة النيل للأدوية", Windows: []string{"09:00-17:00"}, Area: "أسوان"},
		},
	}}
	f := coverageFixture(t, probe)

	a := pharmacist(188, 9)
	branch := int64(73)
	a.BranchID = &branch

	out := f.reg.Dispatch(context.Background(), a, 1, call("coverage_check", `{"day":"thursday"}`))
	if out.Decision != string(tools.DecisionAllowed) {
		t.Fatalf("decision %q, content %s", out.Decision, out.Content)
	}
	if probe.gotBranch != 73 {
		t.Errorf("the rule was asked about branch %d, want the session's 73", probe.gotBranch)
	}
	if probe.gotDay != time.Thursday {
		t.Errorf("the rule was asked about %s, want Thursday", probe.gotDay)
	}
	if !strings.Contains(out.Content, "شركة النيل للأدوية") {
		t.Errorf("the supplier name is missing from the result: %s", out.Content)
	}
	if len(out.Entities) == 0 {
		t.Error("the covering supplier was not collected as a clickable entity")
	}
}

// "Coverage was evaluated and nobody covers you" and "coverage could not be
// evaluated" must not read the same. Collapsing them is the flagship defect the
// availability review recorded, one layer down.
func TestCoverageCheckSeparatesEmptyFromUnevaluated(t *testing.T) {
	branch := int64(73)

	evaluated := &fakeProbe{answer: assistant.CoverageAnswer{Evaluated: true}}
	f := coverageFixture(t, evaluated)
	a := pharmacist(188, 9)
	a.BranchID = &branch
	empty := f.reg.Dispatch(context.Background(), a, 1, call("coverage_check", `{}`))

	unevaluated := &fakeProbe{answer: assistant.CoverageAnswer{Evaluated: false}}
	f2 := coverageFixture(t, unevaluated)
	unknown := f2.reg.Dispatch(context.Background(), a, 1, call("coverage_check", `{}`))

	if empty.Content == unknown.Content {
		t.Fatalf("both cases produced the same answer: %s", empty.Content)
	}
	if !strings.Contains(empty.Content, "لا يوجد مورّد") {
		t.Errorf("the empty case does not say nobody covers the branch: %s", empty.Content)
	}
	if !strings.Contains(unknown.Content, "تعذّر") {
		t.Errorf("the unevaluated case does not say the check failed: %s", unknown.Content)
	}
}

// With no probe wired the tool must say so. Answering "nobody delivers to you"
// from a missing dependency is a confident wrong answer, which is the thing
// this tool exists to stop.
func TestCoverageCheckWithoutAProbeSaysUnavailable(t *testing.T) {
	f := newFixture(t) // no probe
	a := pharmacist(188, 9)
	branch := int64(73)
	a.BranchID = &branch

	out := f.reg.Dispatch(context.Background(), a, 1, call("coverage_check", `{}`))
	if strings.Contains(out.Content, "لا يوجد مورّد") {
		t.Errorf("a missing probe was reported as an empty coverage answer: %s", out.Content)
	}
	if !strings.Contains(out.Content, "غير متاحة") {
		t.Errorf("the unavailability is not stated: %s", out.Content)
	}
}

// A supplier with no buying permission is refused; a supplier that buys asks
// about its own receiving branch like a pharmacy, and never sees itself listed.
func TestCoverageCheckFollowsBuyingPermission(t *testing.T) {
	probe := &fakeProbe{answer: assistant.CoverageAnswer{Evaluated: true, Vendors: []assistant.CoverageVendorRow{
		{ID: 192, Name: "المورد نفسه"}, {ID: 300, Name: "شركة النيل للأدوية"},
	}}}
	f := coverageFixture(t, probe)

	out := f.reg.Dispatch(context.Background(), vendor(192, 4), 1, call("coverage_check", `{}`))
	if out.Decision != string(tools.DecisionPermission) {
		t.Fatalf("seller without buying keys: decision %q, want %q", out.Decision, tools.DecisionPermission)
	}

	buyer := actor(rbac.ScopeVendor, 192, 4, assistant.GateVendor, "vendor.buying.catalog.view")
	branch := int64(81)
	buyer.BranchID = &branch
	out = f.reg.Dispatch(context.Background(), buyer, 1, call("coverage_check", `{}`))
	if out.Decision != string(tools.DecisionAllowed) {
		t.Fatalf("supplier that buys: decision %q, content %s", out.Decision, out.Content)
	}
	if strings.Contains(out.Content, "المورد نفسه") || !strings.Contains(out.Content, "شركة النيل للأدوية") {
		t.Errorf("a buying supplier must see others and not itself: %s", out.Content)
	}
}

// A branch handle issued to one caller must not resolve for another. This is
// the property that makes the branch argument safe to accept at all.
func TestCoverageCheckRefusesAnotherTenantsBranchHandle(t *testing.T) {
	probe := &fakeProbe{answer: assistant.CoverageAnswer{Evaluated: true}}
	f := coverageFixture(t, probe)

	// A handle minted for org 999 / user 1, presented by org 188 / user 9.
	foreign := f.signer.Issue(handles.KindBranch, 76, handles.Binding{OrgID: 999, UserID: 1})

	args, _ := json.Marshal(map[string]string{"branch": foreign})
	out := f.reg.Dispatch(context.Background(), pharmacist(188, 9), 1,
		call("coverage_check", string(args)))

	if out.Decision != string(tools.DecisionHandle) {
		t.Fatalf("decision %q, want %q", out.Decision, tools.DecisionHandle)
	}
	if probe.gotBranch != 0 {
		t.Errorf("the coverage rule was reached with branch %d despite a rejected handle",
			probe.gotBranch)
	}
}

// An unparseable day is refused rather than silently answered about today.
func TestCoverageCheckRefusesAnUnknownDay(t *testing.T) {
	probe := &fakeProbe{answer: assistant.CoverageAnswer{Evaluated: true}}
	f := coverageFixture(t, probe)
	a := pharmacist(188, 9)
	branch := int64(73)
	a.BranchID = &branch

	out := f.reg.Dispatch(context.Background(), a, 1, call("coverage_check", `{"day":"someday"}`))
	if out.Decision != string(tools.DecisionInvalid) {
		t.Fatalf("decision %q, want %q", out.Decision, tools.DecisionInvalid)
	}
	if probe.gotBranch != 0 {
		t.Error("the coverage rule was reached with an invalid day")
	}
}

// The tool is offered to buyers: a pharmacy, and a supplier holding buying
// keys; not to a supplier that only sells.
func TestCoverageCheckIsOfferedToBuyers(t *testing.T) {
	f := coverageFixture(t, &fakeProbe{})
	offered := func(a authctx.Actor) bool {
		for _, spec := range f.reg.Schemas(a) {
			if spec.Name == "coverage_check" {
				return true
			}
		}
		return false
	}
	if !offered(pharmacist(188, 9)) {
		t.Error("coverage_check is not offered to a pharmacy")
	}
	if !offered(actor(rbac.ScopeVendor, 192, 4, assistant.GateVendor, "vendor.buying.order.view")) {
		t.Error("coverage_check is not offered to a supplier that buys")
	}
	if offered(vendor(192, 4)) {
		t.Error("coverage_check is offered to a supplier that only sells")
	}
}
