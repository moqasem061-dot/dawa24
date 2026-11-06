package integration_test

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
)

// S22 on the real database: concurrent and repeated submits of one purchase.
// Each round races several submits at once behind one barrier.

// A double submit (the same token sent twice, one after the other or all at once)
// charges once, and every submit is answered as a success.
func TestSessionPlanPaymentDoubleSubmitChargesOnceOver20Rounds(t *testing.T) {
	w := newPayPlanWorld(t)
	for round := 1; round <= 20; round++ {
		userID, walletID := w.newBuyer(t, "300.00")
		planID := w.newPlan(t, "250.50", false, true, 9)
		token := fmt.Sprintf("tok-same-%s-%02d", w.suffix, round)
		form := url.Values{"purchase_token": {token}}

		if kind, msg := payNotice(w.post(userID, planID, form)); kind != "success" {
			t.Fatalf("round %d: first submit notice = %q (%s), want success", round, kind, msg)
		}
		if kind, msg := payNotice(w.post(userID, planID, form)); kind != "success" {
			t.Fatalf("round %d: repeat submit notice = %q (%s), want success", round, kind, msg)
		}

		forms := make([]url.Values, 8)
		for i := range forms {
			forms[i] = form
		}
		for i, rec := range w.race(userID, planID, forms) {
			if kind, msg := payNotice(rec); kind != "success" {
				t.Fatalf("round %d: racing submit %d notice = %q (%s), want success", round, i, kind, msg)
			}
		}

		if got := w.purchaseCount(t, walletID, token); got != 1 {
			t.Fatalf("round %d: purchase ledger rows = %d, want 1", round, got)
		}
		if got := w.balance(t, walletID); got != "49.50" {
			t.Fatalf("round %d: balance = %s, want 49.50", round, got)
		}
		if got := w.maxSessions(t, userID); got != 9 {
			t.Fatalf("round %d: max login sessions = %d, want 9", round, got)
		}
	}
}

// Concurrent purchases with different tokens against a wallet that covers one of
// them: exactly one is charged, the others are refused, and the balance never
// goes negative.
func TestSessionPlanPaymentConcurrentPurchasesNeverOverdrawOver20Rounds(t *testing.T) {
	w := newPayPlanWorld(t)
	for round := 1; round <= 20; round++ {
		userID, walletID := w.newBuyer(t, "300.00")
		planID := w.newPlan(t, "250.50", false, true, 9)
		forms := make([]url.Values, 8)
		for i := range forms {
			forms[i] = url.Values{"purchase_token": {fmt.Sprintf("tok-dist-%s-%02d-%d", w.suffix, round, i)}}
		}

		successes, refusals := 0, 0
		for i, rec := range w.race(userID, planID, forms) {
			switch kind, msg := payNotice(rec); kind {
			case "success":
				successes++
			case "error":
				refusals++
				if !strings.Contains(msg, "رصيد") {
					t.Fatalf("round %d: refusal %d message %q is not the balance message", round, i, msg)
				}
			default:
				t.Fatalf("round %d: submit %d notice = %q, want success or error", round, i, kind)
			}
		}

		if successes != 1 || refusals != 7 {
			t.Fatalf("round %d: %d successes and %d refusals, want 1 and 7", round, successes, refusals)
		}
		if got := w.debitCount(t, walletID, "-250.50"); got != 1 {
			t.Fatalf("round %d: debit rows of 250.50 = %d, want 1", round, got)
		}
		if got := w.balance(t, walletID); got != "49.50" {
			t.Fatalf("round %d: balance = %s, want 49.50", round, got)
		}
		if got := w.negativeBalances(t, walletID); got != 0 {
			t.Fatalf("round %d: %d ledger rows went negative", round, got)
		}
		if got := w.maxSessions(t, userID); got != 9 {
			t.Fatalf("round %d: max login sessions = %d, want 9", round, got)
		}
	}
}
