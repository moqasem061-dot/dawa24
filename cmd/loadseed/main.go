// Command loadseed fills a disposable database with a realistic marketplace for
// load testing: a master catalogue, suppliers with branches, coverage, stock
// and offers, and pharmacies with branches, orders and notifications.
//
// It refuses any database whose name does not contain "scratch", "loadtest" or
// "dev", so it cannot be pointed at production by mistake. Run migrations
// first, then this, then `cli migrate` again so company roles are seeded.
//
//	DATABASE_URL=postgres://.../dawa24_loadtest go run ./cmd/loadseed
//
// Every seeded user's password is LOADSEED_PASSWORD (default LoadTest#2026).
// Accounts: pharmacy1..N@loadtest.dawa24 and supplier1..N@loadtest.dawa24.
package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	products   = 3000
	suppliers  = 4
	pharmacies = 6
	variantsPS = 2000 // listings per supplier
	ordersPP   = 50   // orders per pharmacy
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}
	name := strings.TrimPrefix(u.Path, "/")
	if !strings.Contains(name, "scratch") && !strings.Contains(name, "loadtest") && !strings.Contains(name, "dev") {
		log.Fatalf("refusing to seed %q: the database name must contain scratch, loadtest or dev", name)
	}
	password := os.Getenv("LOADSEED_PASSWORD")
	if password == "" {
		password = "LoadTest#2026"
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close(ctx)

	tx, err := conn.Begin(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for i, step := range steps(string(hash)) {
		start := time.Now()
		if _, err := tx.Exec(ctx, step.sql); err != nil {
			log.Fatalf("step %d (%s): %v", i+1, step.name, err)
		}
		log.Printf("%-28s %v", step.name, time.Since(start).Round(time.Millisecond))
	}
	if err := tx.Commit(ctx); err != nil {
		log.Fatal(err)
	}
	fmt.Println("seeded; now run `cli migrate` so company roles exist")
}

type step struct{ name, sql string }

func steps(hash string) []step {
	q := func(s string) string { return strings.ReplaceAll(s, "$HASH", "'"+hash+"'") }
	return []step{
		{"master catalogue org", `
INSERT INTO org.organizations (name, trade_name, legal_name, type, status, email, phone)
SELECT '{"ar":"كتالوج دوا24","en":"Dawa24 Catalogue"}', '{"ar":"كتالوج دوا24","en":"Dawa24 Catalogue"}', 'Dawa24 Catalogue', 'company', 'approved', 'catalogue@loadtest.dawa24', '0100'
WHERE NOT EXISTS (SELECT 1 FROM org.organizations WHERE email = 'catalogue@loadtest.dawa24')`},
		{"institutional work", `
INSERT INTO org.institutional_works (title)
SELECT '{"ar":"صيدليات","en":"Pharmacies (loadtest)"}'
WHERE NOT EXISTS (SELECT 1 FROM org.institutional_works WHERE title->>'en' = 'Pharmacies (loadtest)')`},
		{"work connection", `
INSERT INTO org.institutional_work_connections (from_institutional_work_id, to_institutional_work_id)
SELECT w.id, w.id FROM org.institutional_works w WHERE w.title->>'en' = 'Pharmacies (loadtest)'
  AND NOT EXISTS (SELECT 1 FROM org.institutional_work_connections c WHERE c.from_institutional_work_id = w.id AND c.to_institutional_work_id = w.id)`},
		{"products", fmt.Sprintf(`
INSERT INTO catalog.products (organization_id, name, sku, scientific_name, company, status, price)
SELECT (SELECT id FROM org.organizations WHERE email = 'catalogue@loadtest.dawa24'),
       jsonb_build_object('ar', 'دواء تجريبي ' || g, 'en', 'Load Drug ' || g),
       'LT-P-' || g, 'Substance ' || (g %% 300), 'Company ' || (g %% 40), 'active', (20 + g %% 400)
  FROM generate_series(1, %d) g
 WHERE NOT EXISTS (SELECT 1 FROM catalog.products WHERE sku = 'LT-P-1')`, products)},
		{"users", q(fmt.Sprintf(`
INSERT INTO identity.users (email, password_hash, name, role, status, email_verified_at)
SELECT kind || g || '@loadtest.dawa24', $HASH, jsonb_build_object('ar', kind || ' ' || g, 'en', kind || ' ' || g), 'user', 'active', now()
  FROM (SELECT 'supplier' kind, generate_series(1, %d) g UNION ALL SELECT 'pharmacy', generate_series(1, %d)) s
ON CONFLICT DO NOTHING`, suppliers, pharmacies))},
		{"organizations", `
INSERT INTO org.organizations (name, trade_name, legal_name, type, status, email, phone, owner_id, approved_at)
SELECT jsonb_build_object('ar', CASE WHEN u.email LIKE 'supplier%' THEN 'مورد ' ELSE 'صيدلية ' END || split_part(u.email, '@', 1), 'en', split_part(u.email, '@', 1)),
       jsonb_build_object('ar', split_part(u.email, '@', 1), 'en', split_part(u.email, '@', 1)),
       split_part(u.email, '@', 1),
       CASE WHEN u.email LIKE 'supplier%' THEN 'vendor' ELSE 'customer' END, 'approved',
       'org-' || u.email, '0101', u.id, now()
  FROM identity.users u
 WHERE u.email LIKE '%@loadtest.dawa24'
   AND NOT EXISTS (SELECT 1 FROM org.organizations o WHERE o.email = 'org-' || u.email)`},
		{"members", `
INSERT INTO org.members (organization_id, user_id, role_key, status, is_active, job_title)
SELECT o.id, o.owner_id, 'org_owner', 'active', true, 'Owner'
  FROM org.organizations o
 WHERE o.email LIKE 'org-%@loadtest.dawa24'
   AND NOT EXISTS (SELECT 1 FROM org.members m WHERE m.organization_id = o.id AND m.user_id = o.owner_id)`},
		{"branches", `
INSERT INTO org.branches (organization_id, name, address, code, is_main, status, latitude, longitude, city_id)
SELECT o.id, jsonb_build_object('ar', 'فرع رئيسي', 'en', 'Main'), 'Cairo', 'MAIN-' || o.id, true, 'active',
       30.0444 + (o.id % 10) * 0.002, 31.2357 + (o.id % 10) * 0.002,
       (SELECT id FROM platform_admin.cities ORDER BY id LIMIT 1)
  FROM org.organizations o
 WHERE o.email LIKE 'org-%@loadtest.dawa24'
   AND NOT EXISTS (SELECT 1 FROM org.branches b WHERE b.organization_id = o.id)`},
		{"branch works", `
INSERT INTO org.branch_institutional_works (branch_id, work_category, institutional_work_id)
SELECT b.id, 'pharmacy', (SELECT id FROM org.institutional_works WHERE title->>'en' = 'Pharmacies (loadtest)')
  FROM org.branches b JOIN org.organizations o ON o.id = b.organization_id
 WHERE o.email LIKE 'org-%@loadtest.dawa24'
ON CONFLICT DO NOTHING`},
		{"weekly coverage", `
INSERT INTO workflow.weekly_coverages (organization_id, branch_id, day_of_week, address, latitude, longitude, distance_meters, city_id, is_active, coverage_from, coverage_to)
SELECT o.id, b.id, d, 'Greater Cairo', b.latitude, b.longitude, 50000, b.city_id, true, '08:00', '20:00'
  FROM org.organizations o JOIN org.branches b ON b.organization_id = o.id, generate_series(0, 6) d
 WHERE o.email LIKE 'org-supplier%@loadtest.dawa24'
   AND NOT EXISTS (SELECT 1 FROM workflow.weekly_coverages w WHERE w.branch_id = b.id)`},
		{"warehouses", `
INSERT INTO inventory.warehouses (organization_id, branch_id, name, code, address)
SELECT o.id, b.id, 'Main WH', 'WH-' || o.id, 'Cairo'
  FROM org.organizations o JOIN org.branches b ON b.organization_id = o.id
 WHERE o.email LIKE 'org-supplier%@loadtest.dawa24'
   AND NOT EXISTS (SELECT 1 FROM inventory.warehouses w WHERE w.organization_id = o.id)`},
		{"supplier listings", fmt.Sprintf(`
INSERT INTO catalog.product_variants (organization_id, product_id, name, sku, barcode, price, discount, status, branch_id, batch_number, expiry_date)
SELECT o.id, p.id, p.name, 'LT-V-' || o.id || '-' || p.id, 'BC' || o.id || p.id, p.price, (p.id %% 25), 'active', b.id, 'B' || p.id, current_date + 400
  FROM org.organizations o
  JOIN org.branches b ON b.organization_id = o.id
  JOIN LATERAL (SELECT id, name, price FROM catalog.products WHERE sku LIKE 'LT-P-%%' ORDER BY (id * (o.id + 7)) %% 9973 LIMIT %d) p ON true
 WHERE o.email LIKE 'org-supplier%%@loadtest.dawa24'
   AND NOT EXISTS (SELECT 1 FROM catalog.product_variants v WHERE v.organization_id = o.id)`, variantsPS)},
		{"stock", `
INSERT INTO inventory.stocks (organization_id, warehouse_id, product_id, product_variant_id, quantity, min_threshold)
SELECT v.organization_id, w.id, v.product_id, v.id, 50 + (v.id % 500), 20
  FROM catalog.product_variants v JOIN inventory.warehouses w ON w.organization_id = v.organization_id
 WHERE v.sku LIKE 'LT-V-%'
   AND NOT EXISTS (SELECT 1 FROM inventory.stocks s WHERE s.product_variant_id = v.id)`},
		{"offers", `
INSERT INTO promo.offers (organization_id, branch_id, title, description, discount_type, discount_value, admin_status, starts_at, expires_at, is_active, is_draft, source, total_price)
SELECT o.id, NULL, jsonb_build_object('ar', 'عرض ' || o.id || '-' || g, 'en', 'Offer ' || o.id || '-' || g), '{"ar":"عرض تجريبي"}',
       'percentage', 5 + g, 'approved', now() - interval '1 day', now() + interval '60 days', true, false, 'special', 300 + g * 10
  FROM org.organizations o, generate_series(1, 5) g
 WHERE o.email LIKE 'org-supplier%@loadtest.dawa24'
   AND NOT EXISTS (SELECT 1 FROM promo.offers x WHERE x.organization_id = o.id)`},
		{"offer products", `
INSERT INTO promo.offer_products (offer_id, product_id, product_variant_id, variant_id, custom_price, custom_qty)
SELECT x.id, v.product_id, v.id, v.id, v.price, 2
  FROM promo.offers x
  JOIN LATERAL (SELECT id, product_id, price FROM catalog.product_variants WHERE organization_id = x.organization_id ORDER BY id LIMIT 3) v ON true
  JOIN org.organizations o ON o.id = x.organization_id
 WHERE o.email LIKE 'org-supplier%@loadtest.dawa24'
   AND NOT EXISTS (SELECT 1 FROM promo.offer_products op WHERE op.offer_id = x.id)`},
		{"orders", fmt.Sprintf(`
INSERT INTO commerce.orders (order_number, customer_id, organization_id, branch_id, status, payment_status, payment_method, subtotal, total_amount, negotiation_status, created_at)
SELECT 'LT-O-' || o.id || '-' || g, o.owner_id, o.id, b.id,
       (ARRAY['pending','processing','confirmed','shipped','delivered','completed'])[1 + g %% 6], 'unpaid', 'cod', 500, 500, 'none',
       now() - (g || ' days')::interval
  FROM org.organizations o JOIN org.branches b ON b.organization_id = o.id, generate_series(1, %d) g
 WHERE o.email LIKE 'org-pharmacy%%@loadtest.dawa24'
   AND NOT EXISTS (SELECT 1 FROM commerce.orders x WHERE x.order_number = 'LT-O-' || o.id || '-1')`, ordersPP)},
		{"shipments", `
INSERT INTO commerce.order_shipments (order_id, organization_id, shipment_number, status, subtotal, total_amount, tracking_number, carrier_name)
SELECT ord.id, s.id, ord.order_number || '-S', ord.status, 500, 500, '', ''
  FROM commerce.orders ord
  JOIN LATERAL (SELECT id FROM org.organizations WHERE email LIKE 'org-supplier%@loadtest.dawa24' ORDER BY (id + ord.id) % 4 LIMIT 1) s ON true
 WHERE ord.order_number LIKE 'LT-O-%'
   AND NOT EXISTS (SELECT 1 FROM commerce.order_shipments x WHERE x.order_id = ord.id)`},
		{"order lines", `
INSERT INTO commerce.order_lines (order_id, shipment_id, organization_id, product_id, product_variant_id, product_name, variant_name, sku, unit_price, quantity, total_price)
SELECT sh.order_id, sh.id, sh.organization_id, v.product_id, v.id, jsonb_build_object('ar', 'صنف'), jsonb_build_object('ar', 'صنف'), v.sku, 100, 5, 500
  FROM commerce.order_shipments sh
  JOIN commerce.orders ord ON ord.id = sh.order_id AND ord.order_number LIKE 'LT-O-%'
  JOIN LATERAL (SELECT id, product_id, sku FROM catalog.product_variants WHERE organization_id = sh.organization_id ORDER BY (id + sh.id) % 101 LIMIT 1) v ON true
 WHERE NOT EXISTS (SELECT 1 FROM commerce.order_lines l WHERE l.shipment_id = sh.id)`},
		{"notifications", `
INSERT INTO notifications.logs (user_id, organization_id, channel, recipient, title, body, status, is_read, required_permission)
SELECT m.user_id, m.organization_id, 'in_app', 'user', 'إشعار ' || g, 'نص الإشعار', 'sent', g % 3 = 0, ''
  FROM org.members m JOIN org.organizations o ON o.id = m.organization_id, generate_series(1, 30) g
 WHERE o.email LIKE 'org-%@loadtest.dawa24'
   AND NOT EXISTS (SELECT 1 FROM notifications.logs n WHERE n.user_id = m.user_id)`},
	}
}
