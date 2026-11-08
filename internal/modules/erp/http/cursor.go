package http

import (
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
)

const cursorPrefix = "v1:"

// EncodeCursor returns the opaque page token for a keyset position: base64url
// of "v1:<id>" without padding. The id is the last row returned, so the next
// page starts after it (plan trap 27: keyset, never offset).
func EncodeCursor(id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(cursorPrefix + strconv.FormatInt(id, 10)))
}

// DecodeCursor reads a page token. The empty token is the start of the feed;
// anything that is not a version-1 token with a non-negative id is refused.
func DecodeCursor(token string) (int64, error) {
	if token == "" {
		return 0, nil
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

func invalidCursor() error {
	return erp.NewError("ERP_CURSOR_INVALID", 400, nil)
}
