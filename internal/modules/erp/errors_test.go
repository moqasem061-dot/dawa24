package erp

import (
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

func TestAllCodesHaveBilingualMessages(t *testing.T) {
	translate := i18n.T
	for _, code := range AllCodes {
		key := "erp.err." + strings.ToLower(strings.TrimPrefix(code, "ERP_"))
		for _, lang := range []i18n.Lang{i18n.AR, i18n.EN} {
			if got := translate(lang, key); got == "" || got == key {
				t.Errorf("%s: missing %s translation", code, lang)
			}
		}
	}
}
func TestAllCodesCount(t *testing.T) {
	if len(AllCodes) != 58 {
		t.Errorf("got %d codes, want 58", len(AllCodes))
	}
}
func TestNewErrorFields(t *testing.T) {
	err := NewError("ERP_VALIDATION", 422, map[string]any{"field": "name"})
	if err.Code != "ERP_VALIDATION" || err.Status != 422 || err.Details["field"] != "name" || err.Error() != "ERP_VALIDATION" {
		t.Errorf("unexpected error: %+v", err)
	}
}
