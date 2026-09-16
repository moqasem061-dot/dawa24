package catalog

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/platform/progress"
)

// Announcing an administrative import's progress as it is written.
//
// The screen watching this import asked the server for a number twice a second
// for as long as the run lasted — a thirty-eight-thousand-row catalogue takes
// minutes, so that is several hundred requests, each one a full round trip
// through routing, authentication and the session store, to re-read a figure
// that had usually not moved.
//
// Progress is written in exactly one place (SaveImportProgress) and the run's
// end in one more (UpdateImportSession), so wrapping the store is enough to
// make every one of those writes announce itself. Nothing in the import code
// changes, and code written later cannot forget to publish.
//
// See internal/platform/progress for the hub, and internal/platform/importrun
// for the same decorator over the durable-run repository.

// notifyingImportStore publishes every progress and status write.
//
// It embeds the interface rather than listing its thirty methods, so a method
// added later still compiles here and simply does not publish — the safe
// direction for a decorator to fail in.
type notifyingImportStore struct {
	ImportSessionStore
	pub *progress.Publisher
}

// WithProgressNotifications returns a store that publishes as it writes.
//
// A nil publisher returns the store unchanged: a deployment without the live
// channel is not a deployment with a broken one, and the JSON poll beside the
// stream keeps every bar working either way.
func WithProgressNotifications(store ImportSessionStore, pub *progress.Publisher) ImportSessionStore {
	if store == nil || pub == nil {
		return store
	}
	return &notifyingImportStore{ImportSessionStore: store, pub: pub}
}

// SaveImportProgress records the run's position and says so.
func (n *notifyingImportStore) SaveImportProgress(ctx context.Context, publicID string, p ImportProgress) error {
	if err := n.ImportSessionStore.SaveImportProgress(ctx, publicID, p); err != nil {
		return err
	}
	n.pub.Publish(ctx, progress.Snapshot{
		ID:      publicID,
		Percent: p.Percent(),
		Message: p.Message,
		Current: p.Current,
		Total:   p.Total,
		State:   string(p.Phase),
		Done:    p.Phase == ImportPhaseDone || p.Phase == ImportPhaseFailed,
		Error:   importFailureReason(p),
		At:      p.UpdatedAt,
	})
	return nil
}

// UpdateImportSession carries the status changes SaveImportProgress never sees.
//
// A run leaving 'processing' — for review, for a failure, for a commit — is the
// event the watching screen is actually waiting for, and it is written here
// rather than through the progress path. Without this the bar sat at
// ninety-nine per cent until the safety poll noticed the run had finished.
func (n *notifyingImportStore) UpdateImportSession(ctx context.Context, s *ImportSession, fromStatuses ...SessionStatus) error {
	if err := n.ImportSessionStore.UpdateImportSession(ctx, s, fromStatuses...); err != nil {
		return err
	}
	if s == nil || s.PublicID == "" || s.IsProcessing() {
		return nil
	}
	n.pub.Publish(ctx, progress.Snapshot{
		ID:      s.PublicID,
		Percent: s.Progress.Percent(),
		Message: s.Progress.Message,
		Current: s.Progress.Current,
		Total:   s.Progress.Total,
		State:   string(s.Status),
		Done:    true,
		Error:   importFailureReason(s.Progress),
	})
	return nil
}

// failureMessage is the reason a run stopped, and empty for one that did not.
//
// The message field carries the phase label on a healthy run and the reason on
// a failed one, so it may only be reported as an error when the phase says the
// run failed — otherwise every ordinary progress tick would arrive at the
// browser looking like a failure and stop the bar.
func importFailureReason(p ImportProgress) string {
	if p.Phase == ImportPhaseFailed {
		return p.Message
	}
	return ""
}
