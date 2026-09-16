package ingest

// Running the commit without holding the vendor's browser open.
//
// The commit writes a variant and a warehouse balance for every row the vendor
// confirmed. On a nine-thousand-row price list that is tens of thousands of
// statements, and all of it used to happen inside the POST behind the "save"
// button — so a large import raced the proxy's timeout, and a vendor who
// navigated away cancelled the request context halfway through, leaving their
// catalogue half written with nothing on screen to say which half.
//
// Staging already solved this: PhaseProcessing has a screen that polls,
// ingest.import_progress persists how far a run has reached, and
// context.WithoutCancel keeps the tenant binding while dropping the request's
// cancellation. The commit uses the same machinery, so the vendor may close the
// tab and come back to either the results screen or a bar that kept moving.

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// commitTimeout bounds one detached commit. Generous, because the alternative
// to finishing is a vendor with half a catalogue.
const commitTimeout = 45 * time.Minute

// CommitInBackground starts the commit and returns immediately.
//
// The returned session is already in PhaseProcessing, so the caller can
// redirect straight to the screen that polls. An error means the commit could
// not be started at all; once started, its outcome is reported through the
// session's phase rather than to a request that has long since been answered.
func (s *Service) CommitInBackground(ctx context.Context, publicID string) (*Session, error) {
	session, err := s.prepareCommit(ctx, publicID)
	if err != nil {
		return nil, err
	}
	if !s.runs.claim(publicID) {
		return nil, apperr.Conflict("import.running", i18n.TDefault("w4_mod.w4str_192_192"))
	}

	// The phase is published before the goroutine starts, so a vendor
	// redirected faster than the scheduler gets round to the run still lands on
	// the progress screen rather than back on the review table they just
	// submitted. BeginCommit, not Begin: Begin clears the staged rows, which on
	// a commit are the very thing being written.
	if err := s.imports.BeginCommit(ctx, session.ID); err != nil {
		s.runs.release(publicID)
		return nil, err
	}
	session.Phase = PhaseProcessing
	s.note(ctx, session, 1, i18n.TDefault("ingest.commit.running"))

	runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), commitTimeout)
	running := *session

	go func() {
		defer cancel()
		defer s.runs.release(running.PublicID)
		defer func() {
			if p := recover(); p != nil {
				s.log.ErrorContext(runCtx, "vendor import commit panicked",
					"import", running.PublicID, "panic", p)
				s.failCommit(runCtx, &running, i18n.TDefault("ingest.commit.failed"))
			}
		}()
		if _, err := s.commit(runCtx, &running); err != nil {
			s.log.ErrorContext(runCtx, "vendor import commit failed",
				"import", running.PublicID, "error", err)
			s.failCommit(runCtx, &running, importFailureMessage(err))
		}
	}()
	return session, nil
}

// failCommit records that a commit ended badly, so the screen stops polling.
//
// Through Fail rather than Finish: a run that stopped partway has written some
// of the vendor's rows and not others, and reporting it as a completed import
// with a low count would read as "your file only had that many valid rows".
func (s *Service) failCommit(ctx context.Context, session *Session, message string) {
	if message == "" {
		message = i18n.TDefault("ingest.commit.failed")
	}
	if err := s.imports.Fail(ctx, session.ID, message); err != nil {
		s.log.ErrorContext(ctx, "could not record commit failure",
			"import", session.PublicID, "error", err)
	}
}
