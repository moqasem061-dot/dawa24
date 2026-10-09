package ui

// Called under the store lock. Polling must own its metadata instead of reading
// fields the background worker can change after the read lock is released.
// Source grids and matching explanations are immutable after publication;
// editable item fields are copied so row edits cannot race with rendering.
func savingSessionSnapshot(session *SavingImportSession) *SavingImportSession {
	snapshot := *session
	snapshot.Items = make([]*StagedSavingItem, len(session.Items))
	for i, item := range session.Items {
		if item != nil {
			copy := *item
			snapshot.Items[i] = &copy
		}
	}
	return &snapshot
}
