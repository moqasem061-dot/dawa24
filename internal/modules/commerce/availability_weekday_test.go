package commerce

import (
	"context"
	"testing"
	"time"
)

type dayProbe struct {
	*stubProbe
	day time.Weekday
}

func (p *dayProbe) VendorCovers(_ context.Context, _, _ int64, _, _ float64, day time.Weekday, _ *int64) (bool, error) {
	p.day = day
	return true, nil
}

// Coverage is scheduled by Cairo's weekday. Half past midnight on a Saturday in
// Cairo is still Friday in UTC, so a weekday read in any other zone applies the
// previous day's schedule for the first hours of every day.
func TestCoverageUsesTheCairoWeekday(t *testing.T) {
	p := &dayProbe{stubProbe: healthyProbe()}
	req := healthyRequest()
	req.When = time.Date(2026, 9, 25, 22, 30, 0, 0, time.UTC) // Fri 22:30 UTC = Sat 01:30 Cairo

	if _, err := serviceWith(p).CheckAvailability(context.Background(), req); err != nil {
		t.Fatalf("CheckAvailability: %v", err)
	}
	if p.day != time.Saturday {
		t.Fatalf("coverage evaluated for %s, want Saturday (Cairo)", p.day)
	}
}
