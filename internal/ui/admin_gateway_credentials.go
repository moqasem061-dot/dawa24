package ui

import (
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// credentialRejectionMessage renders a rejected Gateway administrator
// credential for the connection-test response.
//
// It lives here rather than in admin_handlers.go because that file is already
// the largest in the repository; nothing new is added to it.
func credentialRejectionMessage(err error, langOptional ...string) string {
	lang := "ar"
	if len(langOptional) > 0 && langOptional[0] != "" {
		lang = langOptional[0]
	}
	if e, ok := apperr.As(err); ok {
		return e.LocalizedMsg(lang)
	}
	return i18n.T(lang, "admin.gateway.invalid_credentials")
}
