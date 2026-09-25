package pages

import (
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/smartorder"
)

// Every run in the history has somewhere to continue from, and it is the
// screen that run is actually at.
func TestEverySmartOrderStatusResumesSomewhereReal(t *testing.T) {
	want := map[smartorder.RunStatus]string{
		smartorder.StatusDraft:      "/mapping",
		smartorder.StatusMapping:    "/mapping",
		smartorder.StatusQueued:     "/progress",
		smartorder.StatusProcessing: "/progress",
		smartorder.StatusCompleted:  "/results",
		smartorder.StatusStale:      "/results",
		smartorder.StatusFinalizing: "/progress",
		smartorder.StatusPlaced:     "/review",
		smartorder.StatusFailed:     "/mapping",
	}
	for status, suffix := range want {
		got := SmartOrderResumeFor(&smartorder.Run{PublicID: "abc", Status: status})
		if !strings.HasSuffix(got.URL, suffix) || got.Label == "" {
			t.Errorf("%s resumes at %q (%q), want …%s", status, got.URL, got.Label, suffix)
		}
	}
	id := int64(42)
	if got := SmartOrderResumeFor(&smartorder.Run{PublicID: "abc", Status: smartorder.StatusPlaced, OrderID: &id}); got.URL != "/orders/42" {
		t.Errorf("a placed run links to %q, want its order", got.URL)
	}
}
