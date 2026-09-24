package catalog_test

import (
	"context"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
)

func TestCancelImport_WhileProcessing(t *testing.T) {
	store := newMemoryStore()
	svc, _ := newImportService(t, store)
	ctx := context.Background()

	session, _, err := svc.AnalyzeImport(ctx, []byte(serviceFixture), "list.csv", 0)
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}

	// Start asynchronous preparation
	if err := svc.PrepareImportAsync(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd, Options: catalog.DefaultImportOptions(),
	}); err != nil {
		t.Fatalf("prepare async failed: %v", err)
	}

	// Cancel while processing must succeed without error
	if err := svc.CancelImport(ctx, session.PublicID); err != nil {
		t.Fatalf("cancel import while processing failed: %v", err)
	}

	// Verify session is marked cancelled
	got, err := svc.GetImportSession(ctx, session.PublicID)
	if err != nil {
		t.Fatalf("get session failed: %v", err)
	}
	if got.Status != catalog.SessionCancelled {
		t.Errorf("session status = %v, want %v", got.Status, catalog.SessionCancelled)
	}
}
