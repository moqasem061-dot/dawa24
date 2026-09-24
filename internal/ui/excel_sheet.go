package ui

import (
	"strings"

	"github.com/xuri/excelize/v2"
)

// excelDefaultSheet is the sheet excelize.NewFile creates.
const excelDefaultSheet = "Sheet1"

// renameFirstSheet names a new workbook's default sheet and returns the name
// actually in effect, which every later write must use.
//
// Excel caps a sheet name at 31 characters and forbids : \ / ? * [ ]. A
// translated title can break either rule, and a rename that fails silently
// leaves every later write aimed at a sheet that does not exist, so the
// download arrives empty. The name is cleaned and truncated instead; if the
// rename still fails, the default name stays and the writes still land.
func renameFirstSheet(f *excelize.File, name string) string {
	clean := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`:\/?*[]`, r) {
			return -1
		}
		return r
	}, name)
	clean = strings.Trim(clean, "' ")
	if rs := []rune(clean); len(rs) > 31 {
		clean = strings.TrimSpace(string(rs[:31]))
	}
	if clean == "" || clean == excelDefaultSheet {
		return excelDefaultSheet
	}
	if err := f.SetSheetName(excelDefaultSheet, clean); err != nil {
		return excelDefaultSheet
	}
	return clean
}
