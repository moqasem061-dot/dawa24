package erp

import "testing"

func TestParseShortID(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"1", 1, true},
		{"4821", 4821, true},
		{"9223372036854775807", 9223372036854775807, true},
		{"", 0, false},
		{"0", 0, false},
		{"00", 0, false},
		{"-5", 0, false},
		{"+5", 0, false},
		{"12a", 0, false},
		{" 12", 0, false},
		{"1.5", 0, false},
		{"9223372036854775808", 0, false},
		{"10000000000000000000", 0, false},
		{"3f2504e0-4f89-11d3-9a0c-0305e82c3301", 0, false},
	}
	for _, tc := range cases {
		got, ok := ParseShortID(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("ParseShortID(%q) = %d, %v; want %d, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
