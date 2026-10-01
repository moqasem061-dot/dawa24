package productmatch_test

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

func TestReasonCodes_HaveI18nKeys(t *testing.T) {
	codes := productmatch.AllReasonCodes()
	if len(codes) != 55 {
		t.Fatalf("expected 55 reason codes, got %d", len(codes))
	}

	for _, code := range codes {
		key := code.I18nKey()

		arText, arOk := i18n.Lookup(i18n.AR, key)
		if !arOk || arText == "" || arText == key {
			t.Errorf("reason code %q: missing Arabic translation for key %q", code, key)
		}

		enText, enOk := i18n.Lookup(i18n.EN, key)
		if !enOk || enText == "" || enText == key {
			t.Errorf("reason code %q: missing English translation for key %q", code, key)
		}
	}
}
