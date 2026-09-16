package pages

import (
	"context"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
)

// The processing screen shows one number and one line of text.
//
// It previously listed every stage the pipeline had entered, each with a count
// that read "0 / 804" because stages reported before doing their work. This
// asserts the list is gone and the number is present, because the temptation to
// "just also show the stages" is exactly how it grew the first time.
func TestProgressPageShowsOneNumberAndNoStageList(t *testing.T) {
	processed, total := 300, 600
	data := SmartOrderProgressData{
		Run: &smartorder.Run{PublicID: "abc"},
		Events: []*smartorder.Event{
			{Stage: smartorder.StageAdjudicate, Processed: &processed, Total: &total},
		},
		Percent: 69,
		Caption: smartorder.StageAdjudicate.Label(),
	}

	var sb strings.Builder
	if err := SmartOrderProgressPage("ar", "rtl", data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if !strings.Contains(html, "69%") {
		t.Error("the percentage is not on the page")
	}
	if !strings.Contains(html, data.Caption) {
		t.Errorf("the caption %q is not on the page", data.Caption)
	}
	if strings.Contains(html, "(0 / ") {
		t.Error("the old zero-valued stage counters are back")
	}
	if !strings.Contains(html, `aria-valuenow="69"`) {
		t.Error("the ring is not announced to assistive technology")
	}
}

// The ring is drawn by leaving part of its circumference undrawn, so the offset
// has to run the full range or the ring never fills — and a ring stuck at
// three-quarters reads as a stall.
func TestRingOffsetSpansTheWholeCircumference(t *testing.T) {
	empty := SmartOrderProgressData{Percent: 0}.ringOffset()
	full := SmartOrderProgressData{Percent: 100}.ringOffset()
	half := SmartOrderProgressData{Percent: 50}.ringOffset()

	if empty != "326.7" {
		t.Errorf("0%% offset = %s, want the full circumference 326.7", empty)
	}
	if full != "0.0" {
		t.Errorf("100%% offset = %s, want 0.0", full)
	}
	// 326.7/2 is 163.35, which formats to 163.3 — Go rounds half to even.
	if half != "163.3" {
		t.Errorf("50%% offset = %s, want 163.3", half)
	}
}

// Out-of-range input must not draw a broken ring. The percentage is computed
// upstream and clamped there, but a view that renders a negative dash offset
// produces an SVG the browser draws as a full circle — which would report a
// finished run.
func TestRingOffsetClampsOutOfRangeInput(t *testing.T) {
	if got := (SmartOrderProgressData{Percent: -20}).ringOffset(); got != "326.7" {
		t.Errorf("negative percent = %s, want 326.7", got)
	}
	if got := (SmartOrderProgressData{Percent: 250}).ringOffset(); got != "0.0" {
		t.Errorf("percent above 100 = %s, want 0.0", got)
	}
}

// A failed run shows the reason, not a ring at whatever it reached.
func TestProgressPageOnFailureShowsTheReason(t *testing.T) {
	data := SmartOrderProgressData{
		Run:     &smartorder.Run{PublicID: "abc"},
		Failed:  true,
		Percent: 40,
		Message: "تعذّر الاتصال بقاعدة البيانات",
	}

	var sb strings.Builder
	if err := SmartOrderProgressPage("ar", "rtl", data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if !strings.Contains(html, data.Message) {
		t.Error("the failure reason is not on the page")
	}
	if strings.Contains(html, "so-ring-percent") {
		t.Error("a failed run must not still show a progress ring")
	}
	if strings.Contains(html, "http-equiv=\"refresh\"") {
		t.Error("a failed run must stop polling")
	}
}
