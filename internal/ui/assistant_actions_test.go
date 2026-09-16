package ui

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/assistant/actions"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/evals"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Capsule's actions must be reachable by exactly the people the dashboard
// screen that performs them is reachable by. These tests hold each command to
// its screen's route: the same permission keys, the same audience, the same
// approval rule.

// commandRoutes names the dashboard route each command is the assistant's copy
// of. A new command without an entry fails TestEveryCommandMatchesItsRouteGuard.
var commandRoutes = map[string]string{
	"cart_add":                 "POST /cart/add",
	"offer_add":                "POST /cart/add-offer",
	"cart_set_quantity":        "POST /cart/update-quantity",
	"cart_remove":              "POST /cart/remove",
	"place_order":              "POST /checkout",
	"order_cancel":             "POST /orders/{id}/cancel",
	"favorite_add":             "POST /favorites/{id}/add",
	"favorite_remove":          "POST /favorites/{id}/remove",
	"shipment_update_status":   "POST /vendor/orders/{id}/status",
	"negotiation_accept":       "POST /vendor/orders/{id}/negotiation/accept",
	"negotiation_reject":       "POST /vendor/orders/{id}/negotiation/reject",
	"purchase_request_respond": "POST /vendor/purchase-requests/{id}/respond",
	"listing_update":           "POST /vendor/variants/{id}/update",
	"stock_adjust":             "POST /vendor/inventory/{id}/adjust",
	"organization_approve":     "POST /admin/organizations/{id}/approve",
	"organization_reject":      "POST /admin/organizations/{id}/reject",
	"issue_update":             "POST /admin/report-issues/{id}/status",
}

var (
	auditGuardRe = regexp.MustCompile(`\bRequire[A-Za-z0-9_]*\(([^)]*)\)`)
	auditRouteRe = regexp.MustCompile(`\b[a-zA-Z_][a-zA-Z0-9_]*\.(Get|Post|Put|Delete|Patch)\(\s*"(/[^"]*)"`)
	quotedRe     = regexp.MustCompile(`"([^"]+)"`)
	capRe        = regexp.MustCompile(`rbac\.([A-Za-z0-9_]+)`)
)

// routeGuards maps "METHOD /path" to the argument text of the innermost
// permission guard around it.
func routeGuards(t *testing.T) map[string]string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || strings.HasSuffix(f, "_templ.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var stack []string
		for _, line := range strings.Split(string(src), "\n") {
			if m := auditGuardRe.FindStringSubmatch(line); m != nil && len(stack) > 0 {
				stack[len(stack)-1] = m[1]
			}
			if m := auditRouteRe.FindStringSubmatch(line); m != nil {
				for i := len(stack) - 1; i >= 0; i-- {
					if stack[i] != "" {
						out[strings.ToUpper(m[1])+" "+m[2]] = stack[i]
						break
					}
				}
			}
			opens := strings.Count(line, "{") - strings.Count(line, "}")
			for i := 0; i < opens; i++ {
				stack = append(stack, "")
			}
			for i := 0; i > opens && len(stack) > 0; i-- {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return out
}

var capabilitiesByIdent = map[string]rbac.Capability{
	"BuyCartUse": rbac.BuyCartUse, "BuyOrderCreate": rbac.BuyOrderCreate, "BuyOrderUpdate": rbac.BuyOrderUpdate,
	"BuyFavoriteView": rbac.BuyFavoriteView, "BuyFavoriteManage": rbac.BuyFavoriteManage,
}

func TestEveryCommandMatchesItsRouteGuard(t *testing.T) {
	h := &UIHandler{log: slog.New(slog.NewTextHandler(os.Stderr, nil))}
	cmds := h.AssistantActions().commands
	guards := routeGuards(t)

	for name, cmd := range cmds {
		route, ok := commandRoutes[name]
		if !ok {
			t.Errorf("command %s has no dashboard route in commandRoutes", name)
			continue
		}
		guard, ok := guards[route]
		if !ok {
			t.Errorf("command %s: route %s not found or not guarded", name, route)
			continue
		}
		switch cmd.audience {
		case audienceBuyer:
			var want []string
			for _, m := range capRe.FindAllStringSubmatch(guard, -1) {
				c, ok := capabilitiesByIdent[m[1]]
				if !ok {
					t.Fatalf("command %s: add rbac.%s to capabilitiesByIdent", name, m[1])
				}
				want = append(want, c.Name)
			}
			var got []string
			for _, c := range cmd.caps {
				got = append(got, c.Name)
			}
			if !sameSet(got, want) {
				t.Errorf("command %s requires %v, its route %s requires %v", name, got, route, want)
			}
		default:
			var want []string
			for _, m := range quotedRe.FindAllStringSubmatch(guard, -1) {
				want = append(want, m[1])
			}
			if !sameSet(cmd.keys, want) {
				t.Errorf("command %s requires %v, its route %s requires %v", name, cmd.keys, route, want)
			}
		}
	}
	for name := range commandRoutes {
		if _, ok := cmds[name]; !ok {
			t.Errorf("commandRoutes names %s, which is not a command", name)
		}
	}
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	a, b = append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(a)
	sort.Strings(b)
	return strings.Join(a, ",") == strings.Join(b, ",")
}

func member(scope rbac.Scope, status string, perms ...string) authctx.Actor {
	a := authctx.Actor{UserID: 5, OrgID: 9, OrganizationID: 9, Scope: scope, OrgStatus: status, IsStaff: scope == rbac.ScopeAdmin}
	switch scope {
	case rbac.ScopePharmacy:
		a.OrgType = "customer"
	case rbac.ScopeVendor:
		a.OrgType = "vendor"
	}
	a.Grants(perms)
	return a
}

func TestCommandsFollowAudienceApprovalAndPermission(t *testing.T) {
	a := (&UIHandler{}).AssistantActions()

	cases := []struct {
		name  string
		actor authctx.Actor
		cmd   string
		want  bool
	}{
		{"pharmacy with cart grant", member(rbac.ScopePharmacy, "approved", "pharmacy.cart.use"), "cart_add", true},
		{"pharmacy without cart grant", member(rbac.ScopePharmacy, "approved", "pharmacy.order.view"), "cart_add", false},
		{"pending pharmacy", member(rbac.ScopePharmacy, "pending", "pharmacy.cart.use"), "cart_add", false},
		{"suspended pharmacy", member(rbac.ScopePharmacy, "suspended", "pharmacy.order.create"), "place_order", false},
		{"supplier buying with its own key", member(rbac.ScopeVendor, "approved", "vendor.buying.cart.use"), "cart_add", true},
		{"supplier holding a pharmacy key", member(rbac.ScopeVendor, "approved", "pharmacy.cart.use"), "cart_add", false},
		{"supplier fulfilling", member(rbac.ScopeVendor, "approved", "vendor.order.update"), "shipment_update_status", true},
		{"pharmacy asking a supplier command", member(rbac.ScopePharmacy, "approved", "vendor.order.update"), "shipment_update_status", false},
		{"staff approving", member(rbac.ScopeAdmin, "", "org.approval.decide"), "organization_approve", true},
		{"staff without the decision grant", member(rbac.ScopeAdmin, "", "org.approval.view"), "organization_approve", false},
		{"supplier asking a staff command", member(rbac.ScopeVendor, "approved", "org.approval.decide"), "organization_approve", false},
		{"unknown command", member(rbac.ScopePharmacy, "approved", "pharmacy.cart.use"), "drop_tables", false},
	}
	for _, tc := range cases {
		if got := a.Permitted(tc.actor, tc.cmd); got != tc.want {
			t.Errorf("%s: Permitted(%s) = %v, want %v", tc.name, tc.cmd, got, tc.want)
		}
	}
}

func TestUnpermittedCommandsNeverRun(t *testing.T) {
	a := (&UIHandler{}).AssistantActions()
	outsider := member(rbac.ScopePharmacy, "approved", "pharmacy.order.view")
	if _, err := a.Prepare(context.Background(), outsider, "place_order", actions.Args{}); !errors.Is(err, actions.ErrNotAllowed) {
		t.Fatalf("prepare: %v", err)
	}
	if _, err := a.Execute(context.Background(), outsider, "place_order", actions.Args{}); !errors.Is(err, actions.ErrNotAllowed) {
		t.Fatalf("execute: %v", err)
	}
}

func TestDefinitionsFollowTheDashboard(t *testing.T) {
	a := (&UIHandler{}).AssistantActions()
	names := func(actor authctx.Actor) map[string]bool {
		out := map[string]bool{}
		for _, d := range a.Definitions(actor) {
			out[d.Name] = true
		}
		return out
	}
	pharmacy := names(member(rbac.ScopePharmacy, "approved"))
	if !pharmacy["place_order"] || pharmacy["stock_adjust"] || pharmacy["organization_approve"] {
		t.Fatalf("pharmacy definitions: %v", pharmacy)
	}
	vendor := names(member(rbac.ScopeVendor, "approved"))
	if !vendor["stock_adjust"] || !vendor["cart_add"] || vendor["issue_update"] {
		t.Fatalf("vendor definitions: %v", vendor)
	}
	staff := names(member(rbac.ScopeAdmin, ""))
	if !staff["issue_update"] || staff["cart_add"] {
		t.Fatalf("staff definitions: %v", staff)
	}
}

func TestEgpFormatsForCards(t *testing.T) {
	for in, want := range map[string]string{"1250.50": "1,250.50 ج.م", "7.00": "7 ج.م", "1000000.00": "1,000,000 ج.م"} {
		amount, _ := money.Parse(in)
		if got := egp(amount); got != want {
			t.Errorf("egp(%s) = %q, want %q", in, got, want)
		}
	}
}

// Every action the eval corpus expects Capsule to prepare must be a command the
// dashboard offers.
func TestCorpusActionsAreCommands(t *testing.T) {
	cases, err := evals.Load()
	if err != nil {
		t.Fatal(err)
	}
	cmds := (&UIHandler{}).AssistantActions().commands
	for _, c := range cases {
		if c.Action == "" {
			continue
		}
		if _, ok := cmds[c.Action]; !ok {
			t.Errorf("%s expects action %q, which is not a command", c.ID, c.Action)
		}
	}
}
