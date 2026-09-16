package main

import (
	"sort"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/assistant/tools"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
)

// The assistant explains the order lifecycle from its own copy, because the
// assistant module may not import commerce. This holds the copy to the status
// machine checkout and fulfilment actually enforce.
func TestOrderStatusGuideMatchesCommerce(t *testing.T) {
	all := []commerce.OrderStatus{
		commerce.StatusPending, commerce.StatusProcessing, commerce.StatusConfirmed, commerce.StatusOnHold,
		commerce.StatusShipped, commerce.StatusInTransit, commerce.StatusOutForDelivery, commerce.StatusDelivered,
		commerce.StatusCompleted, commerce.StatusCancelled, commerce.StatusFailed, commerce.StatusReturned, commerce.StatusRefunded,
	}
	guide := tools.OrderStatusGuide()
	if len(guide) != len(all) {
		t.Fatalf("guide covers %d statuses, commerce has %d", len(guide), len(all))
	}
	for _, from := range all {
		var want []string
		for _, to := range all {
			if to != from && commerce.IsValidStatusTransition(from, to) {
				want = append(want, string(to))
			}
		}
		got := guide[string(from)]
		sort.Strings(want)
		sort.Strings(got)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s: guide says %v, commerce allows %v", from, got, want)
		}
	}
}
