package postgres

import (
	"os"
	"strings"
	"testing"
)

// The ledger's guarantees live in one UPDATE and one INSERT, so they are
// checked as SQL rather than round-tripped through a database this test suite
// does not have.

func creditLedgerSource(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile("credit_ledger.go")
	if err != nil {
		t.Fatalf("read credit_ledger.go: %v", err)
	}
	return string(body)
}

// A refund must not manufacture credits. The old increment took a signed count
// and guarded only the upper bound, so two refunds of the same rejected request
// drove credits_used below zero and handed the vendor credits nobody sold them.
func TestCreditMovementCannotDriveTheBalanceNegative(t *testing.T) {
	src := creditLedgerSource(t)
	if !strings.Contains(src, "credits_used + $2 >= 0") {
		t.Error("the UPDATE has no lower bound; a double refund would mint credits")
	}
	if !strings.Contains(src, "credits_used + $2 <= credits_total") {
		t.Error("the UPDATE has no upper bound; a purchase could be overspent")
	}
}

// A refund is allowed against a lapsed package. The credit was taken while the
// package was live, and refusing to return it because the package has since
// expired keeps money the platform did not earn — but a *charge* against a
// lapsed package must still be refused.
func TestRefundsSurviveExpiryButChargesDoNot(t *testing.T) {
	src := creditLedgerSource(t)
	if !strings.Contains(src, "($3 OR (status = 'active' AND expires_at > now()))") {
		t.Error("the liveness guard does not distinguish a refund from a charge")
	}
}

// The counter and its history move together or not at all. A ledger written
// after the transaction closed would omit exactly the movements worth auditing.
func TestTheEntryIsWrittenInsideTheSameTransaction(t *testing.T) {
	src := creditLedgerSource(t)
	update := strings.Index(src, "UPDATE promo.sponsorship_purchases")
	insert := strings.Index(src, "INSERT INTO promo.sponsorship_credit_entries")
	closeTx := strings.Index(src, "\t})\n\tif err != nil {")
	if update < 0 || insert < 0 || closeTx < 0 {
		t.Fatal("ConsumeSponsorshipCredits no longer has the shape this test reads")
	}
	if !(update < insert && insert < closeTx) {
		t.Error("the ledger entry is not written inside the transaction that moves the counter")
	}
}

// A statement's header describes the purchase, not the page of rows on screen.
func TestCreditTotalsAreAggregatedInSQL(t *testing.T) {
	src := creditLedgerSource(t)
	if !strings.Contains(src, "FILTER (WHERE delta < 0)") ||
		!strings.Contains(src, "FILTER (WHERE delta > 0)") {
		t.Error("CreditTotals no longer separates consumption from refunds in SQL")
	}
}
