package commerce

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// A company never buys from itself, on the cart as well as at checkout.
//
// CheckAvailability is the control and covers the screens that call it, but the
// cart has two paths that do not: AddToCart is reached by the JSON API without
// an availability check of its own, and a cart already holding a line becomes
// self-supplied the moment its owner switches to the company that sells it —
// a cart belongs to a user, and a user may be a member of two companies.

// cartRepo records what reached the repository, so a refusal can be told apart
// from a write that happened to produce nothing.
type cartRepo struct {
	*mockCommerceRepo
	items   []*CartItem
	written int
}

func newCartRepo(items ...*CartItem) *cartRepo {
	return &cartRepo{mockCommerceRepo: newMockCommerceRepo(), items: items}
}

func (r *cartRepo) GetOrCreateCart(_ context.Context, userID int64) (*Cart, error) {
	return &Cart{ID: 1, UserID: userID}, nil
}

func (r *cartRepo) GetCartWithItems(_ context.Context, cartID int64) (*Cart, error) {
	return &Cart{ID: cartID, UserID: 100, Items: r.items}, nil
}

func (r *cartRepo) AddToCartItem(_ context.Context, _ int64, item *CartItem) error {
	r.written++
	r.items = append(r.items, item)
	return nil
}

func cartService(repo Repository) *Service {
	return NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestAddToCartRefusesTheBuyersOwnStock(t *testing.T) {
	const buyerOrg int64 = 42
	repo := newCartRepo()
	svc := cartService(repo)

	_, err := svc.AddToCart(context.Background(), 100, buyerOrg, &CartItem{
		ProductVariantID: 7,
		OrganizationID:   buyerOrg,
		Quantity:         1,
		UnitPrice:        money.MustParse("10.00"),
	})
	if err == nil {
		t.Fatal("AddToCart accepted a line supplied by the buyer's own company")
	}
	if repo.written != 0 {
		t.Errorf("the refused line still reached the repository %d time(s)", repo.written)
	}
}

func TestAddToCartAcceptsAnotherCompanysStock(t *testing.T) {
	repo := newCartRepo()
	svc := cartService(repo)

	cart, err := svc.AddToCart(context.Background(), 100, 42, &CartItem{
		ProductVariantID: 7,
		OrganizationID:   43,
		Quantity:         1,
		UnitPrice:        money.MustParse("10.00"),
	})
	if err != nil {
		t.Fatalf("AddToCart refused another company's stock: %v", err)
	}
	if len(cart.Items) != 1 {
		t.Fatalf("cart holds %d lines, want 1", len(cart.Items))
	}
}

func TestGetCartHidesTheBuyersOwnStock(t *testing.T) {
	const buyerOrg int64 = 42
	own := &CartItem{ID: 1, ProductVariantID: 7, OrganizationID: buyerOrg, Quantity: 1}
	other := &CartItem{ID: 2, ProductVariantID: 8, OrganizationID: 43, Quantity: 1}
	svc := cartService(newCartRepo(own, other))

	cart, err := svc.GetCart(context.Background(), 100, buyerOrg)
	if err != nil {
		t.Fatalf("GetCart: %v", err)
	}
	if len(cart.Items) != 1 || cart.Items[0].ID != other.ID {
		t.Fatalf("cart shows %d line(s); the buyer's own stock is still in it", len(cart.Items))
	}
}

// The same rows come back for the company that may buy them. Hiding is
// per-caller, not a deletion: the line was added while acting for another
// company and is still that company's to order.
func TestGetCartKeepsEveryLineForAnotherCompany(t *testing.T) {
	own := &CartItem{ID: 1, ProductVariantID: 7, OrganizationID: 42, Quantity: 1}
	other := &CartItem{ID: 2, ProductVariantID: 8, OrganizationID: 43, Quantity: 1}
	svc := cartService(newCartRepo(own, other))

	cart, err := svc.GetCart(context.Background(), 100, 99)
	if err != nil {
		t.Fatalf("GetCart: %v", err)
	}
	if len(cart.Items) != 2 {
		t.Fatalf("cart shows %d line(s), want 2", len(cart.Items))
	}
}

// A caller with no company supplies nothing, so nothing is hidden from them.
func TestGetCartHidesNothingWithoutABuyingCompany(t *testing.T) {
	own := &CartItem{ID: 1, ProductVariantID: 7, OrganizationID: 42, Quantity: 1}
	svc := cartService(newCartRepo(own))

	cart, err := svc.GetCart(context.Background(), 100, 0)
	if err != nil {
		t.Fatalf("GetCart: %v", err)
	}
	if len(cart.Items) != 1 {
		t.Fatalf("cart shows %d line(s), want 1", len(cart.Items))
	}
}
