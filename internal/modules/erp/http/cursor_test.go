package http

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
)

func TestCursorRoundTrip(t *testing.T) {
	cases := map[int64]string{1: "1", 42: "42", 9_000_000_000: "9000000000", 10542: "10542"}
	for id, want := range cases {
		token := EncodeCursor(id)
		if token != want {
			t.Errorf("EncodeCursor(%d) = %q, want %q", id, token, want)
		}
		got, err := DecodeCursor(token)
		if err != nil || got != id {
			t.Errorf("DecodeCursor(EncodeCursor(%d)) = %d, %v", id, got, err)
		}
	}
}

// The legacy token is base64url of "v1:<id>" and must still be read.
func TestCursorLegacyBase64StillDecodes(t *testing.T) {
	for id, plain := range map[int64]string{7: "v1:7", 42: "v1:42", 9_000_000_000: "v1:9000000000"} {
		token := base64.RawURLEncoding.EncodeToString([]byte(plain))
		got, err := DecodeCursor(token)
		if err != nil || got != id {
			t.Errorf("DecodeCursor(legacy %q) = %d, %v; want %d", token, got, err, id)
		}
	}
}

func TestCursorEmptyMeansStart(t *testing.T) {
	got, err := DecodeCursor("")
	if err != nil || got != 0 {
		t.Errorf("DecodeCursor(\"\") = %d, %v; want 0, nil", got, err)
	}
}

func TestCursorDecimalZeroIsAccepted(t *testing.T) {
	got, err := DecodeCursor("0")
	if err != nil || got != 0 {
		t.Errorf("DecodeCursor(\"0\") = %d, %v; want 0, nil", got, err)
	}
}

func TestCursorRejectsBadInput(t *testing.T) {
	bad := map[string]string{
		"garbage":            "!!!not-base64!!!",
		"wrong version":      base64.RawURLEncoding.EncodeToString([]byte("v2:7")),
		"negative id":        base64.RawURLEncoding.EncodeToString([]byte("v1:-3")),
		"non numeric id":     base64.RawURLEncoding.EncodeToString([]byte("v1:abc")),
		"no separator":       base64.RawURLEncoding.EncodeToString([]byte("v17")),
		"padded form":        "djE6Nw==",
		"minus sign":         "-3",
		"too many digits":    "12345678901234567890",
		"int64 overflow":     "9223372036854775808",
		"digits with spaces": "12 34",
	}
	for name, token := range bad {
		_, err := DecodeCursor(token)
		var apiErr *erp.Error
		if !errors.As(err, &apiErr) || apiErr.Code != "ERP_CURSOR_INVALID" || apiErr.Status != 400 {
			t.Errorf("%s: DecodeCursor(%q) error = %v, want ERP_CURSOR_INVALID 400", name, token, err)
		}
	}
}
