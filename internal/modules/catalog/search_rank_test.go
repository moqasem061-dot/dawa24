package catalog

import "testing"

func TestFirstWordOf(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"   ", ""},
		{"كريم", "كريم"},
		{"ارت كيو كريم 40 جم", "ارت"},
		{"  انورا توبيكال كريم 50 جم  ", "انورا"},
		{"(بانادول) اكسترا", "بانادول"},
		{"\"اوتريفين\" 15مل", "اوتريفين"},
		{"- خصم -", "خصم"},
	}
	for _, tc := range cases {
		if got := FirstWordOf(tc.in); got != tc.want {
			t.Errorf("FirstWordOf(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
