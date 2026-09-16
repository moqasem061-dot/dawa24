package postgres

import (
	"os"
	"strings"
	"testing"
)

// Approving a sponsorship must not move credits.
//
// The credit is reserved when the vendor submits the request and released again
// on rejection or cancellation. ActivateSponsorshipRequest used to charge a
// second time, so every sponsorship cost two credits and a vendor who had spent
// their balance exactly could not be approved at all — the admin pressed
// approve and got "insufficient credits" on a request already paid for.
//
// The service-level tests cover the decision; this covers the SQL, which those
// tests replace with a mock and therefore cannot see.
func TestActivateSponsorshipRequestDoesNotChargeCredits(t *testing.T) {
	src, err := os.ReadFile("sponsorship.go")
	if err != nil {
		t.Fatalf("read sponsorship.go: %v", err)
	}

	body, ok := functionBody(string(src), "func (r *Repository) ActivateSponsorshipRequest(")
	if !ok {
		t.Fatal("ActivateSponsorshipRequest not found — has it moved? This gate is checking nothing.")
	}

	if strings.Contains(body, "promo.sponsorship_purchases") {
		t.Error("activation writes to promo.sponsorship_purchases; the credit was already " +
			"taken at submission and charging again double-bills the vendor")
	}
	if strings.Contains(body, "credits_used = credits_used +") {
		t.Error("activation increments credits_used; approval is a decision, not a purchase")
	}

	// It must still do the two things that make approval mean something.
	if !strings.Contains(body, "admin_status = 'approved'") || !strings.Contains(body, "status = 'active'") {
		t.Error("activation no longer marks the request approved and active")
	}
	if !strings.Contains(body, "promo.offer_sponsorships") {
		t.Error("activation no longer creates the offer_sponsorships row, so the item " +
			"would never actually rank as sponsored")
	}
}

// functionBody returns the source between a function's opening brace and its
// matching close.
func functionBody(src, signature string) (string, bool) {
	i := strings.Index(src, signature)
	if i < 0 {
		return "", false
	}
	open := strings.Index(src[i:], "{")
	if open < 0 {
		return "", false
	}
	start := i + open
	depth := 0
	for j := start; j < len(src); j++ {
		switch src[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[start : j+1], true
			}
		}
	}
	return "", false
}
