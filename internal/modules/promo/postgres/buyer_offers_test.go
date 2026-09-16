package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// TestBuyerOfferRule seeds one offer per precondition and asserts the listing
// and the single-offer verdict agree on every one of them. The buyer is a
// supplier buying from other suppliers, which is the case the rule must hold
// for as well as a pharmacy.
func TestBuyerOfferRule(t *testing.T) {
	db := getTestDB(t)
	ctx := database.AsSystem(context.Background())
	repo := NewRepository(db)
	f := seedOfferRule(t, ctx, db)

	buyer := promo.BuyerOfferQuery{
		BuyerOrgID: f.buyerOrg,
		Buying:     true,
		Branch: promo.BuyerBranch{
			ID: f.buyerBranch, CityID: f.cityHere, Lat: 30.0500, Lon: 31.2400, HasCoords: true,
			Weekday: f.today, AllowedWorkIDs: []int64{f.work},
		},
		Coverage: promo.SupplierCoverage{OrgIDs: []int64{f.vendor}, BranchIDs: []int64{f.vendorBranch}},
		Search:   f.mark,
		Limit:    50,
	}

	want := map[string]promo.OfferReason{
		"weekly coverage, no branch":       promo.OfferOK,
		"weekly coverage, live branch":     promo.OfferOK,
		"deleted supplier branch":          promo.OfferBranchUnavailable,
		"own rule, city, today":            promo.OfferOK,
		"own rule, city, another day":      promo.OfferNotCovered,
		"own rule, within radius, today":   promo.OfferOK,
		"unapproved supplier":              promo.OfferSupplierUnavailable,
		"pending approval":                 promo.OfferNotLive,
		"draft":                            promo.OfferNotLive,
		"expired":                          promo.OfferNotLive,
		"buyer's own offer":                promo.OfferOwn,
		"supplier without connected works": promo.OfferInstitutionalMismatch,
		"uncovered supplier, no own rules": promo.OfferNotCovered,
	}
	if len(want) != len(f.offers) {
		t.Fatalf("fixture has %d offers, expectations %d", len(f.offers), len(want))
	}

	listed, total, err := repo.ListBuyerOffers(ctx, buyer)
	if err != nil {
		t.Fatal(err)
	}
	shown := map[int64]*promo.BuyerOffer{}
	for _, o := range listed {
		shown[o.ID] = o
	}
	visible := 0
	for name, reason := range want {
		id := f.offers[name]
		q := buyer
		q.OfferID = id
		v, err := repo.OfferVerdict(ctx, q)
		if err != nil {
			t.Fatalf("%s: verdict: %v", name, err)
		}
		if got := v.Reason(true); got != reason {
			t.Errorf("%s: verdict %q, want %q (%+v)", name, got, reason, v)
		}
		if _, on := shown[id]; on != (reason == promo.OfferOK) {
			t.Errorf("%s: listed=%v but verdict is %q", name, on, reason)
		}
		if reason == promo.OfferOK {
			visible++
		}
	}
	if total != visible {
		t.Errorf("total %d, want %d visible offers", total, visible)
	}

	// A product sponsorship whose id equals an offer's must not rank that
	// offer as sponsored; an offer sponsorship does, and ranks first.
	if o := shown[f.offers["weekly coverage, no branch"]]; o == nil || o.Sponsored {
		t.Errorf("product sponsorship leaked onto an offer with the same id: %+v", o)
	}
	if len(listed) == 0 || listed[0].ID != f.offers["own rule, city, today"] || !listed[0].Sponsored {
		t.Errorf("sponsored offer should rank first, got %+v", listed)
	}

	// A coverage row bound to no branch covers the supplier's branch offers.
	orgWide := buyer
	orgWide.Coverage = promo.SupplierCoverage{OrgIDs: []int64{f.vendor}, OrgWideIDs: []int64{f.vendor}}
	orgWide.OfferID = f.offers["weekly coverage, live branch"]
	if v, err := repo.OfferVerdict(ctx, orgWide); err != nil || !v.Covered {
		t.Errorf("org-wide coverage should cover a branch offer: %+v %v", v, err)
	}

	// Browsing (no buyer branch) shows live offers of approved suppliers only,
	// without coverage or institutional rules.
	browse := promo.BuyerOfferQuery{Search: f.mark, Limit: 50}
	all, _, err := repo.ListBuyerOffers(ctx, browse)
	if err != nil {
		t.Fatal(err)
	}
	browsed := map[int64]bool{}
	for _, o := range all {
		browsed[o.ID] = true
	}
	for _, name := range []string{"own rule, city, another day", "supplier without connected works", "buyer's own offer"} {
		if !browsed[f.offers[name]] {
			t.Errorf("browsing should show %q", name)
		}
	}
	for _, name := range []string{"deleted supplier branch", "unapproved supplier", "pending approval", "draft", "expired"} {
		if browsed[f.offers[name]] {
			t.Errorf("browsing must not show %q", name)
		}
	}

	// The anonymous storefront list applies the same live rule.
	active, err := repo.ListActiveOffers(ctx, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range active {
		for _, name := range []string{"deleted supplier branch", "unapproved supplier", "draft"} {
			if o.ID == f.offers[name] {
				t.Errorf("ListActiveOffers shows %q", name)
			}
		}
	}
}

type offerRuleFixture struct {
	mark                                string
	today                               time.Weekday
	cityHere, cityFar                   int64
	work, otherWork                     int64
	buyerOrg, buyerBranch               int64
	vendor, vendorBranch, deletedBranch int64
	offers                              map[string]int64
}

func seedOfferRule(t *testing.T, ctx context.Context, db *database.DB) offerRuleFixture {
	t.Helper()
	pool := db.Pool()
	f := offerRuleFixture{mark: fmt.Sprintf("ZOR%d", time.Now().UnixNano()), today: time.Now().Weekday(), offers: map[string]int64{}}
	id := func(sql string, args ...any) int64 {
		t.Helper()
		var v int64
		if err := pool.QueryRow(ctx, sql, args...).Scan(&v); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return v
	}
	cities := func() []int64 {
		rows, err := pool.Query(ctx, `SELECT id FROM platform_admin.cities ORDER BY id LIMIT 2`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []int64
		for rows.Next() {
			var c int64
			_ = rows.Scan(&c)
			out = append(out, c)
		}
		return out
	}()
	if len(cities) < 2 {
		t.Skip("needs two seeded cities")
	}
	f.cityHere, f.cityFar = cities[0], cities[1]

	org := func(kind, status string) int64 {
		return id(`INSERT INTO org.organizations (name, legal_name, type, status) VALUES (jsonb_build_object('ar', $1::text), $1, $2, $3) RETURNING id`,
			f.mark+" "+kind+" "+status, kind, status)
	}
	branch := func(orgID int64, deleted bool) int64 {
		b := id(`INSERT INTO org.branches (organization_id, name, status, latitude, longitude, city_id) VALUES ($1, '{"ar":"فرع"}', 'active', 30.05, 31.24, $2) RETURNING id`, orgID, f.cityHere)
		if deleted {
			if _, err := pool.Exec(ctx, `UPDATE org.branches SET deleted_at = now(), status = 'inactive' WHERE id = $1`, b); err != nil {
				t.Fatal(err)
			}
		}
		return b
	}
	work := func(branchID int64, title string) int64 {
		w := id(`INSERT INTO org.institutional_works (title) VALUES (jsonb_build_object('ar', $1::text)) RETURNING id`, f.mark+title)
		if _, err := pool.Exec(ctx, `INSERT INTO org.branch_institutional_works (branch_id, work_category, institutional_work_id) VALUES ($1, $2, $3)`, branchID, title, w); err != nil {
			t.Fatal(err)
		}
		return w
	}

	// The buyer is itself a supplier, buying from others.
	f.buyerOrg = org("vendor", "approved")
	f.buyerBranch = branch(f.buyerOrg, false)
	work(f.buyerBranch, "buyer")

	f.vendor = org("vendor", "approved")
	f.vendorBranch = branch(f.vendor, false)
	f.deletedBranch = branch(f.vendor, true)
	f.work = work(f.vendorBranch, "pharmacy")

	unapproved := org("vendor", "pending")
	work(branch(unapproved, false), "pharmacy2")
	disconnected := org("vendor", "approved")
	f.otherWork = work(branch(disconnected, false), "factory")
	uncovered := org("vendor", "approved")
	work(branch(uncovered, false), "pharmacy3")
	// uncovered's branch holds a connected work too, so only coverage refuses it.
	if _, err := pool.Exec(ctx, `UPDATE org.branch_institutional_works SET institutional_work_id = $1 WHERE branch_id IN (SELECT id FROM org.branches WHERE organization_id = $2)`, f.work, uncovered); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE org.branch_institutional_works SET institutional_work_id = $1 WHERE branch_id IN (SELECT id FROM org.branches WHERE organization_id = $2)`, f.work, unapproved); err != nil {
		t.Fatal(err)
	}

	type spec struct {
		org     int64
		branch  *int64
		admin   string
		draft   bool
		expired bool
	}
	offer := func(name string, s spec) int64 {
		if s.admin == "" {
			s.admin = "approved"
		}
		expires := time.Now().Add(24 * time.Hour)
		if s.expired {
			expires = time.Now().Add(-time.Hour)
		}
		o := id(`INSERT INTO promo.offers (organization_id, branch_id, title, discount_type, discount_value, admin_status, starts_at, expires_at, is_active, is_draft, source, total_price)
			VALUES ($1, $2, jsonb_build_object('ar', $3::text), 'percentage', 10, $4, now() - interval '2 days', $5, true, $6, 'special', 100) RETURNING id`,
			s.org, s.branch, f.mark+" "+name, s.admin, expires, s.draft)
		f.offers[name] = o
		return o
	}
	rule := func(offerID, city int64, day time.Weekday, lat, lon float64) {
		if _, err := pool.Exec(ctx, `INSERT INTO promo.offer_location_covers (offer_id, organization_id, city_id, latitude, longitude, radius_meters, day_of_week, status, admin_status)
			VALUES ($1, (SELECT organization_id FROM promo.offers WHERE id = $1), $2, $3, $4, 5000, $5, 'active', 'approved')`, offerID, city, lat, lon, int(day)); err != nil {
			t.Fatal(err)
		}
	}
	vb, db2 := f.vendorBranch, f.deletedBranch
	otherDay := (f.today + 3) % 7

	offer("weekly coverage, no branch", spec{org: f.vendor})
	offer("weekly coverage, live branch", spec{org: f.vendor, branch: &vb})
	offer("deleted supplier branch", spec{org: f.vendor, branch: &db2})
	sponsored := offer("own rule, city, today", spec{org: f.vendor})
	rule(sponsored, f.cityHere, f.today, 0, 0)
	rule(offer("own rule, city, another day", spec{org: f.vendor}), f.cityHere, otherDay, 0, 0)
	rule(offer("own rule, within radius, today", spec{org: f.vendor}), f.cityFar, f.today, 30.0510, 31.2410)
	offer("unapproved supplier", spec{org: unapproved})
	offer("pending approval", spec{org: f.vendor, admin: "pending"})
	offer("draft", spec{org: f.vendor, draft: true})
	offer("expired", spec{org: f.vendor, expired: true})
	offer("buyer's own offer", spec{org: f.buyerOrg})
	offer("supplier without connected works", spec{org: disconnected})
	offer("uncovered supplier, no own rules", spec{org: uncovered})

	pkg := id(`INSERT INTO promo.offer_packages (name, tier_level) VALUES ('{"ar":"باقة"}', 3) RETURNING id`)
	for _, s := range []struct {
		itemType string
		item     int64
	}{{"offer", sponsored}, {"product", f.offers["weekly coverage, no branch"]}} {
		if _, err := pool.Exec(ctx, `INSERT INTO promo.sponsorship_requests (organization_id, package_id, item_type, item_id, status, admin_status, expires_at)
			VALUES ($1, $2, $3, $4, 'active', 'approved', now() + interval '1 day')`, f.vendor, pkg, s.itemType, s.item); err != nil {
			t.Fatal(err)
		}
	}

	t.Cleanup(func() {
		c := context.Background()
		orgs := []int64{f.buyerOrg, f.vendor, unapproved, disconnected, uncovered}
		for _, q := range []string{
			`DELETE FROM promo.sponsorship_requests WHERE organization_id = ANY($1)`,
			`DELETE FROM promo.offers WHERE organization_id = ANY($1)`,
			`DELETE FROM org.branches WHERE organization_id = ANY($1)`,
			`DELETE FROM org.organizations WHERE id = ANY($1)`,
		} {
			_, _ = pool.Exec(c, q, orgs)
		}
		_, _ = pool.Exec(c, `DELETE FROM promo.offer_packages WHERE id = $1`, pkg)
		_, _ = pool.Exec(c, `DELETE FROM org.institutional_works WHERE title->>'ar' LIKE $1`, f.mark+"%")
	})
	return f
}
