package ingest

import (
	"context"
	"strconv"

	"github.com/muhiya/dawa24-store/internal/platform/progress"
)

// Announcing a vendor import's progress as it is written.
//
// The vendor's screen polled for a number every second and a half for the whole
// run. Progress is written through one method and the run's end through three,
// so wrapping the store makes all four announce themselves and leaves the
// import code untouched.
//
// See internal/modules/catalog/import_progress_notify.go for the same decorator
// over the administrative import, and internal/platform/progress for the hub.

// ProgressKey is the hub key one import session is published under.
//
// The INTERNAL id, not the public one. Progress(ctx, id, …) is given only the
// internal id, and the store has no way to resolve one to the other — Get takes
// a public id and there is no reverse. Caching the pair as it went past would
// have worked in the web process and silently not in the worker, where the run
// begins in a different process from the one that created it.
//
// So the id that is always available is the one everything is keyed on, and the
// stream endpoint — which starts from the public id in the URL and loads the
// session anyway — derives the same key from the row it already has.
func ProgressKey(sessionID int64) string {
	return "ingest:" + strconv.FormatInt(sessionID, 10)
}

// notifyingImportStore publishes every progress and outcome write.
//
// It embeds the interface rather than listing its methods, so one added later
// still compiles and simply does not publish.
type notifyingImportStore struct {
	ImportStore
	pub *progress.Publisher
}

// WithProgressNotifications returns a store that publishes as it writes.
//
// A nil publisher returns the store unchanged, so a deployment without the live
// channel keeps working on the JSON poll beside the stream.
func WithProgressNotifications(store ImportStore, pub *progress.Publisher) ImportStore {
	if store == nil || pub == nil {
		return store
	}
	return &notifyingImportStore{ImportStore: store, pub: pub}
}

// Progress records how far a run has reached, and says so.
func (n *notifyingImportStore) Progress(ctx context.Context, id int64, percent int, note string) error {
	if err := n.ImportStore.Progress(ctx, id, percent, note); err != nil {
		return err
	}
	// Never 100 from here. The bar reaches the end because a terminal write
	// said so, not because the arithmetic of a staging pass got there while it
	// was still writing rows.
	if percent >= 100 {
		percent = 99
	}
	n.pub.Publish(ctx, progress.Snapshot{
		ID:      ProgressKey(id),
		Percent: percent,
		Message: note,
		State:   string(PhaseProcessing),
	})
	return nil
}

// FinishStaging is the end of the phase the vendor is watching.
//
// A staging run ends in 'review', not in a terminal state, so IsDone-style
// reasoning misses it entirely — and a bar waiting for "done" sat at
// ninety-nine per cent while the review screen was already built and waiting.
func (n *notifyingImportStore) FinishStaging(ctx context.Context, s *Session) error {
	if err := n.ImportStore.FinishStaging(ctx, s); err != nil {
		return err
	}
	n.publishSession(ctx, s)
	return nil
}

// Finish records a completed run.
func (n *notifyingImportStore) Finish(ctx context.Context, s *Session) error {
	if err := n.ImportStore.Finish(ctx, s); err != nil {
		return err
	}
	n.publishSession(ctx, s)
	return nil
}

// Fail stops the bar where it got to and says why, rather than freezing it.
func (n *notifyingImportStore) Fail(ctx context.Context, id int64, message string) error {
	if err := n.ImportStore.Fail(ctx, id, message); err != nil {
		return err
	}
	n.pub.Publish(ctx, progress.Snapshot{
		ID:      ProgressKey(id),
		Message: message,
		State:   string(PhaseFailed),
		Done:    true,
		Error:   message,
	})
	return nil
}

// Cancel ends the watch as decisively as a failure does.
func (n *notifyingImportStore) Cancel(ctx context.Context, id int64) error {
	if err := n.ImportStore.Cancel(ctx, id); err != nil {
		return err
	}
	n.pub.Publish(ctx, progress.Snapshot{
		ID:    ProgressKey(id),
		State: string(PhaseCancelled),
		Done:  true,
	})
	return nil
}

func (n *notifyingImportStore) publishSession(ctx context.Context, s *Session) {
	if s == nil || s.ID == 0 {
		return
	}
	n.pub.Publish(ctx, progress.Snapshot{
		ID:      ProgressKey(s.ID),
		Percent: 100,
		Message: s.ProgressNote,
		State:   string(s.Phase),
		Done:    true,
		Error:   s.ErrorMessage,
	})
}
