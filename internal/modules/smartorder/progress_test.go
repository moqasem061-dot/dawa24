package smartorder

import "testing"

func event(stage Stage, processed, total int) *Event {
	return &Event{Stage: stage, Processed: &processed, Total: &total}
}

// The bands must be contiguous and reach 100. A gap shows as the bar jumping;
// an overlap shows as it retreating, and both read as a fault.
func TestStageBandsTileTheWholeBar(t *testing.T) {
	// StageAdjudicate is deliberately absent: it is the AI stage's former name
	// and shares StageAIEnhance's band so that pre-rework runs still render.
	// Walking both would read as an overlap.
	order := []Stage{
		StageParse, StageNormalize, StageResolve, StageCandidates,
		StageInitialDone, StageAIEnhance, StageSelect, StageFinalize,
	}

	prevEnd := 0
	for _, s := range order {
		start, end := s.Band()
		if start != prevEnd {
			t.Errorf("%s starts at %d, want %d — the bar would jump", s, start, prevEnd)
		}
		if end <= start {
			t.Errorf("%s has an empty band (%d..%d)", s, start, end)
		}
		prevEnd = end
	}
	if prevEnd != 100 {
		t.Errorf("bands end at %d, want 100", prevEnd)
	}
}

// Progress inside a stage interpolates across that stage's band. The AI stage
// is the one that matters: it is the slowest, and reporting only its start and
// end left the buyer watching a still number through the longest minute of the
// run.
func TestPercentInterpolatesWithinAStage(t *testing.T) {
	start, end := StageAIEnhance.Band()

	if got := event(StageAIEnhance, 0, 600).Percent(); got != start {
		t.Errorf("nothing done = %d%%, want %d%%", got, start)
	}
	if got := event(StageAIEnhance, 600, 600).Percent(); got != end {
		t.Errorf("all done = %d%%, want %d%%", got, end)
	}

	half := event(StageAIEnhance, 300, 600).Percent()
	mid := start + (end-start)/2
	if half != mid {
		t.Errorf("half done = %d%%, want %d%%", half, mid)
	}
}

// A stage with no counts sits at the foot of its band rather than reporting
// zero. Falling to 0% mid-run is what makes a bar look broken.
func TestPercentWithoutCountsHoldsTheBandFloor(t *testing.T) {
	start, _ := StageSelect.Band()
	e := &Event{Stage: StageSelect}
	if got := e.Percent(); got != start {
		t.Errorf("uncounted event = %d%%, want %d%%", got, start)
	}
}

// The bar must never go backwards. Events are appended in order, but a stage
// reporting its start after a slower one reported its finish would otherwise
// drag it back.
func TestRunPercentNeverRetreats(t *testing.T) {
	events := []*Event{
		event(StageNormalize, 804, 804),
		event(StageResolve, 189, 804),
		event(StageCandidates, 500, 804),
		event(StageAIEnhance, 300, 615),
		// Out of order, and lower than what came before it.
		event(StageResolve, 0, 804),
	}

	got := RunPercent(events)
	want := event(StageAIEnhance, 300, 615).Percent()
	if got != want {
		t.Errorf("run percent = %d, want %d — a late low event dragged the bar back", got, want)
	}
}

// A count above the total cannot push the bar past its stage. The AI tier
// reports per batch and the last batch is usually short, so an off-by-one there
// must not overshoot into the next stage's band.
func TestPercentIsClampedToItsBand(t *testing.T) {
	_, end := StageCandidates.Band()
	if got := event(StageCandidates, 900, 804).Percent(); got != end {
		t.Errorf("overshoot = %d%%, want %d%%", got, end)
	}
}

func TestCurrentStageIsTheLatestOne(t *testing.T) {
	events := []*Event{
		event(StageResolve, 1, 1),
		event(StageAIEnhance, 1, 2),
	}
	if got := CurrentStage(events); got != StageAIEnhance {
		t.Errorf("current stage = %q, want %q", got, StageAIEnhance)
	}
	if got := CurrentStage(nil); got != "" {
		t.Errorf("no events = %q, want empty", got)
	}
}
