package ui

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Turning a domain error into a sentence a person can read.
//
// apperr's own comment says the UI layer localises by Code, and this is that
// layer. A handler that put err.Error() in a redirect — several did — showed
// the caller "conflict [org.profile.change_pending]: A change to this section
// is already awaiting review." in the middle of an Arabic page.
//
// The lookup is the code itself: apperr codes are already namespaced
// ("org.profile.change_pending"), so a catalogue key of the same name is the
// message for it. A code with no key falls back to the kind, which is always
// true even when it is not specific.
func (h *UIHandler) errorMessage(r *http.Request, err error) string {
	lang := langOf(r)

	var appErr *apperr.Error
	if !errors.As(err, &appErr) {
		return i18n.T(lang, "errors.500_internal")
	}

	if msg := i18n.Translate(lang, appErr.Code); msg != appErr.Code {
		return msg
	}

	switch appErr.Kind {
	case apperr.KindValidation:
		return i18n.T(lang, "errors.400_bad_request")
	case apperr.KindNotFound:
		return i18n.T(lang, "errors.404_not_found")
	case apperr.KindConflict:
		return i18n.T(lang, "errors.409_conflict")
	case apperr.KindUnauthorized:
		return i18n.T(lang, "errors.401_unauthorized")
	case apperr.KindForbidden:
		return i18n.T(lang, "errors.403_forbidden")
	case apperr.KindRateLimited:
		return i18n.T(lang, "errors.429_rate_limited")
	case apperr.KindUnavailable:
		return i18n.T(lang, "errors.database_error")
	default:
		return i18n.T(lang, "errors.500_internal")
	}
}

// parseInt64PathParam reads a positive integer route parameter.
//
// parseInt64Param reads the query string; a route parameter is a different
// place, and conflating them is how /{id}/withdraw silently acted on id 0.
func parseInt64PathParam(r *http.Request, key string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(r, key)), 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}
