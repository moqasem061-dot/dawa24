package ui

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// The upload handler parses the file and then has to stop and ask which column
// is which. It used to build that session with NewSession, which marks a
// session `processing` for the async run, and only reassigned Phase afterwards.
//
// The wizard tests Status before Phase, so the session rendered the matching
// screen instead of the mapping screen: step 2 was skipped entirely. And since
// the goroutine that moves the bar is started by the mapping form the buyer
// never reached, the bar polled a session frozen at Progress 5 — which the
// client-side drift floors to 4% — with nothing behind it, forever.
//
// These tests pin both halves of that.

func TestUploadedSessionAwaitsMappingRatherThanProcessing(t *testing.T) {
	store := &SavingImportSessionStore{sessions: map[string]*SavingImportSession{}}

	sess := store.NewMappingSession(
		42, 7, "list.xlsx",
		[]string{"الصنف", "الكمية"},
		[][]string{{"بانادول", "5"}},
		[][]string{{"بانادول", "5"}, {"كونجستال", "3"}},
		SavingDetectedCols{NameCol: 0, SKUCol: -1, QtyCol: 1, PriceCol: -1, ProductIDCol: -1},
	)

	if sess.Status == SessionStateProcessing {
		t.Fatal("a session waiting for its columns must not be marked processing; " +
			"the wizard renders the matching screen for that status and skips step 2")
	}
	if sess.Status != SessionStateUploaded {
		t.Errorf("status: got %q, want %q", sess.Status, SessionStateUploaded)
	}
	if sess.Phase != SavingPhaseMapping {
		t.Errorf("phase: got %q, want %q", sess.Phase, SavingPhaseMapping)
	}
	if sess.TotalRows != 2 {
		t.Errorf("total rows: got %d, want 2", sess.TotalRows)
	}
	if len(sess.RawDataRows) != 2 || len(sess.Headers) != 2 {
		t.Errorf("the parsed file must be carried on the session: headers=%d rows=%d",
			len(sess.Headers), len(sess.RawDataRows))
	}
	if sess.Progress != 0 {
		t.Errorf("a session nobody is processing must report no progress, got %d", sess.Progress)
	}
}

// The wizard chooses its screen from these two predicates. A session on the
// mapping phase must reach the mapping screen, and a session the async run is
// actually working on must reach the progress screen.
func TestWizardRoutesMappingAndProcessingToDifferentScreens(t *testing.T) {
	cases := []struct {
		name           string
		status         SessionState
		phase          SavingImportPhase
		wantProcessing bool
		wantMapping    bool
		wantStep       pages.Step
	}{
		{"awaiting columns", SessionStateUploaded, SavingPhaseMapping, false, true, pages.StepColumns},
		{"stale processing status on a mapping session", SessionStateProcessing, SavingPhaseMapping, false, true, pages.StepColumns},
		{"async run in flight", SessionStateProcessing, SavingPhaseReview, true, false, pages.StepReview},
		{"results ready", SessionStateReady, SavingPhaseReview, false, false, pages.StepReview},
		{"committed", SessionStateCommitted, SavingPhaseCompleted, false, false, pages.StepResults},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sess := &SavingImportSession{ID: "s", Status: tc.status, Phase: tc.phase}

			if got := sess.IsProcessing(); got != tc.wantProcessing {
				t.Errorf("IsProcessing: got %v, want %v", got, tc.wantProcessing)
			}
			if got := sess.AwaitingMapping(); got != tc.wantMapping {
				t.Errorf("AwaitingMapping: got %v, want %v", got, tc.wantMapping)
			}
			view := pages.SavingImportView{Session: sess}
			if got := view.WizardStep(); got != tc.wantStep {
				t.Errorf("WizardStep: got %d, want %d", got, tc.wantStep)
			}
		})
	}
}

// A nil session is the upload screen, not a panic.
func TestNilSessionIsNeitherProcessingNorMapping(t *testing.T) {
	var sess *SavingImportSession
	if sess.IsProcessing() || sess.AwaitingMapping() {
		t.Fatal("a nil session must report neither state")
	}
	if got := (pages.SavingImportView{}).WizardStep(); got != pages.StepFile {
		t.Fatalf("WizardStep with no session: got %d, want %d", got, pages.StepFile)
	}
}
