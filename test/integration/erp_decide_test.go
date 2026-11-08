package integration_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	billingpg "github.com/moqasem061-dot/dawa24/internal/modules/billing/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	catalogpg "github.com/moqasem061-dot/dawa24/internal/modules/catalog/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	commercepg "github.com/moqasem061-dot/dawa24/internal/modules/commerce/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	erppg "github.com/moqasem061-dot/dawa24/internal/modules/erp/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider"
	"github.com/moqasem061-dot/dawa24/internal/modules/inventory"
	inventorypg "github.com/moqasem061-dot/dawa24/internal/modules/inventory/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/notifications"
	notificationspg "github.com/moqasem061-dot/dawa24/internal/modules/notifications/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	orgpg "github.com/moqasem061-dot/dawa24/internal/modules/org/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/ui"
)

// S4 (US4) on the real database. The decider behind the erp.OrderDecider port is
// the website's own decision method, so wallet charges, refunds, status history
// and the buyer notice all go through the rules the website uses. The buyer is
// the pharmacy; the supplier acts through its ERP key.

// decideFixture is one buyer pharmacy, one supplier with an ERP key that holds
// erp.orders.decide, and one catalog variant that the order lines point at.
type decideFixture struct {
	buyer, buyerUser, supplier int64
	integ, keyID               int64
	variant                    int64
	walletID                   int64
	walletMinor                int64
	orderNumber                string
}

// seedDecideFixture creates the organisations, users, key, variant and a funded
// buyer wallet. Every id is time based, as in the other ERP integration tests.
func seedDecideFixture(t *testing.T, db *database.DB, base, walletMinor int64) *decideFixture {
	t.Helper()
	ctx := context.Background()
	fx := &decideFixture{buyer: base, buyerUser: base, supplier: base + 1, walletMinor: walletMinor}
	fx.orderNumber = fmt.Sprintf("DEC-%d", base)
	err := db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"Decide buyer"}'), ($2, '{"en":"Decide supplier"}')`, fx.buyer, fx.supplier); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO identity.users (id, email, password_hash) VALUES ($1, $2, 'x')`, fx.buyerUser, fmt.Sprintf("erp-decide-%d@example.test", base)); err != nil {
			return err
		}
		var branch, warehouse int64
		if err := tx.QueryRow(ctx, `INSERT INTO org.branches (organization_id, name) VALUES ($1, '{"en":"Decide branch"}') RETURNING id`, fx.supplier).Scan(&branch); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO inventory.warehouses (organization_id, name) VALUES ($1, 'Decide warehouse') RETURNING id`, fx.supplier).Scan(&warehouse); err != nil {
			return err
		}
		var err error
		fx.integ, fx.keyID, err = seedERPKey(ctx, tx, fx.supplier, warehouse, branch, []string{erp.ScopeOrdersDecide})
		if err != nil {
			return err
		}
		var productID int64
		if err := tx.QueryRow(ctx, `INSERT INTO catalog.products (organization_id, name) VALUES ($1, '{"en":"Panadol"}') RETURNING id`, fx.supplier).Scan(&productID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO catalog.product_variants (organization_id, product_id, name, sku, barcode, price) VALUES ($1, $2, '{"en":"Panadol Extra"}', 'DEC-SKU', '6221024000999', 54.69) RETURNING id`, fx.supplier, productID).Scan(&fx.variant)
	})
	if err != nil {
		t.Fatalf("decide fixture: %v", err)
	}
	cleanupERPOrgs(t, db, ctx, fx.buyer, fx.supplier)
	wallet, err := billingpg.NewRepository(db).GetOrCreateWallet(database.AsSystem(ctx), fx.buyerUser, "EGP")
	if err != nil {
		t.Fatalf("buyer wallet: %v", err)
	}
	fx.walletID = wallet.ID
	if walletMinor > 0 {
		if _, err := billingpg.NewRepository(db).RecordTransaction(database.AsSystem(ctx), wallet.ID, billing.TxDeposit, money.FromMinor(walletMinor), "seed", nil, "seed deposit"); err != nil {
			t.Fatalf("seed deposit: %v", err)
		}
	}
	return fx
}

// seedDecideOrder writes a pending order with one supplier shipment. The
// shipment's goods are 1093.80 and its shipping fee 25.00, so the wallet charge
// on confirmation is exactly 1093.80.
func (fx *decideFixture) seedDecideOrder(t *testing.T, db *database.DB, orderNumber, paymentMethod string) int64 {
	t.Helper()
	var shipment int64
	err := db.InTx(database.AsSystem(context.Background()), func(ctx context.Context, tx pgx.Tx) error {
		var orderID int64
		if err := tx.QueryRow(ctx, `INSERT INTO commerce.orders (order_number, customer_id, organization_id, status, subtotal, shipping_fee, total_amount, notes, payment_method)
			VALUES ($1, $2, $3, 'pending', 1093.80, 25.00, 1118.80, 'decide order', $4) RETURNING id`, orderNumber, fx.buyerUser, fx.buyer, paymentMethod).Scan(&orderID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO commerce.order_shipments (order_id, organization_id, shipment_number, status, subtotal, shipping_fee, total_amount)
			VALUES ($1, $2, $3, 'pending', 1093.80, 25.00, 1118.80) RETURNING id`, orderID, fx.supplier, orderNumber+"-1").Scan(&shipment); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO commerce.order_lines (order_id, shipment_id, organization_id, product_variant_id, product_name, sku, unit_price, quantity, discount_amount, total_price)
			VALUES ($1, $2, $3, $4, '{"en":"Panadol Extra"}', 'DEC-SKU', 54.69, 20, 0.00, 1093.80)`, orderID, shipment, fx.supplier, fx.variant)
		return err
	})
	if err != nil {
		t.Fatalf("seed decide order: %v", err)
	}
	return shipment
}

// decideShipmentStatus reads a shipment's status as the platform sees it.
func decideShipmentStatus(t *testing.T, db *database.DB, shipment int64) string {
	t.Helper()
	var status string
	err := db.InReadTx(database.AsSystem(context.Background()), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT status FROM commerce.order_shipments WHERE id = $1`, shipment).Scan(&status)
	})
	if err != nil {
		t.Fatalf("shipment status: %v", err)
	}
	return status
}

// walletWithdrawals counts the debits made against the buyer's wallet.
func (fx *decideFixture) walletWithdrawals(t *testing.T, db *database.DB) int64 {
	t.Helper()
	var n int64
	err := db.InReadTx(database.AsSystem(context.Background()), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM billing.wallet_transactions WHERE wallet_id = $1 AND type = 'withdrawal'`, fx.walletID).Scan(&n)
	})
	if err != nil {
		t.Fatalf("withdrawal count: %v", err)
	}
	return n
}

// walletBalance returns the buyer's available balance as the website shows it.
func (fx *decideFixture) walletBalance(t *testing.T, db *database.DB) string {
	t.Helper()
	bill := billing.NewService(billingpg.NewRepository(db), slog.New(slog.NewTextHandler(io.Discard, nil)))
	w, err := bill.GetWallet(database.AsSystem(context.Background()), fx.buyerUser, "EGP")
	if err != nil || w == nil {
		t.Fatalf("buyer wallet balance: %v", err)
	}
	return w.AvailableBalance.String()
}

// principal is the machine caller: the supplier's key with the decide scope.
func (fx *decideFixture) principal() erp.Principal {
	return erp.Principal{
		OrganizationID: fx.supplier,
		IntegrationID:  fx.integ,
		KeyID:          fx.keyID,
		Provider:       provider.DefaultProvider,
		Scopes:         []string{erp.ScopeOrdersDecide},
		OrgType:        "vendor",
	}
}

// decideUI adapts the website's decision method to the erp.OrderDecider port.
type decideUI struct{ h *ui.UIHandler }

func (d decideUI) Decide(ctx context.Context, sd erp.ShipmentDecision) (erp.ShipmentDecisionResult, error) {
	return d.h.ErpDecideShipment(ctx, sd)
}

// newDecideService builds the ERP service on the real repository with the
// website's UI handler as the decider. Only the services the decision touches
// are real; the rest are left nil, as they are in the handler's own tests.
func newDecideService(db *database.DB) (*erp.Service, *ui.UIHandler) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := ui.NewUIHandler(
		catalog.NewService(catalogpg.NewRepository(db), log),
		org.NewService(orgpg.NewRepository(db), log),
		nil,
		// The production UI handler's commerce repository: wallet charges and refunds.
		commerce.NewService(commercepg.NewRepository(db).WithWalletRefunder(billingpg.NewRepository(db)).WithWalletPayer(billingpg.NewRepository(db)), log),
		inventory.NewService(inventorypg.NewRepository(db), log),
		nil,
		notifications.NewService(notificationspg.NewRepository(db), log),
		nil,
		nil,
		billing.NewService(billingpg.NewRepository(db), log),
		nil,
		nil,
		nil,
		nil,
		log,
	)
	svc := erp.NewService(erppg.New(db), erp.Ports{Decider: decideUI{h}}, erp.Config{
		Pepper: []byte("decide-erp-pepper-32-bytes-long!!"), Tag: "test", DefaultRate: 600, MaxBatchItems: 50000, Provider: provider.DefaultProvider,
	}, log)
	return svc, h
}

// decideDB opens the test database and registers its close.
func decideDB(t *testing.T) *database.DB {
	t.Helper()
	db := getTestDB(t)
	if db == nil {
		t.Fatal("database required")
	}
	t.Cleanup(db.Close)
	return db
}

func decideBase() int64 { return time.Now().UnixNano()/1000 + 1000 }

func TestERPDecide_ConfirmCash(t *testing.T) {
	db := decideDB(t)
	fx := seedDecideFixture(t, db, decideBase(), 0)
	shipment := fx.seedDecideOrder(t, db, fx.orderNumber, "cash")
	svc, _ := newDecideService(db)

	res, err := svc.Decide(context.Background(), fx.principal(), fx.orderNumber, "confirmed", "")
	if err != nil {
		t.Fatalf("confirm cash order: %v", err)
	}
	if res.Result != "confirmed" || res.OrderStatus != "confirmed" {
		t.Fatalf("result = %q / %q, want confirmed / confirmed", res.Result, res.OrderStatus)
	}
	if got := decideShipmentStatus(t, db, shipment); got != "confirmed" {
		t.Fatalf("shipment status = %q, want confirmed", got)
	}
	if n := fx.walletWithdrawals(t, db); n != 0 {
		t.Fatalf("%d wallet debits on a cash order, want 0", n)
	}
}

func TestERPDecide_ConfirmWalletCharges(t *testing.T) {
	db := decideDB(t)
	fx := seedDecideFixture(t, db, decideBase(), 200000)
	shipment := fx.seedDecideOrder(t, db, fx.orderNumber, "wallet")
	svc, _ := newDecideService(db)

	if _, err := svc.Decide(context.Background(), fx.principal(), fx.orderNumber, "confirmed", ""); err != nil {
		t.Fatalf("confirm wallet order: %v", err)
	}
	if got := decideShipmentStatus(t, db, shipment); got != "confirmed" {
		t.Fatalf("shipment status = %q, want confirmed", got)
	}
	if n := fx.walletWithdrawals(t, db); n != 1 {
		t.Fatalf("%d wallet debits, want 1", n)
	}
	if got := fx.walletBalance(t, db); got != "906.20" {
		t.Fatalf("wallet balance = %q, want 906.20", got)
	}
}

func TestERPDecide_RejectRefunds(t *testing.T) {
	db := decideDB(t)
	fx := seedDecideFixture(t, db, decideBase(), 200000)
	shipment := fx.seedDecideOrder(t, db, fx.orderNumber, "wallet")
	svc, _ := newDecideService(db)

	res, err := svc.Decide(context.Background(), fx.principal(), fx.orderNumber, "rejected", "الصنف غير متوفر حالياً")
	if err != nil {
		t.Fatalf("reject wallet order: %v", err)
	}
	if res.Result != "rejected" || res.OrderStatus != "cancelled" {
		t.Fatalf("result = %q / %q, want rejected / cancelled", res.Result, res.OrderStatus)
	}
	if got := decideShipmentStatus(t, db, shipment); got != "cancelled" {
		t.Fatalf("shipment status = %q, want cancelled", got)
	}
	// A pending wallet shipment was never charged, so a rejection moves no money:
	// the balance is exactly what it was before the decision.
	if got := fx.walletBalance(t, db); got != "2000.00" {
		t.Fatalf("wallet balance after reject = %q, want 2000.00", got)
	}
	if n := fx.walletWithdrawals(t, db); n != 0 {
		t.Fatalf("%d wallet debits after reject, want 0", n)
	}
}

func TestERPDecide_RepeatConfirm(t *testing.T) {
	db := decideDB(t)
	fx := seedDecideFixture(t, db, decideBase(), 200000)
	fx.seedDecideOrder(t, db, fx.orderNumber, "wallet")
	svc, _ := newDecideService(db)
	ctx := context.Background()

	if _, err := svc.Decide(ctx, fx.principal(), fx.orderNumber, "confirmed", ""); err != nil {
		t.Fatalf("first confirm: %v", err)
	}
	res, err := svc.Decide(ctx, fx.principal(), fx.orderNumber, "confirmed", "")
	if err != nil {
		t.Fatalf("repeat confirm: %v", err)
	}
	if res.Result != "already_confirmed" {
		t.Fatalf("repeat result = %q, want already_confirmed", res.Result)
	}
	if n := fx.walletWithdrawals(t, db); n != 1 {
		t.Fatalf("%d wallet debits after two confirms, want 1", n)
	}
	if got := fx.walletBalance(t, db); got != "906.20" {
		t.Fatalf("wallet balance after two confirms = %q, want 906.20", got)
	}
}

func TestERPDecide_ShortFundsStaysPending(t *testing.T) {
	db := decideDB(t)
	fx := seedDecideFixture(t, db, decideBase(), 1000)
	shipment := fx.seedDecideOrder(t, db, fx.orderNumber, "wallet")
	svc, _ := newDecideService(db)

	_, err := svc.Decide(context.Background(), fx.principal(), fx.orderNumber, "confirmed", "")
	var apiErr *erp.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "ERP_WALLET_INSUFFICIENT" {
		t.Fatalf("short-funds confirm error = %v, want ERP_WALLET_INSUFFICIENT", err)
	}
	if apiErr.Details["requiredAmount"] != "1093.80" || apiErr.Details["availableAmount"] != "10.00" {
		t.Fatalf("details = %v, want requiredAmount 1093.80 and availableAmount 10.00", apiErr.Details)
	}
	if got := decideShipmentStatus(t, db, shipment); got != "pending" {
		t.Fatalf("shipment status = %q, want pending", got)
	}
	if n := fx.walletWithdrawals(t, db); n != 0 {
		t.Fatalf("%d wallet debits with short funds, want 0", n)
	}
}

func TestERPDecide_SameHistoryAsWebsite(t *testing.T) {
	db := decideDB(t)
	fx := seedDecideFixture(t, db, decideBase(), 0)
	apiShipment := fx.seedDecideOrder(t, db, fx.orderNumber, "cash")
	webOrder := fmt.Sprintf("%s-W", fx.orderNumber)
	webShipment := fx.seedDecideOrder(t, db, webOrder, "cash")
	svc, _ := newDecideService(db)
	ctx := context.Background()

	if _, err := svc.Decide(ctx, fx.principal(), fx.orderNumber, "confirmed", ""); err != nil {
		t.Fatalf("api confirm: %v", err)
	}
	// The website's own transition, as the supplier's shipment page calls it.
	comm := commerce.NewService(commercepg.NewRepository(db), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := comm.TransitionShipmentStatusWithWallet(database.WithTenant(ctx, fx.supplier), webShipment, commerce.StatusConfirmed, nil, "", 0, nil); err != nil {
		t.Fatalf("website confirm: %v", err)
	}

	apiHistory := decideHistory(t, db, apiShipment)
	webHistory := decideHistory(t, db, webShipment)
	if len(apiHistory) == 0 || len(apiHistory) != len(webHistory) {
		t.Fatalf("history rows: api %d, website %d", len(apiHistory), len(webHistory))
	}
	for i := range apiHistory {
		if apiHistory[i] != webHistory[i] {
			t.Fatalf("history row %d: api %v, website %v", i, apiHistory[i], webHistory[i])
		}
	}
}

// decideHistory returns the from/to status pairs of one shipment's history.
func decideHistory(t *testing.T, db *database.DB, shipment int64) [][2]string {
	t.Helper()
	var out [][2]string
	err := db.InReadTx(database.AsSystem(context.Background()), func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT from_status, to_status FROM commerce.order_status_history WHERE shipment_id = $1 ORDER BY id`, shipment)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var pair [2]string
			if err := rows.Scan(&pair[0], &pair[1]); err != nil {
				return err
			}
			out = append(out, pair)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	return out
}

func TestERPDecide_AuditRowIsAPIKey(t *testing.T) {
	db := decideDB(t)
	fx := seedDecideFixture(t, db, decideBase(), 0)
	fx.seedDecideOrder(t, db, fx.orderNumber, "cash")
	svc, _ := newDecideService(db)

	if _, err := svc.Decide(context.Background(), fx.principal(), fx.orderNumber, "confirmed", ""); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	var kind string
	var keyID *int64
	err := db.InReadTx(database.AsSystem(context.Background()), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT actor_kind, actor_key_id FROM platform.audit_log WHERE action = 'erp.order.confirm' AND actor_key_id = $1 ORDER BY id DESC LIMIT 1`, fx.keyID).Scan(&kind, &keyID)
	})
	if err != nil {
		t.Fatalf("audit row: %v", err)
	}
	if kind != "api_key" || keyID == nil || *keyID != fx.keyID {
		t.Fatalf("audit actor = %q key %v, want api_key key %d", kind, keyID, fx.keyID)
	}
}
