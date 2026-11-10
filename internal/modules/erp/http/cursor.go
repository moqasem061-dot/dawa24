package http

import (
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
)

const cursorPrefix = "v1:"

// EncodeCursor returns the opaque page token for a keyset position: the decimal
// text of the id, which is the last row returned, so the next page starts after
// it (plan trap 27: keyset, never offset). Decimal text keeps the token within
// the 20-character limit on ids the API returns.
func EncodeCursor(id int64) string {
	return strconv.FormatInt(id, 10)
}

// DecodeCursor reads a page token. The empty token is the start of the feed.
// Decimal text (1 to 19 digits) is an id. The legacy form, base64url of "v1:<id>"
// without padding, is still accepted for tokens handed out before. Anything else
// is refused.
func DecodeCursor(token string) (int64, error) {
	if token == "" {
		return 0, nil
	}
	if isDecimal(token) {
		id, err := strconv.ParseInt(token, 10, 64)
		if err != nil {
			return 0, invalidCursor()
		}
		return id, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || !strings.HasPrefix(string(raw), cursorPrefix) {
		return 0, invalidCursor()
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(string(raw), cursorPrefix), 10, 64)
	if err != nil || id < 0 {
		return 0, invalidCursor()
	}
	return id, nil
}

// isDecimal reports whether s is 1 to 19 ASCII digits.
func isDecimal(s string) bool {
	if len(s) < 1 || len(s) > 19 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func invalidCursor() error {
	return erp.NewError("ERP_CURSOR_INVALID", 400, nil)
}
