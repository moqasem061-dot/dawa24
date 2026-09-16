package sheet

import "testing"

func TestCleanCellDropsNUL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"بانادول\x00إكسترا", "بانادولإكسترا"},
		{"SKU-1234\x00\x00", "SKU-1234"},
		{"\x00", ""},
		{"abc\x01def", "abcdef"},
		{"abc\tdef", "abc def"},
		{"بانادول إكسترا", "بانادول إكسترا"},
	}
	for _, c := range cases {
		if got := CleanCell(c.in); got != c.want {
			t.Errorf("CleanCell(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
