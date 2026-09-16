// Package i18n handles the bilingual {"ar":"...","en":"..."} values that the
// legacy schema stores in ~173 columns.
//
// This was one of the better decisions in the original system: product names,
// category names, descriptions and organisation names are all real JSON objects
// with validated structure, not parallel *_ar / *_en columns. We keep that shape
// exactly — in PostgreSQL it becomes JSONB, which indexes and queries better
// than MariaDB's LONGTEXT + CHECK(json_valid(...)) emulation.
package i18n

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Lang is a supported UI/content language.
type Lang string

const (
	AR Lang = "ar"
	EN Lang = "en"
)

// Default is Arabic. This platform serves the Egyptian pharmaceutical market;
// Arabic is the primary language and English is the fallback, not the reverse.
const Default = AR

// IsRTL reports whether a language renders right-to-left. Layout decisions in
// the UI layer key off this rather than hardcoding "ar".
func (l Lang) IsRTL() bool { return l == AR }

func (l Lang) Dir() string {
	if l.IsRTL() {
		return "rtl"
	}
	return "ltr"
}

// ParseLang maps a user preference or Accept-Language fragment onto a supported
// language, falling back to Default rather than erroring. An unknown language is
// a display preference problem, never a request failure.
func ParseLang(s string) Lang {
	switch strings.ToLower(strings.TrimSpace(strings.SplitN(s, "-", 2)[0])) {
	case "en":
		return EN
	case "ar":
		return AR
	default:
		return Default
	}
}

// Text is a translatable string stored as JSONB.
//
// It intentionally holds only the languages that were actually set. A missing
// key is meaningful — it tells the vendor dashboard which translations still
// need filling in — so we do not normalise absent languages into empty strings.
type Text map[Lang]string

// New builds a Text from Arabic and English values.
func New(ar, en string) Text {
	t := Text{}
	if ar != "" {
		t[AR] = ar
	}
	if en != "" {
		t[EN] = en
	}
	return t
}

// Get returns the value for lang, falling back to the other language and then to
// empty. Falling back is deliberate: showing an English product name to an Arabic
// user is far better than showing them a blank row.
func (t Text) Get(lang Lang) string {
	if t == nil {
		return ""
	}
	if v, ok := t[lang]; ok && v != "" {
		return v
	}
	for _, alt := range []Lang{AR, EN} {
		if v, ok := t[alt]; ok && v != "" {
			return v
		}
	}
	return ""
}

// Has reports whether a non-empty translation exists for lang. Vendor-facing
// completeness checks use this.
func (t Text) Has(lang Lang) bool {
	v, ok := t[lang]
	return ok && v != ""
}

// IsEmpty reports whether no language has a value.
func (t Text) IsEmpty() bool {
	return t.Get(AR) == "" && t.Get(EN) == ""
}

// Value implements driver.Valuer for JSONB columns.
func (t Text) Value() (driver.Value, error) {
	if t == nil {
		return nil, nil
	}
	return json.Marshal(t)
}

// Scan implements sql.Scanner for JSONB columns.
//
// It tolerates a bare JSON string as well as an object, because a handful of
// legacy rows were written before the {"ar","en"} convention settled. Those
// decode as Arabic, which matches how the legacy PHP accessor behaved.
func (t *Text) Scan(src any) error {
	if src == nil {
		*t = nil
		return nil
	}

	var raw []byte
	switch v := src.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("i18n: cannot scan %T into Text", src)
	}
	if len(raw) == 0 {
		*t = nil
		return nil
	}

	var obj map[Lang]string
	if err := json.Unmarshal(raw, &obj); err == nil {
		*t = obj
		return nil
	}

	var bare string
	if err := json.Unmarshal(raw, &bare); err == nil {
		*t = Text{AR: bare}
		return nil
	}

	return errors.New("i18n: value is neither a JSON object nor a JSON string")
}
