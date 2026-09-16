package postgres_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/actions"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/datasets"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	assistantPostgres "github.com/muhiya/dawa24-store/internal/modules/assistant/postgres"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/tools"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// The cross-tenant proof for the dataset engine, against a real schema.
//
// Two trading pairs are seeded: pharmacy A buys from supplier V1, pharmacy B
// buys from supplier V2. Every text value on the A/V1 side carries markA,
// every value on the B/V2 side markB. Then every pharmacy dataset is read as A,
// and every supplier dataset as V1 — all fields, rows and groups, through the
// real tool registry and the read-only dataset role — and not one markB may
// come back. Nothing on the A/V1 side ever legitimately references B or V2, so
// a single markB is a leak.
//
// TEST_DATABASE_URL only: this writes fixture rows.

type seed struct {
	t   *testing.T
	ctx context.Context
	db  *database.DB
}

func (s seed) id(sql string, args ...any) int64 {
	s.t.Helper()
	var id int64
	if err := s.db.Pool().QueryRow(s.ctx, sql, args...).Scan(&id); err != nil {
		s.t.Fatalf("%s: %v", sql, err)
	}
	return id
}

type side struct {
	buyer, vendor, buyerUser, vendorUser, buyerBranch, vendorBranch int64
	order, variant, request, warehouse                              int64
}

func jsonName(v string) string {
	b, _ := json.Marshal(map[string]string{"ar": v, "en": v})
	return string(b)
}

func seedSide(s seed, mark string, plan int64) side {
	var x side
	name := func(what string) string { return mark + " " + what }
	x.buyerUser = s.id(`INSERT INTO identity.users (email, password_hash, name, role, status) VALUES ($1,'x',$2,'customer','active') RETURNING id`,
		strings.ToLower(mark)+"-buyer@example.test", jsonName(name("buyer user")))
	x.vendorUser = s.id(`INSERT INTO identity.users (email, password_hash, name, role, status) VALUES ($1,'x',$2,'customer','active') RETURNING id`,
		strings.ToLower(mark)+"-vendor@example.test", jsonName(name("vendor user")))
	x.buyer = s.id(`INSERT INTO org.organizations (name, trade_name, legal_name, type, status, email, phone) VALUES ($1,$1,$2,'customer','approved',$3,$2) RETURNING id`,
		jsonName(name("pharmacy")), name("legal"), strings.ToLower(mark)+"-ph@example.test")
	x.vendor = s.id(`INSERT INTO org.organizations (name, trade_name, legal_name, type, status, email, phone) VALUES ($1,$1,$2,'vendor','approved',$3,$2) RETURNING id`,
		jsonName(name("supplier")), name("legal"), strings.ToLower(mark)+"-vd@example.test")
	for _, m := range [][2]int64{{x.buyer, x.buyerUser}, {x.vendor, x.vendorUser}} {
		s.id(`INSERT INTO org.members (organization_id, user_id, role_key, status, is_active, job_title) VALUES ($1,$2,'org_owner','active',true,$3) RETURNING id`, m[0], m[1], name("job"))
	}
	x.buyerBranch = s.id(`INSERT INTO org.branches (organization_id, name, address, code, is_main, status) VALUES ($1,$2,$3,$3,true,'active') RETURNING id`,
		x.buyer, jsonName(name("branch")), name("addr"))
	x.vendorBranch = s.id(`INSERT INTO org.branches (organization_id, name, address, code, is_main, status) VALUES ($1,$2,$3,$3,true,'active') RETURNING id`,
		x.vendor, jsonName(name("vbranch")), name("vaddr"))

	product := s.id(`INSERT INTO catalog.products (organization_id, name, sku, scientific_name, company, status) VALUES ($1,$2,$3,$3,$3,'active') RETURNING id`,
		x.vendor, jsonName(name("product")), name("sku"))
	x.variant = s.id(`INSERT INTO catalog.product_variants (organization_id, product_id, name, sku, barcode, price, discount, status, branch_id, batch_number)
		VALUES ($1,$2,$3,$4,$4,100,5,'active',$5,$4) RETURNING id`, x.vendor, product, jsonName(name("variant")), name("vsku"), x.vendorBranch)
	x.warehouse = s.id(`INSERT INTO inventory.warehouses (organization_id, branch_id, name, code, address) VALUES ($1,$2,$3,$3,$3) RETURNING id`,
		x.vendor, x.vendorBranch, name("warehouse"))
	stock := s.id(`INSERT INTO inventory.stocks (organization_id, warehouse_id, product_id, product_variant_id, quantity, min_threshold) VALUES ($1,$2,$3,$4,50,10) RETURNING id`,
		x.vendor, x.warehouse, product, x.variant)
	s.id(`INSERT INTO inventory.stock_movements (organization_id, stock_id, type, quantity_delta, balance_after, details, reference_type, user_id) VALUES ($1,$2,'in',50,50,$3,$3,$4) RETURNING id`,
		x.vendor, stock, name("movement"), x.vendorUser)
	second := s.id(`INSERT INTO inventory.warehouses (organization_id, branch_id, name, code) VALUES ($1,$2,$3,$3) RETURNING id`,
		x.vendor, x.vendorBranch, name("warehouse two"))
	s.id(`INSERT INTO inventory.warehouse_transfers (organization_id, from_warehouse_id, to_warehouse_id, product_id, product_variant_id, quantity, status, notes) VALUES ($1,$2,$3,$4,$5,1,'pending',$6) RETURNING id`,
		x.vendor, x.warehouse, second, product, x.variant, name("transfer"))

	x.order = s.id(`INSERT INTO commerce.orders (order_number, customer_id, organization_id, branch_id, status, payment_status, payment_method, subtotal, total_amount, notes, negotiation_status)
		VALUES ($1,$2,$3,$4,'pending','unpaid','cod',95,95,$1,'pending') RETURNING id`, name("order"), x.buyerUser, x.buyer, x.buyerBranch)
	shipment := s.id(`INSERT INTO commerce.order_shipments (order_id, organization_id, shipment_number, status, subtotal, total_amount, tracking_number, carrier_name)
		VALUES ($1,$2,$3,'pending',95,95,$3,$3) RETURNING id`, x.order, x.vendor, name("shipment"))
	s.id(`INSERT INTO commerce.order_lines (order_id, shipment_id, organization_id, product_id, product_variant_id, product_name, variant_name, sku, unit_price, quantity, total_price)
		VALUES ($1,$2,$3,$4,$5,$6,$6,$7,95,1,95) RETURNING id`, x.order, shipment, x.vendor, product, x.variant, jsonName(name("line")), name("lsku"))
	s.id(`INSERT INTO commerce.order_status_history (order_id, shipment_id, from_status, to_status, notes) VALUES ($1,$2,'pending','pending',$3) RETURNING id`,
		x.order, shipment, name("history"))
	s.id(`INSERT INTO billing.invoices (organization_id, customer_org_id, order_id, invoice_number, due_date, total_amount, status, payment_method, notes)
		VALUES ($1,$2,$3,$4,now()::date,95,'issued',$4,$4) RETURNING id`, x.vendor, x.buyer, x.order, name("invoice"))
	s.id(`INSERT INTO billing.payments (order_id, user_id, organization_id, amount, method, status, reference_number, notes) VALUES ($1,$2,$3,95,$4,'paid',$4,$4) RETURNING id`,
		x.order, x.buyerUser, x.buyer, name("pay"))
	for _, w := range [][2]int64{{x.buyerUser, x.buyer}, {x.vendorUser, x.vendor}} {
		wallet := s.id(`INSERT INTO billing.wallets (user_id, organization_id) VALUES ($1,$2) RETURNING id`, w[0], w[1])
		s.id(`INSERT INTO billing.wallet_transactions (wallet_id, type, amount, balance_after, description, reference_type) VALUES ($1,'deposit',10,10,$2,$2) RETURNING id`, wallet, name("wallet tx"))
		s.id(`INSERT INTO billing.wallet_withdrawals (wallet_id, user_id, organization_id, amount, status, payout_method_type, rejection_reason, destination_details)
			VALUES ($1,$2,$3,1,'pending',$4,$4,$4) RETURNING id`, wallet, w[0], w[1], name("withdrawal"))
	}
	x.request = s.id(`INSERT INTO commerce.purchase_requests (request_number, customer_id, organization_id, branch_id, vendor_org_id, status, buyer_notes, vendor_notes)
		VALUES ($1,$2,$3,$4,$5,'pending',$6,$6) RETURNING id`, name("request"), x.buyerUser, x.buyer, x.buyerBranch, x.vendor, name("request notes"))
	s.id(`INSERT INTO commerce.purchase_request_lines (request_id, product_id, product_name, product_sku, quantity, status, notes) VALUES ($1,$2,$3,$3,1,'pending',$3) RETURNING id`,
		x.request, product, name("request line"))
	s.id(`INSERT INTO commerce.quote_requests (organization_id, customer_org_id, product_id, product_name, requested_quantity, status, buyer_notes, supplier_notes)
		VALUES ($1,$2,$3,$4,1,'pending',$4,$4) RETURNING id`, x.vendor, x.buyer, product, name("quote"))
	cart := s.id(`INSERT INTO commerce.carts (user_id, organization_id) VALUES ($1,$2) RETURNING id`, x.buyerUser, x.buyer)
	s.id(`INSERT INTO commerce.cart_items (cart_id, product_id, product_variant_id, quantity, unit_price) VALUES ($1,$2,$3,1,95) RETURNING id`, cart, product, x.variant)
	s.id(`INSERT INTO promo.offers (organization_id, title, description, expires_at, discount_type, discount_value) VALUES ($1,$2,$2,now()+interval '1 day','percentage',5) RETURNING id`,
		x.vendor, jsonName(name("offer")))
	s.id(`INSERT INTO org.organization_reviews (organization_id, user_id, reviewer_org_id, rating, title, review_text, response, status) VALUES ($1,$2,$3,5,$4,$4,$4,'approved') RETURNING id`,
		x.vendor, x.buyerUser, x.buyer, name("review"))
	s.id(`INSERT INTO workflow.weekly_coverages (organization_id, branch_id, day_of_week, address) VALUES ($1,$2,1,$3) RETURNING id`, x.vendor, x.vendorBranch, name("coverage"))
	s.id(`INSERT INTO smartorder.runs (run_number, organization_id, user_id, branch_id, original_filename, status) VALUES ($1,$2,$3,$4,$1,'draft') RETURNING id`,
		name("run"), x.buyer, x.buyerUser, x.buyerBranch)
	for _, o := range [][2]int64{{x.buyerUser, x.buyer}, {x.vendorUser, x.vendor}} {
		s.id(`INSERT INTO billing.subscriptions (user_id, organization_id, plan_id, status, expires_at) VALUES ($1,$2,$3,'active',now()+interval '30 days') RETURNING id`, o[0], o[1], plan)
	}
	return x
}

func ownerActor(scope rbac.Scope, orgID, userID int64) authctx.Actor {
	a := authctx.Actor{UserID: userID, OrgID: orgID, OrganizationID: orgID, Scope: scope, OrgStatus: "approved"}
	a.OrgType = map[rbac.Scope]string{rbac.ScopePharmacy: "customer", rbac.ScopeVendor: "vendor"}[scope]
	a.Grants(rbac.Default().KeysFor(scope))
	return a
}

func openTestDB(t *testing.T) *database.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" || testing.Short() {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := database.Open(context.Background(), config.Database{URL: url, MaxConns: 8, MinConns: 1,
		MaxConnLifetime: time.Hour, MaxConnIdleTime: time.Minute, StatementTimeout: 30 * time.Second})
	if err != nil {
		t.Skipf("cannot connect: %v", err)
	}
	return db
}

func TestDatasetsNeverCrossTenants(t *testing.T) {
	db := openTestDB(t)
	ctx := database.AsSystem(context.Background())
	stamp := time.Now().UnixNano()
	markA, markB := fmt.Sprintf("ZXA%d", stamp), fmt.Sprintf("ZXB%d", stamp)

	s := seed{t: t, ctx: ctx, db: db}
	plan := s.id(`INSERT INTO billing.plans (slug, name) VALUES ($1,$2) RETURNING id`, fmt.Sprintf("capsule-%d", stamp), jsonName("plan"))
	a := seedSide(s, markA, plan)
	seedSide(s, markB, plan)

	repo := assistantPostgres.NewRepository(db)
	reg := tools.NewRegistry(repo, handles.NewSigner("capsule-integration-secret-0123456789"), repo, nil)
	reg.SetDatasets(datasets.Default(), repo, repo)

	dispatch := func(actor authctx.Actor, name string, args any) assistant.ToolOutcome {
		raw, _ := json.Marshal(args)
		return reg.Dispatch(authctx.WithActor(database.WithTenant(context.Background(), actor.OrgID), actor), actor, 0,
			gateway.ToolCall{ID: "c", Name: name, Arguments: string(raw)})
	}

	cases := []struct {
		actor authctx.Actor
		scope rbac.Scope
		must  []string // datasets that must show the actor's own seeded rows
	}{
		{ownerActor(rbac.ScopePharmacy, a.buyer, a.buyerUser), rbac.ScopePharmacy,
			[]string{"purchase_orders", "purchase_order_lines", "purchase_shipments", "purchase_invoices", "cart_items", "purchase_requests", "branches"}},
		{ownerActor(rbac.ScopeVendor, a.vendor, a.vendorUser), rbac.ScopeVendor,
			[]string{"sales_orders", "sales_lines", "catalog_listings", "stock_levels", "stock_movements", "warehouses", "incoming_purchase_requests", "quote_requests", "sales_invoices", "offers", "customer_reviews", "withdrawals"}},
	}
	for _, tc := range cases {
		for _, d := range datasets.Default().All() {
			if d.Scope != tc.scope {
				continue
			}
			var fields []string
			for i := range d.Fields {
				if datasets.Visible(tc.actor, &d.Fields[i]) {
					fields = append(fields, d.Fields[i].Name)
				}
			}
			out := dispatch(tc.actor, "query_data", map[string]any{"dataset": d.Name, "fields": fields, "limit": 200})
			if out.Decision != "allowed" {
				t.Errorf("%s/%s: decision %s: %s", tc.scope, d.Name, out.Decision, out.Content)
				continue
			}
			if strings.Contains(out.Content, markB) {
				t.Errorf("LEAK %s/%s returned the other tenant's rows: %s", tc.scope, d.Name, out.Content)
			}
			for _, must := range tc.must {
				if must == d.Name && !strings.Contains(out.Content, markA) {
					t.Errorf("%s/%s did not return the caller's own rows: %s", tc.scope, d.Name, out.Content)
				}
			}
			for _, f := range d.Fields {
				if f.Type != datasets.Text && f.Type != datasets.Enum {
					continue
				}
				if !datasets.Visible(tc.actor, &f) {
					continue
				}
				g := dispatch(tc.actor, "query_data", map[string]any{"dataset": d.Name, "group_by": []string{f.Name}, "metrics": []string{"count"}})
				if strings.Contains(g.Content, markB) {
					t.Errorf("LEAK %s/%s grouped by %s: %s", tc.scope, d.Name, f.Name, g.Content)
				}
			}
		}
	}

	t.Run("get_record opens own records with related rows and refuses the other tenant", func(t *testing.T) {
		buyer := cases[0].actor
		list := dispatch(buyer, "query_data", map[string]any{"dataset": "purchase_orders"})
		var payload struct {
			Data struct {
				Columns []string `json:"columns"`
				Rows    [][]any  `json:"rows"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(list.Content), &payload); err != nil || len(payload.Data.Rows) == 0 {
			t.Fatalf("no rows to open: %v %s", err, list.Content)
		}
		ref := payload.Data.Rows[0][0].(string)
		rec := dispatch(buyer, "get_record", map[string]any{"ref": ref})
		if rec.Decision != "allowed" || !strings.Contains(rec.Content, markA+" line") || strings.Contains(rec.Content, markB) {
			t.Fatalf("record: %s %s", rec.Decision, rec.Content)
		}
		intruder := ownerActor(rbac.ScopePharmacy, a.buyer+1_000_000, a.buyerUser)
		if out := dispatch(intruder, "get_record", map[string]any{"ref": ref}); out.Decision == "allowed" {
			t.Fatalf("a handle opened for another organisation: %s", out.Content)
		}
	})

	t.Run("exports carry only the caller's rows and only its owner can load them", func(t *testing.T) {
		buyer := cases[0].actor
		out := dispatch(buyer, "export_data", map[string]any{"dataset": "purchase_order_lines", "format": "csv"})
		if out.Decision != "allowed" {
			t.Fatalf("export: %s", out.Content)
		}
		var token string
		for _, e := range out.Entities {
			if e.Kind == assistant.EntityExport {
				token = strings.TrimPrefix(e.URL, assistant.ExportPath(""))
			}
		}
		file, err := repo.LoadExport(ctx, token)
		if err != nil || file == nil {
			t.Fatalf("load export: %v", err)
		}
		if file.UserID != buyer.UserID || file.OrganizationID != buyer.OrgID {
			t.Fatalf("export owned by %d/%d", file.UserID, file.OrganizationID)
		}
		if !strings.Contains(string(file.Content), markA) || strings.Contains(string(file.Content), markB) {
			t.Fatalf("export content: %s", file.Content)
		}
		if f, _ := repo.LoadExport(ctx, token+"x"); f != nil {
			t.Fatal("a wrong token loaded a file")
		}
	})

	t.Run("the dataset role cannot read secrets or write", func(t *testing.T) {
		for _, q := range []string{
			`SELECT password_hash FROM identity.users LIMIT 1`,
			`SELECT delivery_code FROM commerce.order_shipments LIMIT 1`,
			`SELECT destination_details FROM billing.wallet_withdrawals LIMIT 1`,
			`SELECT * FROM identity.user_sessions LIMIT 1`,
			`SELECT * FROM telegram.links LIMIT 1`,
			`UPDATE commerce.orders SET notes = 'x' WHERE false`,
		} {
			tx, err := db.Pool().Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+assistantPostgres.DatasetRole); err != nil {
				_ = tx.Rollback(ctx)
				t.Fatal(err)
			}
			_, err = tx.Exec(ctx, q)
			_ = tx.Rollback(ctx)
			if err == nil || !strings.Contains(err.Error(), "permission denied") {
				t.Errorf("%s: want permission denied, got %v", q, err)
			}
		}
	})
}

func TestPendingActionsAreSingleUseAndOwnerScoped(t *testing.T) {
	db := openTestDB(t)
	ctx := database.AsSystem(context.Background())
	repo := assistantPostgres.NewRepository(db)
	stamp := time.Now().UnixNano()
	s := seed{t: t, ctx: ctx, db: db}
	user := s.id(`INSERT INTO identity.users (email, password_hash, name, role, status) VALUES ($1,'x','{"ar":"u"}','customer','active') RETURNING id`, fmt.Sprintf("act-%d@example.test", stamp))
	org := s.id(`INSERT INTO org.organizations (name, type, status) VALUES ('{"ar":"o"}','customer','approved') RETURNING id`)

	p := &actions.Pending{
		OrganizationID: org, UserID: user, Scope: "pharmacy", Channel: actions.ChannelWeb, Action: "cart_add",
		Risk: actions.RiskLow, Args: actions.Args{"offer": int64(7), "quantity": int64(2)},
		Preview: actions.Preview{Title: "t"}, PreviewHash: []byte{1}, Status: actions.StatusPending,
		ExpiresAt: time.Now().Add(actions.TTL),
	}
	if err := repo.CreatePendingAction(ctx, p); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.GetPendingAction(ctx, p.PublicID, user+1, org); got != nil {
		t.Fatal("another user loaded the proposal")
	}
	if got, _ := repo.GetPendingAction(ctx, p.PublicID, user, org+1); got != nil {
		t.Fatal("another organisation loaded the proposal")
	}
	if got, _ := repo.GetPendingAction(ctx, uuid.New(), user, org); got != nil {
		t.Fatal("an unknown id loaded a proposal")
	}
	got, err := repo.GetPendingAction(ctx, p.PublicID, user, org)
	if err != nil || got == nil || got.Args.ID("offer") != 7 || got.Args.Int("quantity") != 2 {
		t.Fatalf("round trip: %+v %v", got, err)
	}

	first, err := repo.ClaimPendingAction(ctx, p.ID)
	second, _ := repo.ClaimPendingAction(ctx, p.ID)
	if err != nil || !first || second {
		t.Fatalf("claims: first=%v second=%v err=%v", first, second, err)
	}
	if err := repo.FinishPendingAction(ctx, p.ID, actions.StatusExecuted, &actions.Outcome{Message: "done"}, ""); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.GetPendingAction(ctx, p.PublicID, user, org)
	if got.Status != actions.StatusExecuted || got.Outcome == nil || got.Outcome.Message != "done" {
		t.Fatalf("finished: %+v", got)
	}

	expired := *p
	expired.ExpiresAt = time.Now().Add(-time.Minute)
	if err := repo.CreatePendingAction(ctx, &expired); err != nil {
		t.Fatal(err)
	}
	if ok, _ := repo.ClaimPendingAction(ctx, expired.ID); ok {
		t.Fatal("an expired proposal was claimed")
	}
}
