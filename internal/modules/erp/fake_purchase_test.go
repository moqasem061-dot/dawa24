package erp

import (
	"context"
	"errors"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// fakePurchaseRepo is the in-memory PurchaseRepository for the purchase service
// tests. (fakePurchase already names the order row type in fake_orders_test.go.)
type fakePurchaseRepo struct {
	poOffers      []Offer
	poVariants    map[string]VariantOfferRow
	poLinks       map[[2]int64]bool
	poIdem        map[string]*fakeIdem
	poMarkers     map[string]fakeMarker
	poAudits      []AuditRecord
	poOfferLimit  int
	poIdemDeletes int
	poIdemBegins  int
	poNow         func() time.Time
	poStaleAfter  time.Duration
}

type fakeIdem struct {
	sha, state, orderNumber string
	orderID                 int64
	response                []byte
	started                 time.Time
}

type fakeMarker struct {
	orderID     int64
	orderNumber string
}

func newFakePurchaseRepo() *fakePurchaseRepo {
	return &fakePurchaseRepo{
		poVariants: map[string]VariantOfferRow{}, poLinks: map[[2]int64]bool{}, poIdem: map[string]*fakeIdem{},
		poMarkers: map[string]fakeMarker{}, poNow: time.Now, poStaleAfter: IdempotencyStaleAfter,
	}
}

// addVariant seeds one variant that the buyer may order from.
func (f *fakePurchaseRepo) addVariant(publicID string, id, supplier int64, price, discount money.Amount) {
	f.poVariants[publicID] = VariantOfferRow{ID: id, OrganizationID: supplier, PublicID: publicID, PublicPrice: price,
		DiscountPercent: discount, SupplierName: map[string]string{"en": "Supplier"}, Active: true, InStock: true}
}

func (f *fakePurchaseRepo) ListOffersAsSystem(_ context.Context, _ int64, _ string, limit int) ([]Offer, error) {
	f.poOfferLimit = limit
	out := f.poOffers
	if len(out) > limit {
		out = out[:limit]
	}
	return append([]Offer(nil), out...), nil
}

func (f *fakePurchaseRepo) VariantOfferAsSystem(_ context.Context, publicID string) (VariantOfferRow, error) {
	row, ok := f.poVariants[publicID]
	if !ok {
		return VariantOfferRow{}, ErrNotFound
	}
	return row, nil
}

func (f *fakePurchaseRepo) BeginIdempotency(_ context.Context, _, _ int64, ref, requestSHA string) (IdemState, error) {
	f.poIdemBegins++
	if row, ok := f.poIdem[ref]; ok {
		state := IdemState{RequestSHA: row.sha}
		switch {
		case row.state == "done":
			state.Replay, state.Response = true, row.response
		case f.poNow().Sub(row.started) > f.poStaleAfter:
			state.InProgress, state.Stale = true, true
		default:
			state.InProgress = true
		}
		return state, nil
	}
	f.poIdem[ref] = &fakeIdem{sha: requestSHA, state: "in_progress", started: f.poNow()}
	return IdemState{Created: true, RequestSHA: requestSHA}, nil
}

func (f *fakePurchaseRepo) FinishIdempotency(_ context.Context, _, _ int64, ref string, orderID int64, orderNumber string, response []byte, audit AuditRecord) error {
	row, ok := f.poIdem[ref]
	if !ok {
		return errors.New("no idempotency row")
	}
	row.state, row.orderID, row.orderNumber, row.response = "done", orderID, orderNumber, response
	if audit.Action != "" {
		f.poAudits = append(f.poAudits, audit)
	}
	return nil
}

func (f *fakePurchaseRepo) DeleteIdempotency(_ context.Context, _, _ int64, ref string) error {
	f.poIdemDeletes++
	delete(f.poIdem, ref)
	return nil
}

func (f *fakePurchaseRepo) FindOrderByNoteMarker(_ context.Context, _ int64, marker string) (int64, string, bool, error) {
	m, ok := f.poMarkers[marker]
	return m.orderID, m.orderNumber, ok, nil
}

func (f *fakePurchaseRepo) SupplierLinked(_ context.Context, buyer, supplier int64) (bool, error) {
	return f.poLinks[[2]int64{buyer, supplier}], nil
}

// fakePlacer is the PurchasePlacer behind the service. It records the marker it
// was given, so a stale repair can find the order it wrote.
type fakePlacer struct {
	repo           *fakeRepo
	calls          int
	last           PurchaseRequest
	err            error
	failAfterWrite bool
	next           int64
}

func (p *fakePlacer) Place(_ context.Context, req PurchaseRequest) (PlacedOrder, error) {
	p.calls++
	p.last = req
	if p.err != nil && !p.failAfterWrite {
		return PlacedOrder{}, p.err
	}
	p.next++
	number := "9000" + string(rune('0'+p.next))
	id := 500 + p.next
	p.repo.addPurchase(req.BuyerOrgID, number, "pending", time.Now())
	marker := req.Notes
	if i := indexOfNewline(marker); i >= 0 {
		marker = marker[:i]
	}
	p.repo.poMarkers[marker] = fakeMarker{orderID: id, orderNumber: number}
	if p.err != nil {
		return PlacedOrder{}, p.err
	}
	return PlacedOrder{OrderID: id, OrderNumber: number, Total: money.FromMajor(100)}, nil
}

func indexOfNewline(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return i
		}
	}
	return -1
}
