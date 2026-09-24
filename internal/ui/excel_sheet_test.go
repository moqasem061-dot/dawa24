package ui

import (
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestRenameFirstSheet(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain", "Employees", "Employees"},
		{"arabic", "قائمة الأسعار", "قائمة الأسعار"},
		{"forbidden characters dropped", "Offers: 2026/09 [draft]?", "Offers 202609 draft"},
		{"truncated to 31 runes", strings.Repeat("ع", 40), strings.Repeat("ع", 31)},
		{"empty keeps default", "  ", excelDefaultSheet},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := excelize.NewFile()
			defer func() { _ = f.Close() }()
			got := renameFirstSheet(f, tc.in)
			if got != tc.want {
				t.Fatalf("renameFirstSheet(%q) = %q, want %q", tc.in, got, tc.want)
			}
			// The returned name must be writable: this is what the handlers rely on.
			if err := f.SetCellValue(got, "A1", "x"); err != nil {
				t.Fatalf("write to %q: %v", got, err)
			}
		})
	}
}
