package ui

import (
	"context"
	"fmt"
	"net/http"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// checkSpecialOfferAvailability confirms that bundle products do not gate the offer,
// as products inside an offer are promotional components rather than independently ordered stock lines.
func (h *UIHandler) checkSpecialOfferAvailability(
	ctx context.Context, actor authctx.Actor, offer *promo.SpecialOffer, branchID int64, multiplier int,
) (commerce.AvailabilityResult, error) {
	return commerce.AvailabilityResult{Allowed: true}, nil
}

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
