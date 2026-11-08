package mask

import "testing"

// Table tests for the masking rules: the last three characters stay visible,
// values of three characters or fewer are fully masked, and the length is
// counted in characters, so Arabic-Indic digits mask like Latin ones.
func TestPhone(t *testing.T) {
	cases := map[string]string{
		"":                 "",
		"123":              "***",
		"1234":             "*234",
		"01001234567":      "********567",
		"+20 100 123 4567": "*************567",
		"٠١٠٠١٢٣٤٥٦٧":      "********٥٦٧",
	}
	for in, want := range cases {
		if got := Phone(in); got != want {
			t.Errorf("Phone(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNumber(t *testing.T) {
	cases := map[string]string{
		"":         "",
		"ab":       "**",
		"ABCD":     "*BCD",
		"12345678": "*****678",
		"١٢٣٤٥٦٧٨": "*****٦٧٨",
	}
	for in, want := range cases {
		if got := Number(in); got != want {
			t.Errorf("Number(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEmailKeepsTheDomain(t *testing.T) {
	cases := map[string]string{
		"":                      "",
		"ahmed@example.com":     "**med@example.com",
		"ali@clinic.eg":         "***@clinic.eg",
		"a.very.long@pharma.eg": "********ong@pharma.eg",
		"not-an-email":          "*********ail",
	}
	for in, want := range cases {
		if got := Email(in); got != want {
			t.Errorf("Email(%q) = %q, want %q", in, got, want)
		}
	}
}
