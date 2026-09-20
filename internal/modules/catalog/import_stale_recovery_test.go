package catalog_test

import (
	"context"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

func TestSessionProgress_ReapsStaleProcessingSession(t *testing.T) {
	store := newMemoryStore()
	svc, _ := newImportService(t, store)
	ctx := context.Background()

	session, _, err := svc.AnalyzeImport(ctx, []byte(serviceFixture), "list.csv", 0)
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}

	// Simulate a session left in 'processing' in the database from a previous run or server restart.
	session.Status = catalog.SessionProcessing
	session.UpdatedAt = time.Now().Add(-2 * time.Minute)
	session.Progress = catalog.ImportProgress{
		Phase:   catalog.ImportPhaseMatching,
		Current: 0,
		Total:   1000,
	}
	if err := store.UpdateImportSession(ctx, session); err != nil {
		t.Fatalf("update session failed: %v", err)
	}

	// SessionProgress must detect the orphaned session, transition it to SessionFailed, and return failed progress.
	prog, updatedSession, err := svc.SessionProgress(ctx, session.PublicID)
	if err != nil {
		t.Fatalf("SessionProgress failed: %v", err)
	}

	if updatedSession.Status != catalog.SessionFailed {
		t.Errorf("expected session status %v, got %v", catalog.SessionFailed, updatedSession.Status)
	}
	if prog.Phase != catalog.ImportPhaseFailed {
		t.Errorf("expected progress phase %v, got %v", catalog.ImportPhaseFailed, prog.Phase)
	}
	if updatedSession.IsProcessing() {
		t.Errorf("expected IsProcessing to be false for reaped session")
	}
	if updatedSession.ErrorMessage == "" {
		t.Errorf("expected helpful ErrorMessage on reaped session")
	}
}
