package http

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
)

func TestCursorRoundTrip(t *testing.T) {
	for _, id := range []int64{1, 42, 9_000_000_000} {
		token := EncodeCursor(id)
		if want := base64.RawURLEncoding.EncodeToString([]byte("v1:" + itoa(id))); token != want {
			t.Errorf("EncodeCursor(%d) = %q, want %q", id, token, want)
		}
		got, err := DecodeCursor(token)
		if err != nil || got != id {
			t.Errorf("DecodeCursor(EncodeCursor(%d)) = %d, %v", id, got, err)
		}
	}
}

func TestCursorEmptyMeansStart(t *testing.T) {
	got, err := DecodeCursor("")
	if err != nil || got != 0 {
		t.Errorf("DecodeCursor(\"\") = %d, %v; want 0, nil", got, err)
	}
}

func TestCursorRejectsBadInput(t *testing.T) {
	bad := map[string]string{
		"garbage":        "!!!not-base64!!!",
		"wrong version":  base64.RawURLEncoding.EncodeToString([]byte("v2:7")),
		"negative id":    base64.RawURLEncoding.EncodeToString([]byte("v1:-3")),
		"non numeric id": base64.RawURLEncoding.EncodeToString([]byte("v1:abc")),
		"no separator":   base64.RawURLEncoding.EncodeToString([]byte("v17")),
		"padded form":    "djE6Nw==",
	}
	for name, token := range bad {
		_, err := DecodeCursor(token)
		var apiErr *erp.Error
		if !errors.As(err, &apiErr) || apiErr.Code != "ERP_CURSOR_INVALID" || apiErr.Status != 400 {
			t.Errorf("%s: DecodeCursor(%q) error = %v, want ERP_CURSOR_INVALID 400", name, token, err)
		}
	}
}

// itoa keeps the expected-value text independent of strconv in the code under test.
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
