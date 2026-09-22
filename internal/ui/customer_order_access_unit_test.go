package ui

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func TestEvaluateCustomerOrderAccess(t *testing.T) {
	buyerOrgID := int64(100)
	vendorOrgID := int64(200)
	otherOrgID := int64(300)

	order := &commerce.Order{
		ID:             1,
		CustomerID:     50,
		OrganizationID: &buyerOrgID,
		Shipments: []*commerce.OrderShipment{
			{ID: 10, OrderID: 1, OrganizationID: vendorOrgID},
		},
	}

	tests := []struct {
		name     string
		actor    authctx.Actor
		expected OrderAccessDecision
	}{
		{
			name: "buyer by customer ID",
			actor: authctx.Actor{
				UserID: 50,
			},
			expected: OrderAccessAllowed,
		},
		{
			name: "buyer by organization ID",
			actor: authctx.Actor{
				UserID:         99,
				OrganizationID: buyerOrgID,
			},
			expected: OrderAccessAllowed,
		},
		{
			name: "platform staff",
			actor: authctx.Actor{
				UserID:  77,
				IsStaff: true,
			},
			expected: OrderAccessAllowed,
		},
		{
			name: "vendor fulfilling shipment on order",
			actor: authctx.Actor{
				UserID:         88,
				OrganizationID: vendorOrgID,
			},
			expected: OrderAccessRedirectToVendor,
		},
		{
			name: "unrelated vendor or pharmacy organization",
			actor: authctx.Actor{
				UserID:         999,
				OrganizationID: otherOrgID,
			},
			expected: OrderAccessDenied,
		},
		{
			name: "unrelated individual customer",
			actor: authctx.Actor{
				UserID: 999,
			},
			expected: OrderAccessDenied,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			decision := evaluateCustomerOrderAccess(tc.actor, order)
			if decision != tc.expected {
				t.Errorf("evaluateCustomerOrderAccess() = %v, want %v", decision, tc.expected)
			}
		})
	}

	// Test nil order
	if decision := evaluateCustomerOrderAccess(authctx.Actor{UserID: 1}, nil); decision != OrderAccessDenied {
		t.Errorf("nil order should return OrderAccessDenied, got %v", decision)
	}
}

func TestParseSingleInstitutionalWork(t *testing.T) {
	// 1. Multiple values in checkbox array should be trimmed to 1
	r1, _ := http.NewRequest(http.MethodPost, "/", nil)
	r1.Form = url.Values{
		"institutional_works": []string{"10", "20", "30"},
	}
	got1 := parseSingleInstitutionalWork(r1)
	if len(got1) != 1 || got1[0] != "10" {
		t.Errorf("expected ['10'], got %v", got1)
	}

	// 2. Single radio value in post form
	r2, _ := http.NewRequest(http.MethodPost, "/", nil)
	r2.PostForm = url.Values{
		"institutional_work": []string{"25"},
	}
	got2 := parseSingleInstitutionalWork(r2)
	if len(got2) != 1 || got2[0] != "25" {
		t.Errorf("expected ['25'], got %v", got2)
	}

	// 3. Empty form should return empty
	r3, _ := http.NewRequest(http.MethodPost, "/", nil)
	r3.Form = url.Values{}
	got3 := parseSingleInstitutionalWork(r3)
	if len(got3) != 0 {
		t.Errorf("expected empty, got %v", got3)
	}
}

func TestResolveCustomerInstitutionalWork_SingleLimit(t *testing.T) {
	h := &UIHandler{}
	ctx := context.Background()

	// Multiple values trimmed to 1
	r1, _ := http.NewRequest(http.MethodPost, "/", nil)
	r1.Form = url.Values{
		"institutional_works": []string{"1", "2"},
	}
	got1 := h.resolveCustomerInstitutionalWork(ctx, r1)
	if len(got1) != 1 || got1[0] != "1" {
		t.Errorf("expected ['1'], got %v", got1)
	}

	// Single radio value preserved
	r2, _ := http.NewRequest(http.MethodPost, "/", nil)
	r2.PostForm = url.Values{
		"institutional_work": []string{"5"},
	}
	got2 := h.resolveCustomerInstitutionalWork(ctx, r2)
	if len(got2) != 1 || got2[0] != "5" {
		t.Errorf("expected ['5'], got %v", got2)
	}
}
