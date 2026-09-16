package pagination_test

import (
	"net/http"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/pagination"
)

func TestPaginationNormalize(t *testing.T) {
	tests := []struct {
		inputLimit    int
		expectedLimit int
	}{
		{0, pagination.DefaultLimit},
		{-5, pagination.DefaultLimit},
		{25, 25},
		{200, 200},
		{500, pagination.MaxLimit},
	}

	for _, tt := range tests {
		p := pagination.Params{Limit: tt.inputLimit}
		p.Normalize()
		if p.Limit != tt.expectedLimit {
			t.Errorf("Limit %d got normalized to %d, want %d", tt.inputLimit, p.Limit, tt.expectedLimit)
		}
	}
}

func TestCursorRoundTrip(t *testing.T) {
	id := int64(987654321)
	sortVal := "2026-08-16T12:00:00Z"

	token := pagination.EncodeCursor(id, sortVal)
	if token == "" {
		t.Fatal("expected non-empty cursor token")
	}

	decodedID, decodedSortVal, err := pagination.DecodeCursor(token)
	if err != nil {
		t.Fatalf("unexpected error decoding cursor: %v", err)
	}

	if decodedID != id || decodedSortVal != sortVal {
		t.Errorf("decoded (%d, %s), want (%d, %s)", decodedID, decodedSortVal, id, sortVal)
	}
}

func TestFromRequest(t *testing.T) {
	req, _ := http.NewRequest("GET", "/api/v1/items?limit=75&cursor=abc&offset=10", nil)
	p := pagination.FromRequest(req)

	if p.Limit != 75 || p.Cursor != "abc" || p.Offset != 10 {
		t.Errorf("unexpected params from request: %+v", p)
	}

	// Default fallback on empty query
	emptyReq, _ := http.NewRequest("GET", "/api/v1/items", nil)
	pEmpty := pagination.FromRequest(emptyReq)
	if pEmpty.Limit != pagination.DefaultLimit {
		t.Errorf("expected default limit %d, got %d", pagination.DefaultLimit, pEmpty.Limit)
	}
}

func TestDecodeInvalidCursor(t *testing.T) {
	_, _, err := pagination.DecodeCursor("not-valid-base64-json@@@")
	if err == nil {
		t.Error("expected error on corrupted cursor token, got nil")
	}
}
