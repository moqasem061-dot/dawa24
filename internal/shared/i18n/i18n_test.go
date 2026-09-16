package i18n_test

import (
	"database/sql/driver"
	"reflect"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

func TestLang(t *testing.T) {
	if !i18n.AR.IsRTL() {
		t.Errorf("AR.IsRTL() = false; want true")
	}
	if i18n.EN.IsRTL() {
		t.Errorf("EN.IsRTL() = true; want false")
	}

	if i18n.AR.Dir() != "rtl" {
		t.Errorf("AR.Dir() = %q; want %q", i18n.AR.Dir(), "rtl")
	}
	if i18n.EN.Dir() != "ltr" {
		t.Errorf("EN.Dir() = %q; want %q", i18n.EN.Dir(), "ltr")
	}

	if i18n.Default != i18n.AR {
		t.Errorf("Default = %v; want %v (Arabic-first)", i18n.Default, i18n.AR)
	}
}

func TestParseLang(t *testing.T) {
	tests := []struct {
		input    string
		expected i18n.Lang
	}{
		{"ar", i18n.AR},
		{"AR", i18n.AR},
		{"ar-EG", i18n.AR},
		{"ar-SA", i18n.AR},
		{"en", i18n.EN},
		{"EN", i18n.EN},
		{"en-US", i18n.EN},
		{"en-GB", i18n.EN},
		{"fr", i18n.Default},
		{"", i18n.Default},
		{"  ", i18n.Default},
		{"es-ES", i18n.Default},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := i18n.ParseLang(tt.input)
			if got != tt.expected {
				t.Errorf("ParseLang(%q) = %v; want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNew(t *testing.T) {
	t.Run("both populated", func(t *testing.T) {
		text := i18n.New("دواء", "Medicine")
		if text[i18n.AR] != "دواء" || text[i18n.EN] != "Medicine" {
			t.Errorf("New() = %+v; want AR and EN set", text)
		}
	})

	t.Run("ar only", func(t *testing.T) {
		text := i18n.New("دواء", "")
		if text[i18n.AR] != "دواء" {
			t.Errorf("text[AR] = %q; want %q", text[i18n.AR], "دواء")
		}
		if _, ok := text[i18n.EN]; ok {
			t.Errorf("text[EN] should not be present when initialized empty")
		}
	})

	t.Run("en only", func(t *testing.T) {
		text := i18n.New("", "Medicine")
		if text[i18n.EN] != "Medicine" {
			t.Errorf("text[EN] = %q; want %q", text[i18n.EN], "Medicine")
		}
		if _, ok := text[i18n.AR]; ok {
			t.Errorf("text[AR] should not be present when initialized empty")
		}
	})
}

func TestTextGet(t *testing.T) {
	var nilText i18n.Text
	if nilText.Get(i18n.AR) != "" {
		t.Errorf("nil.Get(AR) = %q; want empty string", nilText.Get(i18n.AR))
	}

	emptyText := i18n.Text{}
	if emptyText.Get(i18n.AR) != "" {
		t.Errorf("empty.Get(AR) = %q; want empty string", emptyText.Get(i18n.AR))
	}

	full := i18n.New("بانادول", "Panadol")
	if full.Get(i18n.AR) != "بانادول" {
		t.Errorf("full.Get(AR) = %q; want %q", full.Get(i18n.AR), "بانادول")
	}
	if full.Get(i18n.EN) != "Panadol" {
		t.Errorf("full.Get(EN) = %q; want %q", full.Get(i18n.EN), "Panadol")
	}

	// Fallback behavior: if requested language is missing, falls back to the available language
	arOnly := i18n.New("بانادول", "")
	if arOnly.Get(i18n.EN) != "بانادول" {
		t.Errorf("arOnly.Get(EN) fallback = %q; want %q", arOnly.Get(i18n.EN), "بانادول")
	}

	enOnly := i18n.New("", "Panadol")
	if enOnly.Get(i18n.AR) != "Panadol" {
		t.Errorf("enOnly.Get(AR) fallback = %q; want %q", enOnly.Get(i18n.AR), "Panadol")
	}
}

func TestTextHasAndIsEmpty(t *testing.T) {
	var nilText i18n.Text
	if nilText.Has(i18n.AR) {
		t.Errorf("nilText.Has(AR) should be false")
	}
	if !nilText.IsEmpty() {
		t.Errorf("nilText.IsEmpty() should be true")
	}

	arOnly := i18n.New("دواء", "")
	if !arOnly.Has(i18n.AR) {
		t.Errorf("arOnly.Has(AR) should be true")
	}
	if arOnly.Has(i18n.EN) {
		t.Errorf("arOnly.Has(EN) should be false")
	}
	if arOnly.IsEmpty() {
		t.Errorf("arOnly.IsEmpty() should be false")
	}

	empty := i18n.Text{}
	if !empty.IsEmpty() {
		t.Errorf("empty.IsEmpty() should be true")
	}
}

func TestTextValue(t *testing.T) {
	var nilText i18n.Text
	val, err := nilText.Value()
	if err != nil || val != nil {
		t.Errorf("nilText.Value() = (%v, %v); want (nil, nil)", val, err)
	}

	text := i18n.New("دواء", "Medicine")
	val, err = text.Value()
	if err != nil {
		t.Fatalf("text.Value() error: %v", err)
	}

	b, ok := val.([]byte)
	if !ok {
		t.Fatalf("val is not []byte: %T", val)
	}
	if len(b) == 0 {
		t.Errorf("serialized JSON is empty")
	}
}

func TestTextScan(t *testing.T) {
	t.Run("nil source", func(t *testing.T) {
		var text i18n.Text
		if err := text.Scan(nil); err != nil {
			t.Fatalf("Scan(nil) error: %v", err)
		}
		if text != nil {
			t.Errorf("text after Scan(nil) = %+v; want nil", text)
		}
	})

	t.Run("empty bytes/string", func(t *testing.T) {
		var text i18n.Text
		if err := text.Scan([]byte{}); err != nil {
			t.Fatalf("Scan([]) error: %v", err)
		}
		if text != nil {
			t.Errorf("text after Scan([]) = %+v; want nil", text)
		}

		if err := text.Scan(""); err != nil {
			t.Fatalf("Scan(\"\") error: %v", err)
		}
		if text != nil {
			t.Errorf("text after Scan(\"\") = %+v; want nil", text)
		}
	})

	t.Run("json object []byte and string", func(t *testing.T) {
		var text i18n.Text
		jsonBytes := []byte(`{"ar":"بانادول","en":"Panadol"}`)
		if err := text.Scan(jsonBytes); err != nil {
			t.Fatalf("Scan(bytes) error: %v", err)
		}
		expected := i18n.New("بانادول", "Panadol")
		if !reflect.DeepEqual(text, expected) {
			t.Errorf("Scan() = %+v; want %+v", text, expected)
		}

		var textStr i18n.Text
		if err := textStr.Scan(string(jsonBytes)); err != nil {
			t.Fatalf("Scan(string) error: %v", err)
		}
		if !reflect.DeepEqual(textStr, expected) {
			t.Errorf("Scan(string) = %+v; want %+v", textStr, expected)
		}
	})

	t.Run("bare string backwards compatibility", func(t *testing.T) {
		var text i18n.Text
		bareJSON := []byte(`"بانادول إكسترا"`)
		if err := text.Scan(bareJSON); err != nil {
			t.Fatalf("Scan(bare string) error: %v", err)
		}
		if text.Get(i18n.AR) != "بانادول إكسترا" {
			t.Errorf("bare string decoded = %q; want %q", text.Get(i18n.AR), "بانادول إكسترا")
		}
	})

	t.Run("invalid type", func(t *testing.T) {
		var text i18n.Text
		if err := text.Scan(12345); err == nil {
			t.Fatal("expected error on Scan(int), got nil")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		var text i18n.Text
		if err := text.Scan([]byte(`{unquoted: key}`)); err == nil {
			t.Fatal("expected error on Scan(invalid json), got nil")
		}
	})

	// Ensure driver.Valuer / sql.Scanner interface satisfaction
	var _ driver.Valuer = i18n.Text{}
}

func TestEngineTranslationAndOverrides(t *testing.T) {
	// Test basic key lookup
	arSave := i18n.T(i18n.AR, "common.save")
	if arSave != "حفظ" {
		t.Errorf("T(AR, common.save) = %q; want %q", arSave, "حفظ")
	}
	enSave := i18n.T(i18n.EN, "common.save")
	if enSave != "Save" {
		t.Errorf("T(EN, common.save) = %q; want %q", enSave, "Save")
	}

	// Test fallback for missing key
	unknown := i18n.T(i18n.EN, "unknown.custom.key")
	if unknown != "unknown.custom.key" {
		t.Errorf("T(EN, unknown.custom.key) = %q; want %q", unknown, "unknown.custom.key")
	}

	// Test arguments format
	key := "common.rows_per_page"
	arCount := i18n.T(i18n.AR, key, 50)
	if arCount != "50 صفوف" {
		t.Errorf("T(AR, common.rows_per_page, 50) = %q; want %q", arCount, "50 صفوف")
	}

	// Test Override
	i18n.SetOverride("common.save", i18n.New("حفظ فوري", "Instant Save"))
	if got := i18n.T(i18n.AR, "common.save"); got != "حفظ فوري" {
		t.Errorf("Overridden T(AR) = %q; want %q", got, "حفظ فوري")
	}
	if got := i18n.T(i18n.EN, "common.save"); got != "Instant Save" {
		t.Errorf("Overridden T(EN) = %q; want %q", got, "Instant Save")
	}

	// Test RemoveOverride
	i18n.RemoveOverride("common.save")
	if got := i18n.T(i18n.AR, "common.save"); got != "حفظ" {
		t.Errorf("Reverted T(AR) = %q; want %q", got, "حفظ")
	}

	// Test GetAllKeyEntries
	entries := i18n.GetAllKeyEntries()
	if len(entries) == 0 {
		t.Error("GetAllKeyEntries returned empty slice")
	}

	// Test GetNamespaces
	namespaces := i18n.GetNamespaces()
	if len(namespaces) == 0 {
		t.Error("GetNamespaces returned empty slice")
	}
}

func TestShellUIKeysTranslation(t *testing.T) {
	keys := []string{
		"admin.shell.subtitle",
		"pharmacy.shell.subtitle",
		"vendor.shell.subtitle",
		"branch.locked_tooltip",
		"branch.select_active",
		"auth.notice.title",
		"auth.notice.understood",
		"preview.modal.title",
		"preview.modal.download",
		"nav.exit_dashboard",
		"toast.network_error",
		"toast.success",
	}

	for _, k := range keys {
		ar, okAR := i18n.Lookup(i18n.AR, k)
		if !okAR || ar == "" {
			t.Errorf("expected translated AR for key %q, got %q", k, ar)
		}
		en, okEN := i18n.Lookup(i18n.EN, k)
		if !okEN || en == "" {
			t.Errorf("expected translated EN for key %q, got %q", k, en)
		}
		if ar == en {
			t.Errorf("key %q has identical AR and EN translation: %q", k, ar)
		}
	}
}

