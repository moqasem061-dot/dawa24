package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/tools"
)

// ---------------------------------------------------------------------------
// Arguments
// ---------------------------------------------------------------------------

// No tool may accept an identity parameter. This walks the whole registry, so
// a tool added later inherits the rule rather than needing its own test.
func TestNoToolAcceptsAnIdentityArgument(t *testing.T) {
	f := newFixture(t)
	banned := []string{
		"org_id", "organization_id", "org", "organisation_id", "tenant", "tenant_id",
		"user_id", "user", "customer_id", "owner_id", "branch_id", "account_id",
		"is_admin", "is_staff", "role", "scope", "permission", "permissions",
	}

	for _, name := range f.reg.Names() {
		tool, ok := f.reg.Lookup(name)
		if !ok {
			t.Fatalf("declared tool %s cannot be looked up", name)
		}
		props, _ := tool.Params["properties"].(map[string]any)
		for field := range props {
			lower := strings.ToLower(field)
			for _, bad := range banned {
				if lower == bad {
					t.Errorf("tool %s accepts identity argument %q", name, field)
				}
			}
		}
		// additionalProperties:false is what turns a smuggled field into a
		// visible refusal instead of a silently dropped one.
		if extra, ok := tool.Params["additionalProperties"].(bool); !ok || extra {
			t.Errorf("tool %s does not forbid additional properties", name)
		}
	}
}

// A smuggled argument is refused rather than ignored.
func TestUnknownArgumentsAreRefused(t *testing.T) {
	f := newFixture(t)
	ph := pharmacist(1, 10)

	bad := []string{
		`{"dataset":"purchase_orders","organization_id": 99}`,
		`{"dataset":"purchase_orders","limit": 5, "org_id": 2}`,
		`{"dataset":"purchase_orders","__proto__":{"admin":true}}`,
		`{"dataset":"purchase_orders","filters":[{"field":"status","op":"eq","value":"x","sql":"1=1"}]}`,
		`{"dataset":"purchase_orders"}{"dataset":"sales_lines"}`,
	}
	for _, args := range bad {
		out := f.reg.Dispatch(context.Background(), ph, 0, call("query_data", args))
		if out.Decision == string(tools.DecisionAllowed) {
			t.Fatalf("accepted smuggled arguments: %s", args)
		}
	}
	if f.reads() != 0 {
		t.Fatal("invalid arguments reached the database")
	}
}

// Out-of-range and nonsense values are refused, not clamped into a confidently
// wrong answer.
func TestArgumentValidation(t *testing.T) {
	f := newFixture(t)
	ph := pharmacist(1, 10)

	cases := map[string]string{
		"negative offset":  `{"dataset":"purchase_orders","offset": -5}`,
		"absurd offset":    `{"dataset":"purchase_orders","offset": 500000}`,
		"unknown field":    `{"dataset":"purchase_orders","fields":["password_hash"]}`,
		"bad date":         `{"dataset":"purchase_orders","filters":[{"field":"created_at","op":"gte","value":"last tuesday"}]}`,
		"reversed period":  `{"dataset":"purchase_orders","filters":[{"field":"created_at","op":"between","value":["2026-06-01","2026-01-01"]}]}`,
		"oversized search": `{"dataset":"purchase_orders","search": "` + strings.Repeat("ا", 400) + `"}`,
		"unknown metric":   `{"dataset":"purchase_orders","metrics":["stddev:total"]}`,
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			out := f.reg.Dispatch(context.Background(), ph, 0, call("query_data", args))
			if out.Decision == string(tools.DecisionAllowed) {
				t.Fatalf("accepted %s", name)
			}
		})
	}
}

// A model that asks for a thousand rows gets a page.
func TestPageSizeIsCapped(t *testing.T) {
	f := newFixture(t)
	ph := pharmacist(1, 10)

	f.reg.Dispatch(context.Background(), ph, 0, call("query_data", `{"dataset":"purchase_orders","limit": 5000}`))
	for _, p := range f.data.plans {
		if p.Limit > 200 {
			t.Fatalf("row limit %d passed the ceiling", p.Limit)
		}
	}
}

// ---------------------------------------------------------------------------
// Unknown tools and injection
// ---------------------------------------------------------------------------

// An invented tool name must be refused outright, never resolved to the
// nearest real one.
func TestUnknownToolsAreRefused(t *testing.T) {
	f := newFixture(t)
	ph := pharmacist(1, 10)

	for _, name := range []string{
		"", "sql", "execute_sql", "query_data_all", "QUERY_DATA",
		"admin_query_data", "../query_data", "run_sql",
	} {
		out := f.reg.Dispatch(context.Background(), ph, 0, call(name, `{"dataset":"purchase_orders"}`))
		if out.Decision == string(tools.DecisionAllowed) {
			t.Fatalf("invented tool %q was allowed", name)
		}
	}
}

// A refusal must not tell the model anything it can act on: no SQL, no column
// names, no tenant identifiers.
func TestRefusalsLeakNothing(t *testing.T) {
	f := newFixture(t)
	attacker := pharmacist(2, 20)
	stolen := f.signer.Issue(handles.KindOrder, 501, handles.Binding{OrgID: 1, UserID: 10})
	args, _ := json.Marshal(map[string]string{"ref": stolen})

	out := f.reg.Dispatch(context.Background(), attacker, 0, call("get_record", string(args)))
	lower := strings.ToLower(out.Content)
	for _, leak := range []string{"select", "commerce.", "organization_id", "pgx", "sql", "501"} {
		if strings.Contains(lower, leak) {
			t.Fatalf("refusal leaked %q: %s", leak, out.Content)
		}
	}
}

// Every decision, allowed or not, is recorded.
func TestEveryDecisionIsAudited(t *testing.T) {
	f := newFixture(t)
	ph := pharmacist(1, 10)

	f.reg.Dispatch(context.Background(), ph, 77, call("query_data", `{"dataset":"branches"}`))
	f.reg.Dispatch(context.Background(), ph, 77, call("inventory_health", "{}"))
	f.reg.Dispatch(context.Background(), ph, 77, call("nope", "{}"))

	if len(f.audit.entries) != 3 {
		t.Fatalf("audited %d calls, want 3", len(f.audit.entries))
	}
	for _, e := range f.audit.entries {
		if e.TurnID != 77 || e.UserID != ph.UserID {
			t.Fatalf("audit entry not attributed: %+v", e)
		}
	}
}

// ---------------------------------------------------------------------------
// Read-only
// ---------------------------------------------------------------------------

// The assistant must offer no way to change anything. This reads the registry
// rather than the source, so a mutating tool added later fails here.
func TestNoToolNameSuggestsAWrite(t *testing.T) {
	f := newFixture(t)
	forbidden := []string{
		"create", "update", "delete", "remove", "cancel", "place", "submit",
		"pay_", "refund", "confirm", "approve", "reject", "set_", "add_", "edit",
	}
	for _, name := range f.reg.Names() {
		lower := strings.ToLower(name)
		for _, verb := range forbidden {
			if strings.Contains(lower, verb) {
				t.Errorf("tool %q reads as a mutation; the assistant is read-only", name)
			}
		}
	}
}
