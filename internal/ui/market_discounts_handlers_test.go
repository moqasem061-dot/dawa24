package ui_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/ui"
)

// SuggestMarketDiscounts mirrors what the Postgres query does, over the same
// fixture the page tests use: temporary warehouses only, matched offers grouped
// by product, unmatched ones by name. The SQL itself is exercised against a real
// database in test/integration/market_board_rebuild_test.go.
func (m *mockCompareRepoE2E) SuggestMarketDiscounts(_ context.Context, query string, limit int) ([]compare.MarketSuggestion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	byKey := map[string]*compare.MarketSuggestion{}
	var order []string
	for fileID, rows := range m.fileRows {
		file := m.files[fileID]
		if file == nil || !file.IsTempWarehouse {
			continue
		}
		for _, r := range rows {
			if !strings.Contains(strings.ToLower(r.RawName), strings.ToLower(query)) {
				continue
			}
			key := "n:" + r.RawName
			if r.MatchedProductID != nil {
				key = "p:" + string(rune(*r.MatchedProductID))
			}
			s, ok := byKey[key]
			if !ok {
				s = &compare.MarketSuggestion{ProductID: r.MatchedProductID, FileName: r.RawName, LowestPrice: r.Price}
				byKey[key] = s
				order = append(order, key)
			}
			s.Offers++
			s.Warehouses++
			if r.Discount > s.BestDiscount {
				s.BestDiscount = r.Discount
			}
		}
	}
	var out []compare.MarketSuggestion
	for _, key := range order {
		out = append(out, *byKey[key])
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

type entitlementRepo struct {
	billing.Repository
	allow bool
}

func (r entitlementRepo) CheckOrgEntitlement(context.Context, int64, int64, string) (bool, error) {
	return r.allow, nil
}

type boardFixture struct {
	h    *ui.UIHandler
	repo *mockCompareRepoE2E
}

// newBoardFixture is a board with one temporary warehouse of twelve offers (each
// a different product) and one supplier's private upload that matches the same
// words. bill, when set, gates the page the way a plan does.
func newBoardFixture(t *testing.T, bill *billing.Service) *boardFixture {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := newMockCompareRepoE2E()
	h := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, bill, nil, nil, nil, nil, logger)
	h.SetCompareService(compare.NewService(repo, logger))

	warehouse := &compare.CompareFile{ID: 25, UserID: 1, SupplierName: "مخزن المتحدة", OriginalFilename: "united.xlsx", Status: compare.FileReady, IsTempWarehouse: true}
	_ = repo.CreateFile(context.Background(), warehouse)
	private := &compare.CompareFile{ID: 26, UserID: 100, OrganizationID: &[]int64{200}[0], SupplierName: "قائمة خاصة", OriginalFilename: "private.xlsx", Status: compare.FileReady}
	_ = repo.CreateFile(context.Background(), private)

	var rows []*compare.CompareFileRow
	for i := int64(1); i <= 12; i++ {
		rows = append(rows, &compare.CompareFileRow{
			FileID: warehouse.ID, RawName: "panadol item " + string(rune('a'+i)), Price: money.FromMajor(40),
			Discount: float64(i), PriceAfterDiscount: money.FromMajor(30), MatchedProductID: &[]int64{100 + i}[0],
		})
	}
	rows = append(rows, &compare.CompareFileRow{FileID: private.ID, RawName: "panadol private secret", Price: money.FromMajor(50), Discount: 99, PriceAfterDiscount: money.FromMajor(1)})
	_ = repo.InsertFileRows(context.Background(), rows)
	return &boardFixture{h: h, repo: repo}
}

func vendorActor() authctx.Actor {
	return authctx.Actor{UserID: 100, OrganizationID: 200, OrgType: "vendor", Permissions: []string{"vendor.*"}}
}

func (f *boardFixture) get(actor *authctx.Actor, target string, headers map[string]string, handler func(http.ResponseWriter, *http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", target, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if actor != nil {
		req = req.WithContext(authctx.WithActor(req.Context(), *actor))
	}
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func varyOf(rec *httptest.ResponseRecorder) string {
	return strings.Join(rec.Header().Values("Vary"), ",")
}

var resultsHeaders = map[string]string{"HX-Request": "true", "HX-Target": "md-results"}

// A request that targets the results region gets the results and nothing else;
// every other request gets the whole page. Both say Vary so a cache can never
// show a fragment as a page.
func TestMarketDiscountsFragmentIsResultsOnly(t *testing.T) {
	f := newBoardFixture(t, nil)
	actor := vendorActor()

	page := f.get(&actor, "/market-discounts", nil, f.h.MarketDiscountsPage)
	if page.Code != http.StatusOK {
		t.Fatalf("page status %d", page.Code)
	}
	body := page.Body.String()
	for _, want := range []string{`id="md-page"`, `id="md-results"`, `aria-live="polite"`, `id="market-filter-form"`, "data-no-filter", "data-no-boost", "market-discounts.js", "data-md-fragment"} {
		if !strings.Contains(body, want) {
			t.Errorf("full page lacks %s", want)
		}
	}

	frag := f.get(&actor, "/market-discounts", resultsHeaders, f.h.MarketDiscountsPage)
	if frag.Code != http.StatusOK {
		t.Fatalf("fragment status %d", frag.Code)
	}
	fb := frag.Body.String()
	if !strings.Contains(fb, "data-md-fragment") || !strings.Contains(fb, "md-offers") {
		t.Fatalf("the fragment does not carry the results: %.200s", fb)
	}
	for _, shell := range []string{"<html", "<body", `id="md-page"`, `id="md-results"`, `id="market-filter-form"`, "sidebar", "market-discounts.js"} {
		if strings.Contains(fb, shell) {
			t.Errorf("the fragment carries shell markup %q", shell)
		}
	}
	for name, rec := range map[string]*httptest.ResponseRecorder{"page": page, "fragment": frag} {
		vary := varyOf(rec)
		if !strings.Contains(vary, "HX-Request") || !strings.Contains(vary, "HX-Target") {
			t.Errorf("%s: Vary = %q, want HX-Request and HX-Target", name, vary)
		}
	}

	// Another target (a boosted navigation) is still a page.
	other := f.get(&actor, "/market-discounts", map[string]string{"HX-Request": "true", "HX-Target": "main-content"}, f.h.MarketDiscountsPage)
	if !strings.Contains(other.Body.String(), `id="md-page"`) {
		t.Error("a request targeting another region must get the whole page")
	}
}

// One <article> per offer in either layout; the layout is a class, not a second
// copy of the cards.
func TestMarketDiscountsRendersEachOfferOnce(t *testing.T) {
	f := newBoardFixture(t, nil)
	actor := vendorActor()
	for _, view := range []string{"list", "grid"} {
		rec := f.get(&actor, "/market-discounts?view="+view, resultsHeaders, f.h.MarketDiscountsPage)
		body := rec.Body.String()
		if got := strings.Count(body, "<article"); got != 12 {
			t.Errorf("%s: %d articles, want 12 (one per offer)", view, got)
		}
		if strings.Contains(body, "x-show") || strings.Contains(body, "market-list-row") || strings.Contains(body, "market-card") {
			t.Errorf("%s: the duplicated list/grid markup is back", view)
		}
		if !strings.Contains(body, "md-offers") || !strings.Contains(body, "is-"+view) {
			t.Errorf("%s: the layout class is missing", view)
		}
		if strings.Contains(body, "private secret") {
			t.Errorf("%s: a private upload reached the board", view)
		}
	}
}

// The product filter is parsed, applied and carried through the pager's links
// and the chips; the chip removes it.
func TestMarketDiscountsProductFilterReachesTheService(t *testing.T) {
	f := newBoardFixture(t, nil)
	actor := vendorActor()
	rec := f.get(&actor, "/market-discounts?product=7&min_discount=oops&sort=bogus", resultsHeaders, f.h.MarketDiscountsPage)
	if rec.Code != http.StatusOK {
		t.Fatalf("an invalid value must not fail the board: status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "md-chip") || !strings.Contains(rec.Body.String(), "#7") {
		t.Error("the product filter is not shown as a removable chip")
	}
}

func TestMarketDiscountsSuggest(t *testing.T) {
	suggest := func(f *boardFixture, actor *authctx.Actor, q string) *httptest.ResponseRecorder {
		return f.get(actor, "/market-discounts/suggest?q="+q, nil, f.h.MarketDiscountsSuggest)
	}

	t.Run("unauthenticated callers get no data", func(t *testing.T) {
		rec := suggest(newBoardFixture(t, nil), nil, "panadol")
		if rec.Code != http.StatusUnauthorized || strings.Contains(rec.Body.String(), "panadol item") {
			t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
		}
	})
	t.Run("a customer account gets no data", func(t *testing.T) {
		customer := authctx.Actor{UserID: 5, OrganizationID: 9, OrgType: "customer"}
		rec := suggest(newBoardFixture(t, nil), &customer, "panadol")
		if rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "panadol item") {
			t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
		}
	})
	t.Run("an unentitled organization gets no data", func(t *testing.T) {
		bill := billing.NewService(entitlementRepo{allow: false}, slog.New(slog.NewTextHandler(io.Discard, nil)))
		actor := vendorActor()
		rec := suggest(newBoardFixture(t, bill), &actor, "panadol")
		if rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "panadol item") {
			t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
		}
	})
	t.Run("an entitled organization gets at most eight lines and never a private upload", func(t *testing.T) {
		bill := billing.NewService(entitlementRepo{allow: true}, slog.New(slog.NewTextHandler(io.Discard, nil)))
		actor := vendorActor()
		rec := suggest(newBoardFixture(t, bill), &actor, "panadol")
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		body := rec.Body.String()
		lines := len(regexp.MustCompile(`data-md-kind="(product|text)"`).FindAllString(body, -1))
		if lines == 0 || lines > 8 {
			t.Fatalf("%d suggestion lines, want 1 to 8", lines)
		}
		if !strings.Contains(body, `data-md-kind="all"`) || !strings.Contains(body, `role="option"`) {
			t.Error("the view-all option is missing")
		}
		if !strings.Contains(body, `data-md-product="`) {
			t.Error("a product suggestion must carry its product id")
		}
		if strings.Contains(body, "private secret") {
			t.Fatal("a private compare upload was suggested")
		}
		if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
			t.Errorf("Cache-Control = %q, want no-store", got)
		}
	})
	t.Run("a one-character query returns nothing", func(t *testing.T) {
		actor := vendorActor()
		rec := suggest(newBoardFixture(t, nil), &actor, "p")
		if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "" {
			t.Fatalf("status=%d body=%q, want an empty 200", rec.Code, rec.Body.String())
		}
	})
	t.Run("a failed lookup is a note, not an error page", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		h := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)
		// No compare service wired: the dropdown still answers.
		req := httptest.NewRequest("GET", "/market-discounts/suggest?q=panadol", nil)
		actor := vendorActor()
		req = req.WithContext(authctx.WithActor(req.Context(), actor))
		rec := httptest.NewRecorder()
		h.MarketDiscountsSuggest(rec, req)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `data-md-note="error"`) || !strings.Contains(rec.Body.String(), `data-md-kind="all"`) {
			t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
		}
	})
}
