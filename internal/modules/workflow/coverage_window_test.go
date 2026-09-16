package workflow

import "testing"

// The coverage window bounds map to nullable Postgres TIME columns. Two bugs
// shipped against them and both are covered here:
//
//   - the write path passed a Go "" into a TIME column, which Postgres rejects
//     with `invalid input syntax for type time`, so a vendor could not save a
//     coverage row with a blank window;
//   - the read path scanned a TIME into a Go string, which pgx cannot do, so the
//     whole coverage screen errored.
//
// TimeOfDay is the single place a raw form value becomes a window bound, so a
// blank field can never reach the database as "".

func TestTimeOfDay(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want *string // nil means "no window"
	}{
		{"blank field means no window", "", nil},
		{"whitespace only means no window", "   ", nil},
		{"HH:MM passes through", "09:00", strptr("09:00")},
		{"surrounding space is trimmed", "  17:30 ", strptr("17:30")},
		{"browser HH:MM:SS is truncated", "08:15:00", strptr("08:15")},
		{"midnight is a real time, not blank", "00:00", strptr("00:00")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := TimeOfDay(tc.raw)
			switch {
			case tc.want == nil && got != nil:
				t.Fatalf("TimeOfDay(%q) = %q, want nil", tc.raw, *got)
			case tc.want != nil && got == nil:
				t.Fatalf("TimeOfDay(%q) = nil, want %q", tc.raw, *tc.want)
			case tc.want != nil && *got != *tc.want:
				t.Fatalf("TimeOfDay(%q) = %q, want %q", tc.raw, *got, *tc.want)
			}
		})
	}
}

func TestValidTimeOfDay(t *testing.T) {
	cases := []struct {
		in   *string
		want bool
	}{
		{nil, true}, // no window is valid
		{strptr("00:00"), true},
		{strptr("23:59"), true},
		{strptr("09:05"), true},
		{strptr("24:00"), false}, // hour out of range
		{strptr("23:60"), false}, // minute out of range
		{strptr("9:00"), false},  // must be zero-padded so string compare works
		{strptr("09:00:00"), false},
		{strptr("0900"), false},
		{strptr("ab:cd"), false},
		{strptr(""), false}, // blank must arrive as nil, never ""
	}

	for _, tc := range cases {
		label := "<nil>"
		if tc.in != nil {
			label = *tc.in
		}
		if got := validTimeOfDay(tc.in); got != tc.want {
			t.Errorf("validTimeOfDay(%q) = %v, want %v", label, got, tc.want)
		}
	}
}

func TestWeeklyCoverageValidate_Window(t *testing.T) {
	base := func() *WeeklyCoverage {
		return &WeeklyCoverage{BranchID: 1, DayOfWeek: 0, DistanceMeters: 5000}
	}

	cases := []struct {
		name    string
		mutate  func(*WeeklyCoverage)
		wantErr bool
	}{
		{
			name:   "no window at all is valid",
			mutate: func(c *WeeklyCoverage) {},
		},
		{
			name: "a complete window is valid",
			mutate: func(c *WeeklyCoverage) {
				c.CoverageFrom, c.CoverageTo = strptr("09:00"), strptr("17:00")
			},
		},
		{
			name: "start without end is rejected",
			mutate: func(c *WeeklyCoverage) {
				c.CoverageFrom = strptr("09:00")
			},
			wantErr: true,
		},
		{
			name: "end without start is rejected",
			mutate: func(c *WeeklyCoverage) {
				c.CoverageTo = strptr("17:00")
			},
			wantErr: true,
		},
		{
			name: "end before start is rejected",
			mutate: func(c *WeeklyCoverage) {
				c.CoverageFrom, c.CoverageTo = strptr("17:00"), strptr("09:00")
			},
			wantErr: true,
		},
		{
			name: "equal start and end is an empty window, rejected",
			mutate: func(c *WeeklyCoverage) {
				c.CoverageFrom, c.CoverageTo = strptr("09:00"), strptr("09:00")
			},
			wantErr: true,
		},
		{
			name: "malformed time is rejected before it reaches Postgres",
			mutate: func(c *WeeklyCoverage) {
				c.CoverageFrom, c.CoverageTo = strptr("25:99"), strptr("26:00")
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base()
			tc.mutate(c)
			err := c.Validate()
			if tc.wantErr && err == nil {
				t.Fatal("Validate() = nil, want a validation error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

func strptr(s string) *string { return &s }
