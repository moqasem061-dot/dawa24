package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/tools"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// The hostile-model suite.
//
// Every test here scripts a tool call the way a compromised or manipulated
// model would emit one — a foreign handle, another dashboard's dataset, an
// argument nobody declared — and asserts that dispatch refuses it before any
// query runs. The reader and the dataset runner are instrumented, so "no query
// ran" is a fact the test can check rather than an inference.

const testSecret = "assistant-tools-test-secret-value-32ch"

func (f *fixture) reads() int { return len(f.reader.calls) + len(f.data.plans) }

// ---------------------------------------------------------------------------
// The gate: the owner's per-role switch
// ---------------------------------------------------------------------------

func TestAssistantGateIsRequired(t *testing.T) {
	f := newFixture(t)
	ungated := actor(rbac.ScopePharmacy, 1, 10, "pharmacy.order.view")

	if got := f.reg.Schemas(ungated); len(got) != 0 {
		t.Fatalf("ungated user was shown %d tools", len(got))
	}
	out := f.reg.Dispatch(context.Background(), ungated, 0, call("query_data", `{"dataset":"purchase_orders"}`))
	if out.Decision != string(tools.DecisionGate) {
		t.Fatalf("decision = %q, want denied_gate", out.Decision)
	}
	if f.reads() != 0 {
		t.Fatal("a denied call still read data")
	}
}

// Holding the gate alone must not widen access: it admits the assistant, and
// each dataset still asks for the permission its screen asks for.
func TestGateAloneGrantsNoData(t *testing.T) {
	f := newFixture(t)
	gateOnly := actor(rbac.ScopePharmacy, 1, 10, assistant.GatePharmacy)

	out := f.reg.Dispatch(context.Background(), gateOnly, 0, call("query_data", `{"dataset":"purchase_orders"}`))
	if out.Decision == string(tools.DecisionAllowed) {
		t.Fatalf("gate alone read orders: %s", out.Content)
	}
	if f.reads() != 0 {
		t.Fatal("a refused dataset still ran")
	}
	if f.audit.last().Decision == string(tools.DecisionAllowed) {
		t.Fatal("refusal was not audited as a refusal")
	}
}

// ---------------------------------------------------------------------------
// Cross-role
// ---------------------------------------------------------------------------

func TestCrossRoleDatasetsAreRefused(t *testing.T) {
	f := newFixture(t)
	ph := pharmacist(1, 10)

	for _, ds := range []string{"sales_lines", "catalog_listings", "stock_levels", "users", "organizations", "platform_orders"} {
		t.Run(ds, func(t *testing.T) {
			out := f.reg.Dispatch(context.Background(), ph, 0, call("query_data", `{"dataset":"`+ds+`"}`))
			if out.Decision == string(tools.DecisionAllowed) {
				t.Fatalf("pharmacy user read %s", ds)
			}
		})
	}
	for _, name := range []string{"platform_overview", "inventory_health"} {
		if out := f.reg.Dispatch(context.Background(), ph, 0, call(name, "{}")); out.Decision == string(tools.DecisionAllowed) {
			t.Fatalf("pharmacy user was allowed %s", name)
		}
	}
	if f.reads() != 0 {
		t.Fatal("refused calls still read data")
	}
}

func TestVendorCannotReachPharmacyData(t *testing.T) {
	f := newFixture(t)
	v := vendor(2, 20)

	for _, c := range []struct{ name, args string }{
		{"saving_products_list", `{}`},
		{"decision_memory_search", `{"search":"panadol"}`},
		{"list_promotions", `{}`},
		{"find_offers", `{"search":"x"}`},
	} {
		if out := f.reg.Dispatch(context.Background(), v, 0, call(c.name, c.args)); out.Decision == string(tools.DecisionAllowed) {
			t.Fatalf("vendor was allowed %s %s", c.name, c.args)
		}
	}
	if f.reads() != 0 {
		t.Fatal("refused calls still read data")
	}
}

func TestVendorCanQueryPurchasingData(t *testing.T) {
	f := newFixture(t)
	v := vendor(2, 20)

	for _, c := range []struct{ name, args string }{
		{"query_data", `{"dataset":"purchase_orders"}`},
		{"query_data", `{"dataset":"purchase_order_lines"}`},
		{"export_data", `{"dataset":"purchase_orders","title":"طلبات الشراء","format":"csv"}`},
	} {
		if out := f.reg.Dispatch(context.Background(), v, 0, call(c.name, c.args)); out.Decision != string(tools.DecisionAllowed) {
			t.Fatalf("vendor should be allowed %s %s: got %s (%s)", c.name, c.args, out.Decision, out.Content)
		}
	}
}

func TestSchemasAreScoped(t *testing.T) {
	f := newFixture(t)
	cases := []struct {
		name            string
		actor           authctx.Actor
		allowed, denied []string
	}{
		{"pharmacy", pharmacist(1, 10),
			[]string{"query_data", "describe_data", "get_record", "export_data", "list_promotions", "offer_details", "coverage_check", "reorder_suggestions", "wallet_summary"},
			// find_offers needs the catalogue key this pharmacist was not granted.
			[]string{"inventory_health", "platform_overview", "finance_overview", "find_offers"}},
		{"vendor", vendor(2, 20),
			[]string{"query_data", "get_record", "inventory_health"},
			[]string{"platform_overview", "saving_products_list", "decision_memory_search", "find_offers", "list_promotions"}},
		// A supplier that buys holds the vendor.buying keys and gets the
		// buying tools a pharmacy gets, but not a pharmacy's own screens.
		{"vendor buying", actor(rbac.ScopeVendor, 2, 20, assistant.GateVendor,
			"vendor.buying.catalog.view", "vendor.buying.offer.view", "vendor.buying.order.view",
			"vendor.buying.favorite.view", "vendor.buying.supplier.view", "vendor.buying.smart_order.view", "vendor.wallet.view"),
			[]string{"find_offers", "list_promotions", "offer_details", "coverage_check", "reorder_suggestions",
				"favourites_list", "supplier_profile", "smart_order_run_details", "financial_obligations_summary", "query_data"},
			[]string{"saving_products_list", "decision_memory_search", "platform_overview", "inventory_health"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seen := map[string]bool{}
			for _, s := range f.reg.Schemas(tc.actor) {
				seen[s.Name] = true
			}
			for _, want := range tc.allowed {
				if !seen[want] {
					t.Errorf("%s should be offered %s", tc.name, want)
				}
			}
			for _, deny := range tc.denied {
				if seen[deny] {
					t.Errorf("%s must not be offered %s", tc.name, deny)
				}
			}
		})
	}
}

// A partially-granted employee can list only the datasets their grants admit,
// so the model cannot even name the others.
func TestDescribeFollowsIndividualGrants(t *testing.T) {
	f := newFixture(t)
	limited := actor(rbac.ScopePharmacy, 1, 10, assistant.GatePharmacy, "pharmacy.branch.view")

	out := f.reg.Dispatch(context.Background(), limited, 0, call("describe_data", "{}"))
	if out.Decision != string(tools.DecisionAllowed) {
		t.Fatalf("describe refused: %s", out.Content)
	}
	if !strings.Contains(out.Content, `"dataset":"branches"`) {
		t.Fatalf("branch permission did not list the branches dataset: %s", out.Content)
	}
	for _, forbidden := range []string{"purchase_orders", `"payments"`, "cart_items"} {
		if strings.Contains(out.Content, forbidden) {
			t.Fatalf("%s listed without its permission", forbidden)
		}
	}
}

// ---------------------------------------------------------------------------
// Tenant binding
// ---------------------------------------------------------------------------

// Whatever the model sends, the compiled statement is bound to the caller's own
// organisation, taken from the session.
func TestQueriesAreBoundToTheCallersOrganisation(t *testing.T) {
	f := newFixture(t)
	ph := pharmacist(41, 10)

	out := f.reg.Dispatch(context.Background(), ph, 0, call("query_data",
		`{"dataset":"purchase_orders","filters":[{"field":"number","op":"eq","value":"PO-1 OR 1=1"}],"search":"'; --"}`))
	if out.Decision != string(tools.DecisionAllowed) {
		t.Fatalf("decision = %q: %s", out.Decision, out.Content)
	}
	if len(f.data.plans) != 1 {
		t.Fatalf("plans = %d", len(f.data.plans))
	}
	plan := f.data.plans[0]
	if plan.Args[0] != int64(41) || !strings.Contains(plan.SQL, "o.organization_id = $1") {
		t.Fatalf("tenant not bound to the session org: %s %v", plan.SQL, plan.Args)
	}
	if strings.Contains(plan.SQL, "OR 1=1") || strings.Contains(plan.SQL, "--") {
		t.Fatalf("a value reached the SQL text: %s", plan.SQL)
	}
}

// ---------------------------------------------------------------------------
// Handles: forgery, enumeration, cross-tenant replay
// ---------------------------------------------------------------------------

func TestForeignHandleIsRefused(t *testing.T) {
	f := newFixture(t)
	victim := pharmacist(1, 10)
	attacker := pharmacist(2, 20)

	stolen := f.signer.Issue(handles.KindOrder, 501, handles.Binding{OrgID: victim.OrgID, UserID: victim.UserID})
	for _, c := range []struct{ name, args string }{
		{"get_record", `{"ref":"` + stolen + `"}`},
		{"query_data", `{"dataset":"purchase_order_lines","filters":[{"field":"order_ref","op":"eq","value":"` + stolen + `"}]}`},
	} {
		out := f.reg.Dispatch(context.Background(), attacker, 0, call(c.name, c.args))
		if out.Decision != string(tools.DecisionHandle) {
			t.Fatalf("%s: decision = %q, want denied_handle", c.name, out.Decision)
		}
	}
	if f.reads() != 0 {
		t.Fatal("a foreign handle still reached the database")
	}
}

func TestRawIDsAreNotAccepted(t *testing.T) {
	f := newFixture(t)
	ph := pharmacist(1, 10)

	for _, guess := range []string{"1", "501", "999999", "hord_1", "", "null", "hzzz_AAAA.BBBB"} {
		args, _ := json.Marshal(map[string]string{"ref": guess})
		out := f.reg.Dispatch(context.Background(), ph, 0, call("get_record", string(args)))
		if out.Decision == string(tools.DecisionAllowed) {
			t.Fatalf("raw id %q was accepted as a handle", guess)
		}
	}
	if f.reads() != 0 {
		t.Fatal("guessed ids reached the database")
	}
}

// The caller's own handle opens the record and the rows under it — the refusals
// above are not simply "nothing works".
func TestOwnHandleResolves(t *testing.T) {
	f := newFixture(t)
	ph := pharmacist(1, 10)

	own := f.signer.Issue(handles.KindOrder, 501, handles.Binding{OrgID: ph.OrgID, UserID: ph.UserID})
	out := f.reg.Dispatch(context.Background(), ph, 0, call("get_record", `{"ref":"`+own+`"}`))
	if out.Decision != string(tools.DecisionAllowed) {
		t.Fatalf("decision = %q, want allowed: %s", out.Decision, out.Content)
	}
	if len(f.data.plans) < 2 {
		t.Fatalf("want the record and its related rows, got %d plans", len(f.data.plans))
	}
	for _, p := range f.data.plans {
		if p.Args[0] != int64(1) {
			t.Fatalf("related read not bound to the caller: %s %v", p.Dataset.Name, p.Args)
		}
	}
	if !strings.Contains(out.Content, `"related"`) {
		t.Fatalf("related rows missing: %s", out.Content)
	}
}

// Field permissions hold through the tools too.
func TestFieldPermissionsHoldThroughTheTools(t *testing.T) {
	f := newFixture(t)
	seller := actor(rbac.ScopeVendor, 2, 20, assistant.GateVendor, "vendor.order.view")

	out := f.reg.Dispatch(context.Background(), seller, 0, call("query_data",
		`{"dataset":"sales_lines","metrics":["sum:unit_cost"]}`))
	if out.Decision == string(tools.DecisionAllowed) || f.reads() != 0 {
		t.Fatalf("cost was aggregated without earnings permission: %s", out.Content)
	}
}

func TestExportProducesADownloadForItsOwner(t *testing.T) {
	f := newFixture(t)
	ph := pharmacist(1, 10)

	out := f.reg.Dispatch(context.Background(), ph, 0, call("export_data",
		`{"dataset":"purchase_orders","title":"طلباتي","format":"csv"}`))
	if out.Decision != string(tools.DecisionAllowed) {
		t.Fatalf("export refused: %s", out.Content)
	}
	if len(f.data.exports) != 1 || !strings.HasSuffix(f.data.exports[0].Filename, ".csv") {
		t.Fatalf("export not stored: %+v", f.data.exports)
	}
	var found bool
	for _, e := range out.Entities {
		if e.Kind == assistant.EntityExport && strings.HasPrefix(e.URL, "/api/v1/assistant/exports/") {
			found = true
		}
	}
	if !found {
		t.Fatalf("export entity missing: %+v", out.Entities)
	}
	if f.data.plans[0].Limit > 50000 {
		t.Fatalf("export limit not server-owned: %d", f.data.plans[0].Limit)
	}
}
