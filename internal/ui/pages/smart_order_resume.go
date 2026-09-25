package pages

import (
	"fmt"

	"github.com/moqasem061-dot/dawa24/internal/modules/smartorder"
)

// SmartOrderHistoryPageSize is how many runs one history page lists.
const SmartOrderHistoryPageSize = 25

// SmartOrderResume is where a run continues from, and what the button says.
//
// One rule for every status, so no run is ever unreachable from the history:
// a queued run used to be sent to the column mapping it had already passed,
// and anything past the fiftieth run could not be reached at all.
type SmartOrderResume struct {
	URL     string
	Label   string
	Primary bool
}

// SmartOrderResumeFor resolves a run's continuation.
func SmartOrderResumeFor(r *smartorder.Run) SmartOrderResume {
	base := "/customer/smart-order/" + r.PublicID
	switch r.Status {
	case smartorder.StatusCompleted, smartorder.StatusStale:
		return SmartOrderResume{URL: base + "/results", Label: "مراجعة النتائج واستكمال الطلب", Primary: true}
	case smartorder.StatusQueued, smartorder.StatusProcessing, smartorder.StatusFinalizing:
		return SmartOrderResume{URL: base + "/progress", Label: "متابعة المعالجة"}
	case smartorder.StatusPlaced:
		if r.OrderID != nil {
			return SmartOrderResume{URL: fmt.Sprintf("/orders/%d", *r.OrderID), Label: "عرض الطلب"}
		}
		return SmartOrderResume{URL: base + "/review", Label: "عرض الطلب"}
	case smartorder.StatusFailed:
		return SmartOrderResume{URL: base + "/mapping", Label: "تصحيح الأعمدة وإعادة المحاولة"}
	default: // draft, mapping
		return SmartOrderResume{URL: base + "/mapping", Label: "استكمال المسودة", Primary: true}
	}
}
