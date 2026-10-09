package ui

import (
	"sync"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

func TestSavingSessionReadsStayStableDuringProgressAndEdits(t *testing.T) {
	store := &SavingImportSessionStore{sessions: make(map[string]*SavingImportSession)}
	id := store.NewSession(1, 2, "supplier.csv", 1).ID
	store.CompleteProcessing(id, []*StagedSavingItem{{Index: 1, Price: money.FromMinor(100), Quantity: 1}}, 0, 1, 1, money.FromMinor(100))
	before, ok := store.GetSession(id, 1)
	if !ok {
		t.Fatal("missing session")
	}
	admin, ok := store.GetSessionForAdmin(id)
	if !ok {
		t.Fatal("missing admin session")
	}
	history := store.ListSessions(1)
	price := money.FromMinor(200)
	if err := store.UpdateStagedItem(id, 1, 1, "Changed", &price, nil, nil); err != nil {
		t.Fatal(err)
	}
	store.UpdateProgress(id, 20, "Changed", 0)
	for _, snapshot := range []*SavingImportSession{before, admin, history[0]} {
		if snapshot.Progress != 100 || snapshot.Items[0].Price.Minor() != 100 {
			t.Fatalf("a subsequent update changed a returned snapshot: %+v", snapshot)
		}
	}
	var updates sync.WaitGroup
	updates.Add(1)
	go func() {
		defer updates.Done()
		for i := 0; i < 100; i++ {
			store.UpdateProgress(id, i, "Processing", i)
		}
	}()
	for i := 0; i < 100; i++ {
		snapshot, ok := store.GetSession(id, 1)
		if !ok {
			t.Fatal("session disappeared")
		}
		_ = snapshot.Progress
	}
	updates.Wait()
}
