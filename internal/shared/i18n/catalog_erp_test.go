package i18n_test

import (
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"testing"
)

func assertERPTranslations(t *testing.T, keys []string) {
	t.Helper()
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			translate := i18n.T
			ar, en := translate(i18n.AR, key), translate(i18n.EN, key)
			if ar == "" || en == "" || ar == key || en == key || ar == en {
				t.Errorf("missing or identical translations: ar=%q en=%q", ar, en)
			}
		})
	}
}
func TestErpErrorKeys(t *testing.T) {
	assertERPTranslations(t, []string{"erp.err.auth_missing", "erp.err.rate_limited", "erp.err.po_price_mismatch", "erp.err.reveal_forbidden"})
}
func TestErpUIKeys(t *testing.T) {
	assertERPTranslations(t, []string{"erp.keys.title", "erp.keys.change_limit"})
}
