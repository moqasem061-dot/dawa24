package pages

import (
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// erpReasonLabel is the label for a review reason code, for example ERP_NO_MATCH
// gives the erp.err.no_match text. The key is built at run time, so it is looked
// up directly: i18n.T formats its key, and go vet rejects a dynamic format string.
func erpReasonLabel(lang, reasonCode string) string {
	key := "erp.err." + strings.ToLower(strings.TrimPrefix(reasonCode, "ERP_"))
	if text, ok := i18n.Lookup(i18n.ParseLang(lang), key); ok {
		return text
	}
	if text, ok := i18n.Lookup(i18n.Default, key); ok {
		return text
	}
	return key
}
