package ui

import (
	"fmt"
	"net/http"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

func (h *UIHandler) rejectOfferAvailability(
	w http.ResponseWriter, r *http.Request, offerID int64, result commerce.AvailabilityResult, err error,
) {
	message := result.Message(langOf(r))
	if message == "" || err != nil {
		message = i18n.T(langOf(r), "offers.cov_reason_verify_failed")
	}
	if h.isHTMX(r) {
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%+q,"type":"error"}}`, message))
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}
	h.redirectWithNotice(w, r, fmt.Sprintf("/offers/%d", offerID), "error", message)
}
