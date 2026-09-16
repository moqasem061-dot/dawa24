package ingest

// What a run reads the file with, and what it says when it cannot.
//
// This used to hold a second, complete import writer: a streaming path that
// parsed, matched and wrote a whole file in one pass, with its own copy of the
// mode rules, its own variant builder and its own stock rules. Nothing called
// it — the wizard has staged rows through a review screen for some time now —
// but it was the file anyone looking for "where does the import decide things"
// found first, and every setting it read was read a second, different way by
// the commit that actually runs. Two writers with two answers is how a setting
// comes to look like it does nothing. There is one writer now, in
// catalog_commit.go.

import (
	"errors"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// parseOptionsFrom translates the vendor's settings into reading rules.
func parseOptionsFrom(s Settings) productmatch.ParseOptions {
	return productmatch.ParseOptions{
		DefaultMinOrderQty:  s.DefaultMinOrderQty,
		DefaultMinThreshold: s.DefaultMinThreshold,
		BlankQuantityIsZero: s.BlankQuantityIsZero,
		// Always inferred, never a vendor setting. Neither value is written by
		// a vendor import — product_variants has no column for either — but
		// both feed the matcher's strength and dosage-form vetoes, which are
		// what stop a 500 mg row being applied to the 1 g product.
		InferDosageForm:    true,
		InferConcentration: true,
		RejectExpired:      s.RejectExpired,
	}
}

// importFailureMessage renders a run failure for the vendor.
//
// A domain error already carries a message written for them; anything else is
// ours and is prefixed rather than dressed up, because a vendor reading it
// needs to know it was not their file.
func importFailureMessage(err error) string {
	if err == nil {
		return ""
	}
	var domain *apperr.Error
	if errors.As(err, &domain) && domain.Msg != "" {
		return domain.Msg
	}
	return i18n.TDefault("w4_mod.w4str_194_194") + err.Error()
}
