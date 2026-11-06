package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
)

type paymentStatusRepo struct {
	happyRepo
	paymentStatus commerce.PaymentStatus
}

func (r *paymentStatusRepo) CreateOrder(_ context.Context, order *commerce.Order, _ []*commerce.OrderShipment, _ []*commerce.OrderLine) error {
	r.paymentStatus = order.PaymentStatus
	return nil
}

func TestAPICheckoutNeverAcceptsPaymentStatus(t *testing.T) {
	repo := &paymentStatusRepo{}
	router := newAuthedRouter(repo)
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/commerce/checkout", strings.NewReader(`{"branch_id":3,"payment_method":"cod","payment_status":"paid","items":[{"vendor_org_id":2,"product_variant_id":1,"product_name":{"en":"Example"},"unit_price":"50.00","quantity":1}]}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || repo.paymentStatus != commerce.PaymentUnpaid {
		t.Fatalf("status=%d payment_status=%s body=%s", response.Code, repo.paymentStatus, response.Body.String())
	}
}

// A caller may not create an order with a price that cannot be verified.
func TestAPICheckoutRejectsUnverifiablePrice(t *testing.T) {
	router := newAuthedRouter(happyRepo{})
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/commerce/checkout", strings.NewReader(`{"payment_method":"cod","payment_status":"paid","items":[{"vendor_org_id":1,"product_name":{"en":"Example"},"unit_price":"0.01","quantity":1}]}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code == http.StatusCreated {
		t.Fatalf("unverifiable checkout created an order: %s", response.Body.String())
	}
}
