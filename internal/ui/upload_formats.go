package ui

// What an upload screen accepts, decided in one place.
//
// Five screens carried the same three-way extension test, and all five were
// narrower than the reader behind them: internal/shared/sheet decodes OOXML,
// legacy BIFF, the Office XML 2003 dialect, an HTML table and any delimited
// text file, and it decides which by SNIFFING THE BYTES rather than by
// trusting the name. A supplier who exports "prices.xlsm" from their ERP, or
// whose accounting package writes a tab dump named ".txt", was refused at the
// door by a screen that could have read the file perfectly.
//
// The extension check is kept, and this is what it is for: catching an obvious
// mistake before the file is read at all — someone picking a PDF, an image, a
// ZIP of invoices. It is a courtesy, not a format decision. Whether the bytes
// are a spreadsheet is the reader's answer to give, and its refusal names the
// real problem where a generic "unsupported format" never could.

import (
	"path/filepath"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// uploadExtensions are the names a spreadsheet upload plausibly arrives under.
//
// The empty string is included because a browser can post a file with no
// extension at all, and refusing that tells the user nothing about a file the
// reader may well decode.
var uploadExtensions = map[string]struct{}{
	"":      {},
	".xlsx": {}, ".xlsm": {}, ".xltx": {}, ".xls": {},
	".csv": {}, ".txt": {}, ".tsv": {}, ".tab": {},
	".htm": {}, ".html": {}, ".xml": {},
}

// SupportedUploadName reports whether a filename is worth reading at all.
func SupportedUploadName(name string) bool {
	_, ok := uploadExtensions[strings.ToLower(filepath.Ext(name))]
	return ok
}

// unsupportedUploadMsg is what a refused upload is told, and it names the
// formats a person recognises rather than the eleven extensions above.
func unsupportedUploadMsg(lang string) string {
	return i18n.T(lang, "common.unsupported_upload")
}
