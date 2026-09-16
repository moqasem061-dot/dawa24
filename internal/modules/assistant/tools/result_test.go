package tools

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// orderRow has the shape and size of a real listing row.
type orderRow struct {
	ID            int64        `json:"-"`
	Handle        string       `json:"order"`
	Number        string       `json:"number"`
	Status        string       `json:"status"`
	PaymentStatus string       `json:"payment_status"`
	Subtotal      money.Amount `json:"subtotal"`
	Discount      money.Amount `json:"discount"`
	Shipping      money.Amount `json:"shipping"`
	Total         money.Amount `json:"total"`
	LineCount     int          `json:"line_count"`
	Suppliers     []string     `json:"suppliers,omitempty"`
	PlacedAt      time.Time    `json:"placed_at"`
}

// orderRows builds a listing whose rows are the size real ones are: an Arabic
// supplier name, a signed handle, and seven money or count columns.
func orderRows(n int) []orderRow {
	rows := make([]orderRow, 0, n)
	for i := 0; i < n; i++ {
		rows = append(rows, orderRow{
			ID: int64(i + 1),
			Handle: "horder_AbCdEfGhIjKlMnOpQrStUvWxYz0123456789." +
				"AbCdEfGhIjKlMnOpQrStUvWxYz012345",
			Number:        "PO-2026-000123",
			Status:        "delivered",
			PaymentStatus: "paid",
			Subtotal:      money.FromMinor(123456),
			Discount:      money.FromMinor(1200),
			Shipping:      money.FromMinor(5000),
			Total:         money.FromMinor(127256),
			LineCount:     14,
			Suppliers:     []string{"شركة الدواء المتحدة للتوزيع", "مؤسسة النيل للأدوية"},
			PlacedAt:      time.Date(2026, 8, 14, 9, 30, 0, 0, time.UTC),
		})
	}
	return rows
}

// A full page of orders must reach the model as orders.
//
// This is the regression for the reported "the assistant returns almost no
// data": a 15-row listing — below the tools' own 25-row ceiling — used to
// encode to a 138-byte note containing not one order, because crossing the byte
// budget discarded the payload instead of shrinking it.
func TestEncodeResultKeepsRowsWhenOversize(t *testing.T) {
	for _, n := range []int{15, 20, assistant.PageLimit} {
		rows := orderRows(n)
		out := encodeResult(page(assistant.Page[orderRow]{
			Rows: rows, Total: 240, HasMore: true, NextOffset: n,
		}, "orders"))

		if len(out) > maxResultBytes {
			t.Fatalf("%d rows: encoded %d bytes, over the %d ceiling", n, len(out), maxResultBytes)
		}

		var payload struct {
			Truncated bool `json:"truncated"`
			Shown     int  `json:"shown_rows"`
			Omitted   int  `json:"omitted_rows"`
			Data      struct {
				Orders []json.RawMessage `json:"orders"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(out), &payload); err != nil {
			t.Fatalf("%d rows: result is not valid JSON: %v", n, err)
		}

		got := len(payload.Data.Orders)
		if got < trimFloor {
			t.Fatalf("%d rows: only %d orders survived, want at least %d", n, got, trimFloor)
		}
		if got > n {
			t.Fatalf("%d rows: %d orders returned, more than were asked for", n, got)
		}
		if got == n {
			continue // fitted whole; nothing to assert about trimming
		}
		if !payload.Truncated {
			t.Errorf("%d rows: trimmed to %d without saying so", n, got)
		}
		if payload.Shown != got || payload.Omitted != n-got {
			t.Errorf("%d rows: reported shown=%d omitted=%d, actually showed %d",
				n, payload.Shown, payload.Omitted, got)
		}
	}
}

// A result that fits must be passed through untouched — no trim annotations on
// a listing that was never trimmed, or the model reports missing rows that are
// all present.
func TestEncodeResultLeavesSmallResultsAlone(t *testing.T) {
	out := encodeResult(page(assistant.Page[orderRow]{
		Rows: orderRows(3), Total: 3,
	}, "orders"))

	if strings.Contains(out, "truncated") {
		t.Errorf("a 3-row result was annotated as truncated: %s", out)
	}
	var payload struct {
		Data struct {
			Orders []json.RawMessage `json:"orders"`
			Count  int               `json:"count"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(payload.Data.Orders) != 3 || payload.Data.Count != 3 {
		t.Errorf("got %d orders (count=%d), want 3", len(payload.Data.Orders), payload.Data.Count)
	}
}

// Money and ids must survive the decode-trim-encode round trip exactly. A
// generic JSON tree decoded into float64 would round them, and a subtly wrong
// total in a prompt is worse than a large one.
func TestEncodeResultPreservesNumericsExactly(t *testing.T) {
	rows := orderRows(assistant.PageLimit)
	rows[0].Total = money.FromMinor(987654321987)
	rows[0].Number = "PO-EXACT-1"

	out := encodeResult(page(assistant.Page[orderRow]{
		Rows: rows, Total: 9007199254740993,
	}, "orders"))

	var payload struct {
		Data struct {
			Orders []struct {
				Number string          `json:"number"`
				Total  json.RawMessage `json:"total"`
			} `json:"orders"`
			TotalMatching json.RawMessage `json:"total_matching"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(payload.Data.Orders) == 0 {
		t.Fatal("no orders survived")
	}
	first := payload.Data.Orders[0]
	if first.Number != "PO-EXACT-1" {
		t.Fatalf("first row is %q, ordering was not preserved", first.Number)
	}

	want, err := json.Marshal(money.FromMinor(987654321987))
	if err != nil {
		t.Fatal(err)
	}
	if string(first.Total) != string(want) {
		t.Errorf("total round-tripped to %s, want %s", first.Total, want)
	}
	if string(payload.Data.TotalMatching) != "9007199254740993" {
		t.Errorf("total_matching round-tripped to %s, want 9007199254740993",
			payload.Data.TotalMatching)
	}
}

// A single row too large to fit cannot be trimmed into the budget, and must
// come back as a note rather than as malformed JSON.
func TestEncodeResultUntrimmableFallsBackToNote(t *testing.T) {
	huge := strings.Repeat("د", maxResultBytes)
	out := encodeResult(Result{Data: map[string]any{"blob": huge}, Rows: 1})

	if len(out) > maxResultBytes {
		t.Fatalf("fallback note is %d bytes, over the ceiling", len(out))
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("fallback is not valid JSON: %v", err)
	}
	if _, ok := payload["note"]; !ok {
		t.Errorf("fallback carries no note: %s", out)
	}
}
