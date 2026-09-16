package datasets

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

func owner(scope rbac.Scope, orgID, userID int64) authctx.Actor {
	a := authctx.Actor{UserID: userID, OrgID: orgID, OrganizationID: orgID, Scope: scope, IsStaff: scope == rbac.ScopeAdmin}
	a.Grants(rbac.Default().KeysFor(scope))
	return a
}

func member(scope rbac.Scope, orgID, userID int64, perms ...string) authctx.Actor {
	a := authctx.Actor{UserID: userID, OrgID: orgID, OrganizationID: orgID, Scope: scope, IsStaff: scope == rbac.ScopeAdmin}
	a.Grants(perms)
	return a
}

var rowOpts = Options{MaxLimit: 200, DefaultLimit: 50, MaxOffset: 10000}

func compileOK(t *testing.T, a authctx.Actor, req Request, opts Options) *Plan {
	t.Helper()
	d, ok := Default().For(a, req.Dataset)
	if !ok {
		t.Fatalf("dataset %q not readable", req.Dataset)
	}
	p, err := Compile(a, d, req, opts)
	if err != nil {
		t.Fatalf("compile %+v: %v", req, err)
	}
	return p
}

func TestCatalogDeclarationsAreSound(t *testing.T) {
	cat := Default()
	perms := rbac.Default()
	if len(cat.All()) < 40 {
		t.Fatalf("catalogue unexpectedly small: %d datasets", len(cat.All()))
	}
	for _, d := range cat.All() {
		name := d.Name
		for _, p := range d.Permissions {
			perm, ok := perms.Lookup(p)
			if !ok {
				t.Errorf("%s: permission %q is not in the RBAC catalogue", name, p)
				continue
			}
			if !perm.InScope(d.Scope) {
				t.Errorf("%s: permission %q is not a %s permission", name, p, d.Scope)
			}
		}
		for _, f := range d.Fields {
			for _, p := range f.Permissions {
				if _, ok := perms.Lookup(p); !ok {
					t.Errorf("%s.%s: field permission %q unknown", name, f.Name, p)
				}
			}
		}
	}
}

// Columns no dataset may ever read, whatever a screen shows. A test rather than
// a comment because declarations are added by people who did not write this.
var forbiddenColumn = regexp.MustCompile(`\b(password_hash|delivery_code|destination_details|sender_account|stack_trace|request_payload|ai_virtual_key|ai_user_id|user_agent|ip_address|base_salary|variable_salary|attachment_url|transfer_receipt_url|shipping_address|token_hash|secret|mfa|totp)\b|\ba\.(before|after|ip)\b`)

func TestNoDatasetReadsASecretColumn(t *testing.T) {
	cat := Default()
	for _, d := range cat.All() {
		name := d.Name
		sources := []string{d.From, d.Tenant}
		if d.Key != nil {
			sources = append(sources, d.Key.SQL)
		}
		for _, f := range d.Fields {
			sources = append(sources, f.SQL)
		}
		for _, p := range d.Parents {
			sources = append(sources, p)
		}
		for _, src := range sources {
			if m := forbiddenColumn.FindString(src); m != "" {
				t.Errorf("%s reads forbidden column %q", name, m)
			}
		}
		if strings.Contains(d.From, "identity.user_security") || strings.Contains(d.From, "identity.user_sessions") ||
			strings.Contains(d.From, "telegram.") || strings.Contains(d.From, "payment_methods") {
			t.Errorf("%s joins a credential or session table", name)
		}
	}
}

func TestTenantDatasetsAreScopedToTheCaller(t *testing.T) {
	_, err := Build(Dataset{
		Name: "leaky", Scope: rbac.ScopePharmacy, Permissions: []string{"pharmacy.order.view"},
		From: "commerce.orders o", Tenant: "o.deleted_at IS NULL",
		Fields: []Field{text("number", "n", "o.order_number")},
	})
	if err == nil {
		t.Fatal("a pharmacy dataset without @org or @user must not build")
	}
	_, err = Build(Dataset{
		Name: "smuggled", Scope: rbac.ScopeVendor, Permissions: []string{"vendor.order.view"},
		From: "commerce.orders o", Tenant: "o.organization_id = @org OR @anything",
		Fields: []Field{text("number", "n", "o.order_number")},
	})
	if err == nil {
		t.Fatal("unknown tenant tokens must not build")
	}
}

func TestEveryDatasetCompilesInEveryMode(t *testing.T) {
	cat := Default()
	for _, d := range cat.All() {
		name := d.Name
		a := owner(d.Scope, 42, 7)
		if !Readable(a, d) {
			t.Fatalf("owner cannot read %s", name)
		}
		if _, err := Compile(a, d, Request{Dataset: name}, rowOpts); err != nil {
			t.Errorf("%s rows: %v", name, err)
		}
		if _, err := Compile(a, d, Request{Dataset: name, Search: "x"}, rowOpts); err != nil && !errors.Is(err, ErrInvalid) {
			t.Errorf("%s search: %v", name, err)
		}
		for _, f := range d.Fields {
			req := Request{Dataset: name}
			switch f.Type {
			case Text, Enum, Bool, Int:
				req.GroupBy = []string{f.Name}
			case Date, Time:
				req.GroupBy = []string{f.Name + ":month"}
			case Money, Number:
				req.Metrics = []string{"sum:" + f.Name, "avg:" + f.Name}
			default:
				continue
			}
			if _, err := Compile(a, d, req, rowOpts); err != nil {
				t.Errorf("%s aggregate on %s: %v", name, f.Name, err)
			}
		}
	}
}

func TestTenantPredicateBindsTheLiveCaller(t *testing.T) {
	a := owner(rbac.ScopePharmacy, 42, 7)
	p := compileOK(t, a, Request{Dataset: "cart_items"}, rowOpts)
	if !strings.Contains(p.SQL, "c.organization_id = $1 AND c.user_id = $2") {
		t.Fatalf("tenant predicate not rendered first with parameters: %s", p.SQL)
	}
	if p.Args[0] != int64(42) || p.Args[1] != int64(7) {
		t.Fatalf("tenant args = %v", p.Args[:2])
	}

	noOrg := owner(rbac.ScopePharmacy, 0, 7)
	d, _ := Default().Get(rbac.ScopePharmacy, "purchase_orders")
	if _, err := Compile(noOrg, d, Request{Dataset: "purchase_orders"}, rowOpts); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a caller without an organisation must be refused, got %v", err)
	}
}

func TestBuyingDatasetsTenantIsolation(t *testing.T) {
	for _, scope := range []rbac.Scope{rbac.ScopePharmacy, rbac.ScopeVendor} {
		a := owner(scope, 42, 7)
		poPlan := compileOK(t, a, Request{
			Dataset: "purchase_orders",
			Metrics: []string{"sum:total"},
		}, rowOpts)
		if !strings.Contains(poPlan.SQL, "o.organization_id = $1 OR o.customer_id = $2") {
			t.Fatalf("%s purchase_orders does not contain scoped tenant condition: %s", scope, poPlan.SQL)
		}
		if poPlan.Args[0] != int64(42) || poPlan.Args[1] != int64(7) {
			t.Fatalf("%s purchase_orders args mismatch: %v", scope, poPlan.Args)
		}

		polPlan := compileOK(t, a, Request{
			Dataset: "purchase_order_lines",
			GroupBy: []string{"supplier"},
			Metrics: []string{"sum:total"},
		}, rowOpts)
		if !strings.Contains(polPlan.SQL, "o.organization_id = $1 OR o.customer_id = $2") {
			t.Fatalf("%s purchase_order_lines does not contain scoped tenant condition: %s", scope, polPlan.SQL)
		}
		if polPlan.Args[0] != int64(42) || polPlan.Args[1] != int64(7) {
			t.Fatalf("%s purchase_order_lines args mismatch: %v", scope, polPlan.Args)
		}
	}
}

func TestValuesNeverReachTheSQLText(t *testing.T) {
	a := owner(rbac.ScopeVendor, 9, 3)
	payload := `x'); DROP TABLE commerce.orders; --`
	raw, _ := json.Marshal(payload)
	list, _ := json.Marshal([]string{payload, "ok"})
	p := compileOK(t, a, Request{
		Dataset: "sales_lines",
		Search:  payload,
		Filters: []Filter{
			{Field: "customer", Op: "contains", Value: raw},
			{Field: "product", Op: "in", Value: list},
			{Field: "sku", Op: "eq", Value: raw},
		},
	}, rowOpts)
	if strings.Contains(p.SQL, "DROP") || strings.Contains(p.SQL, "x'") {
		t.Fatalf("value leaked into SQL: %s", p.SQL)
	}
}

func TestNamesOutsideTheDeclarationAreRefused(t *testing.T) {
	a := owner(rbac.ScopePharmacy, 1, 1)
	d, _ := Default().Get(rbac.ScopePharmacy, "purchase_orders")
	bad := []Request{
		{Dataset: "purchase_orders", Fields: []string{"password_hash"}},
		{Dataset: "purchase_orders", Fields: []string{"o.total_amount"}},
		{Dataset: "purchase_orders", Filters: []Filter{{Field: "organization_id", Op: "eq", Value: json.RawMessage(`1`)}}},
		{Dataset: "purchase_orders", GroupBy: []string{"status; DROP"}},
		{Dataset: "purchase_orders", Metrics: []string{"sum:status"}},
		{Dataset: "purchase_orders", Metrics: []string{"stddev:total"}},
		{Dataset: "purchase_orders", Sort: []Sort{{By: "1"}}},
		{Dataset: "purchase_orders", Sort: []Sort{{By: "total", Dir: "desc; DELETE"}}},
		{Dataset: "purchase_orders", Filters: []Filter{{Field: "total", Op: "gt", Value: json.RawMessage(`"1 OR 1=1"`)}}},
		{Dataset: "purchase_orders", Filters: []Filter{{Field: "created_at", Op: "gte", Value: json.RawMessage(`"yesterday"`)}}},
		{Dataset: "purchase_orders", Filters: []Filter{{Field: "status", Op: "regex", Value: json.RawMessage(`".*"`)}}},
		{Dataset: "purchase_orders", Offset: -1},
		{Dataset: "purchase_orders", Fields: []string{"number"}, GroupBy: []string{"status"}},
	}
	for _, req := range bad {
		if _, err := Compile(a, d, req, rowOpts); !errors.Is(err, ErrInvalid) {
			t.Errorf("request %+v should be refused as invalid, got %v", req, err)
		}
	}
}

func TestDatasetsFollowDashboardAndPermission(t *testing.T) {
	// A supplier that buys reads its own purchases, under its own dashboard's
	// buying key — never through a pharmacy key.
	vendorBuyer := member(rbac.ScopeVendor, 5, 5, "vendor.buying.order.view")
	d, ok := Default().For(vendorBuyer, "purchase_orders")
	if !ok || d.Scope != rbac.ScopeVendor {
		t.Fatal("a supplier's buyer should read its own purchase orders")
	}
	if _, ok := Default().For(member(rbac.ScopeVendor, 5, 5, "pharmacy.order.view"), "purchase_orders"); ok {
		t.Fatal("a pharmacy key must not admit a vendor-dashboard caller")
	}
	if _, ok := Default().For(owner(rbac.ScopePharmacy, 5, 5), "sales_lines"); ok {
		t.Fatal("a pharmacy must not reach a supplier's sales")
	}
	clerk := member(rbac.ScopePharmacy, 5, 6, "pharmacy.cart.use")
	if _, ok := Default().For(clerk, "purchase_orders"); ok {
		t.Fatal("an employee without order view must not read orders")
	}
	if _, ok := Default().For(clerk, "cart_items"); !ok {
		t.Fatal("cart permission should admit the cart dataset")
	}
	pharmacyOwner := owner(rbac.ScopePharmacy, 5, 5)
	if _, ok := Default().For(pharmacyOwner, "users"); ok {
		t.Fatal("a pharmacy must not reach platform users")
	}
}

func TestFieldPermissionHidesTheField(t *testing.T) {
	seller := member(rbac.ScopeVendor, 5, 6, "vendor.order.view")
	d, _ := Default().Get(rbac.ScopeVendor, "sales_lines")
	if _, err := Compile(seller, d, Request{Dataset: "sales_lines", Fields: []string{"unit_cost"}}, rowOpts); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cost must be refused without earnings permission, got %v", err)
	}
	if _, err := Compile(seller, d, Request{Dataset: "sales_lines", Metrics: []string{"sum:unit_cost"}}, rowOpts); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cost must not be aggregatable without permission, got %v", err)
	}
	for _, f := range Describe(seller, d)["fields"].([]map[string]any) {
		if f["name"] == "unit_cost" {
			t.Fatal("describe must not list a field the caller cannot see")
		}
	}
	accountant := member(rbac.ScopeVendor, 5, 6, "vendor.order.view", "vendor.earnings.view")
	if _, err := Compile(accountant, d, Request{Dataset: "sales_lines", Fields: []string{"unit_cost"}}, rowOpts); err != nil {
		t.Fatalf("earnings permission should admit cost: %v", err)
	}
}

func TestAggregateOrdersByTheRawValue(t *testing.T) {
	a := owner(rbac.ScopePharmacy, 1, 1)
	p := compileOK(t, a, Request{
		Dataset: "purchase_order_lines",
		GroupBy: []string{"supplier"},
		Metrics: []string{"sum:total", "count"},
		Sort:    []Sort{{By: "sum:total", Dir: "desc"}},
	}, rowOpts)
	if !strings.Contains(p.SQL, "ORDER BY sum((l.total_price)) DESC") {
		t.Fatalf("sums must sort numerically, not as text: %s", p.SQL)
	}
	if !strings.Contains(p.SQL, "GROUP BY") || len(p.Columns) != 3 {
		t.Fatalf("unexpected aggregate plan: %s %+v", p.SQL, p.Columns)
	}
}

func TestTimeFiltersUseWholeCairoDays(t *testing.T) {
	a := owner(rbac.ScopePharmacy, 1, 1)
	p := compileOK(t, a, Request{
		Dataset: "purchase_orders",
		Filters: []Filter{{Field: "created_at", Op: "between", Value: json.RawMessage(`["2025-01-01","2025-01-31"]`)}},
	}, rowOpts)
	got := strings.Join(argStrings(p.Args), "|")
	if !strings.Contains(got, "2025-01-01 00:00:00") || !strings.Contains(got, "2025-02-01 00:00:00") {
		t.Fatalf("between must cover the whole last day, args %s", got)
	}
	if !strings.Contains(p.SQL, "AT TIME ZONE 'Africa/Cairo'") {
		t.Fatalf("bounds must be Cairo local: %s", p.SQL)
	}
}

func TestRefFiltersRequireAVerifiedHandle(t *testing.T) {
	a := owner(rbac.ScopePharmacy, 1, 1)
	d, _ := Default().Get(rbac.ScopePharmacy, "purchase_order_lines")
	opts := rowOpts
	opts.ResolveRef = func(kind handles.Kind, token string) (int64, error) {
		if kind == handles.KindOrder && token == "good" {
			return 77, nil
		}
		return 0, errors.New("forged")
	}
	req := Request{Dataset: d.Name, Filters: []Filter{{Field: "order_ref", Op: "eq", Value: json.RawMessage(`"good"`)}}}
	p, err := Compile(a, d, req, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !containsArg(p.Args, []int64{77}) {
		t.Fatalf("resolved id not bound: %v", p.Args)
	}
	req.Filters[0].Value = json.RawMessage(`"123"`)
	if _, err := Compile(a, d, req, opts); !errors.Is(err, ErrHandle) {
		t.Fatalf("a raw id must be refused as a bad reference, got %v", err)
	}
}

func TestLimitsAreServerOwned(t *testing.T) {
	a := owner(rbac.ScopePharmacy, 1, 1)
	p := compileOK(t, a, Request{Dataset: "purchase_orders", Limit: 1000000}, rowOpts)
	if p.Limit != 200 {
		t.Fatalf("limit = %d, want the ceiling 200", p.Limit)
	}
	d, _ := Default().Get(rbac.ScopePharmacy, "purchase_orders")
	if _, err := Compile(a, d, Request{Dataset: "purchase_orders", Offset: 20000}, rowOpts); !errors.Is(err, ErrInvalid) {
		t.Fatal("offset beyond the ceiling must be refused")
	}
}

func TestParentAndKeyRestrictions(t *testing.T) {
	a := owner(rbac.ScopeVendor, 3, 3)
	opts := rowOpts
	opts.ParentKind, opts.ParentID = handles.KindShipment, 55
	p := compileOK(t, a, Request{Dataset: "sales_lines"}, opts)
	if !strings.Contains(p.SQL, "(l.shipment_id) = $") || !containsArg(p.Args, int64(55)) {
		t.Fatalf("parent restriction missing: %s %v", p.SQL, p.Args)
	}
	opts = rowOpts
	opts.ParentKind, opts.ParentID = handles.KindOrder, 55
	d, _ := Default().Get(rbac.ScopeVendor, "sales_lines")
	if _, err := Compile(a, d, Request{Dataset: "sales_lines"}, opts); !errors.Is(err, ErrInvalid) {
		t.Fatal("an unrelated parent kind must be refused")
	}
}

func argStrings(args []any) []string {
	out := make([]string, len(args))
	for i, a := range args {
		b, _ := json.Marshal(a)
		out[i] = string(b)
	}
	return out
}

func containsArg(args []any, want any) bool {
	w, _ := json.Marshal(want)
	for _, a := range args {
		if b, _ := json.Marshal(a); string(b) == string(w) {
			return true
		}
	}
	return false
}
