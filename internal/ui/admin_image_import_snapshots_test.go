package ui

import "testing"

func TestAdminImageImportProgressSnapshots(t *testing.T) {
	store := &AdminImageImportSessionStore{sessions: make(map[string]*AdminImageImportSession)}
	created := store.NewSession(AdminImageImportSession{
		OrgID: 1, UserID: 10, Filename: "images.csv", TotalRows: 1,
		Headers: []string{"sku", "url"}, RawDataRows: [][]string{{"sku-1", "https://example.com/image.png"}},
	})
	before, ok := store.GetSession(created.ID)
	if !ok {
		t.Fatal("session not created")
	}
	listed := store.ListSessions()
	if !store.SetMapping(created.ID, "barcode", 1, 2) {
		t.Fatal("mapping update failed")
	}
	store.mu.Lock()
	stored := store.sessions[created.ID]
	stored.Phase = AdminImagePhaseCompleted
	stored.Progress = 100
	stored.Rows = []*AdminImageImportRow{{SKU: "sku-1", Status: "success"}}
	store.mu.Unlock()
	if before.Phase != AdminImagePhaseMapping || before.Progress != 0 || len(before.Rows) != 0 {
		t.Fatal("a previously returned progress snapshot changed")
	}
	if listed[0].Phase != AdminImagePhaseMapping || listed[0].IdentifierType != "sku" {
		t.Fatal("the session listing exposed mutable progress")
	}
	current, ok := store.GetSession(created.ID)
	if !ok || current.Progress != 100 || current.IdentifierType != "barcode" || current.DetectedSKUCol != 1 || current.DetectedURLCol != 2 {
		t.Fatal("locked mapping/progress updates were not persisted")
	}
	current.Rows[0] = nil
	if fresh, _ := store.GetSession(created.ID); fresh.Rows[0] == nil {
		t.Fatal("snapshot row slice aliases the store")
	}
}
