package ui

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/ingest"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// A page whose data took the whole request budget to load must still render
// in full. templ stops at the first component that sees ctx.Err(), and under
// the request deadline that cut the vendor import review off after the bulk
// bar, with its table missing and no error shown.
func TestAPageStartedAfterTheDeadlineRendersInFull(t *testing.T) {
	h := &UIHandler{log: slog.New(slog.NewTextHandler(io.Discard, nil))}

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	rows := make([]*ingest.RowOutcome, 0, 25)
	for i := 1; i <= 25; i++ {
		rows = append(rows, &ingest.RowOutcome{ID: int64(i), SourceRow: i, DisplayName: "صنف"})
	}
	view := pages.VendorImportView{Lang: "ar",
		Session: &ingest.Session{PublicID: "p", Phase: ingest.PhaseReview, TotalRows: 25},
		Rows:    rows, RowTotal: 25, Page: 1, PerPage: 25}

	w := httptest.NewRecorder()
	h.renderPage(ctx, w, "review", pages.VendorIngestPage(view, "ar", "rtl"))

	body := w.Body.String()
	for _, want := range []string{"review-table", `id="row-25"`, "</html>"} {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered page is missing %q — it was cut off at the deadline (%d bytes)", want, len(body))
		}
	}
}

// The render context keeps what pages read from the request — nonce, actor,
// language — and loses only the deadline.
func TestRenderContextKeepsValuesDropsDeadline(t *testing.T) {
	type key struct{}
	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), key{}, "nonce"), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)

	rc := renderContext(ctx)
	if rc.Err() != nil {
		t.Fatalf("render context carries the request's expiry: %v", rc.Err())
	}
	if rc.Value(key{}) != "nonce" {
		t.Fatal("render context lost the request's values")
	}
}
