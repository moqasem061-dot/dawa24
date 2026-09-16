package ingest

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Running the staging pass without holding the vendor's browser open.
//
// Staging parses the whole file, scores every row against a catalogue of up to
// a hundred and fifty thousand products, and then asks a model about whatever
// the arithmetic could not settle. On a thirty-thousand-row price list that is
// minutes of work, and every second of it used to happen inside the POST that
// submitted the settings form.
//
// Three things followed from that, and all three were reported as bugs:
//
//   - The browser waited on a blank page and eventually gave up. Any proxy in
//     front of the application has its own timeout, and it is measured in
//     tens of seconds, not minutes.
//   - A vendor who navigated away lost the run. The context belonged to the
//     request, so cancelling the request cancelled the matching — halfway
//     through, with rows already staged and no way to tell which.
//   - Retrying looked like the fix and made it worse: a second POST started a
//     second full pass, including a second full AI bill, over the same file.
//
// The phase machinery to do better already existed and was only used by the
// commit path: PhaseProcessing has a screen of its own that polls, and
// ingest.import_progress persists how far a run has reached. Staging now uses
// both, so the vendor may close the tab, come back tomorrow, and find either
// the review screen or a progress bar that has kept moving without them.

// stageTimeout bounds one detached staging pass.
//
// Generous because the work is genuinely long — a large file against a large
// catalogue, plus the AI stage — and because the alternative to finishing is a
// vendor doing the matching by hand. A file that cannot be staged inside this
// is one whose column mapping is wrong, and the vendor is told so rather than
// left watching a bar that has stopped.
const stageTimeout = 45 * time.Minute

// StageInBackground starts the staging pass and returns immediately.
//
// The returned session is already in PhaseProcessing, so the caller can redirect
// straight to the screen that polls. An error means the run could not be
// started at all; once it has started, its outcome is reported through the
// session's phase rather than to the caller, who is by then a request that has
// long since been answered.
func (s *Service) StageInBackground(ctx context.Context, session *Session) error {
	if session == nil {
		return apperr.Validation("import.missing", i18n.TDefault("w4_mod.w4str_202_202"), nil)
	}
	if !s.runs.claim(session.PublicID) {
		return apperr.Conflict("import.running",
			i18n.TDefault("w4_mod.w4str_190_190"))
	}

	// Publish the phase before the goroutine starts. A vendor who is redirected
	// faster than the scheduler gets round to the run must still land on the
	// progress screen rather than on the settings form they just submitted.
	session.Phase = PhaseProcessing
	if err := s.imports.SaveDraft(ctx, session); err != nil {
		s.runs.release(session.PublicID)
		return err
	}
	if err := s.imports.Progress(ctx, session.ID, 1, i18n.TDefault("w4_mod.s_391_391")); err != nil {
		s.log.WarnContext(ctx, "import progress not recorded", "import", session.PublicID, "error", err)
	}

	// The run outlives the request that asked for it, so it gets a context
	// carrying the request's values — the tenant binding row-level security
	// reads, most of all — but not its cancellation. context.WithoutCancel is
	// exactly this: keep who you are, lose when you must stop.
	runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), stageTimeout)
	runCtx = database.WithTenant(runCtx, session.OrganizationID)
	var actorUserID int64
	if session.CreatedBy != nil {
		actorUserID = *session.CreatedBy
	}
	runCtx = authctx.WithActor(runCtx, authctx.Actor{
		UserID:         actorUserID,
		OrganizationID: session.OrganizationID,
		OrgID:          session.OrganizationID,
		Role:           "vendor",
	})

	// The goroutine works on its own copy. The caller still holds the session it
	// passed in and a handler is free to read it while rendering; two goroutines
	// writing and reading the same struct is a data race whatever the fields
	// happen to be.
	running := *session

	go func() {
		defer cancel()
		defer s.runs.release(running.PublicID)
		// A panic in a detached goroutine takes the whole web process with it,
		// which would end every other vendor's import too. Contained here and
		// reported as a failed run, so one bad file cannot be an outage.
		defer func() {
			if p := recover(); p != nil {
				s.log.ErrorContext(runCtx, "vendor import staging panicked",
					"import", running.PublicID, "panic", p)
				s.failStaging(runCtx, &running,
					i18n.TDefault("w4_mod.w4str_203_203"))
			}
		}()
		s.runStaging(runCtx, &running)
	}()
	return nil
}

// runStaging performs the pass and records how it ended.
func (s *Service) runStaging(ctx context.Context, session *Session) {
	err := s.StageImport(ctx, session)
	if err == nil {
		s.log.InfoContext(ctx, "vendor import staged",
			"import", session.PublicID, "rows", session.TotalRows,
			"matched", session.MatchedRows, "review", session.ReviewRows,
			"unmatched", session.UnmatchedRows)
		return
	}

	// A failed run must not leave the session in PhaseProcessing: the screen
	// would poll a bar that has stopped, forever, and the vendor has no way to
	// tell that from a slow file.
	s.log.ErrorContext(ctx, "vendor import staging failed",
		"import", session.PublicID, "error", err)

	s.failStaging(ctx, session, s.stagingFailureMessage(err))
}

// failStaging records that a run ended badly, so the screen stops polling.
//
// Through FinishStaging for the same reason the success path is: the session is
// in 'processing', and SaveDraft refuses that phase — so the previous version
// of this could not record a failure either, and a failed run was
// indistinguishable from a slow one for as long as anyone cared to watch.
func (s *Service) failStaging(ctx context.Context, session *Session, message string) {
	session.Phase = PhaseFailed
	session.ErrorMessage = message
	if err := s.imports.FinishStaging(ctx, session); err != nil {
		s.log.ErrorContext(ctx, "could not record staging failure",
			"import", session.PublicID, "error", err)
	}
}

// stagingFailureMessage turns an error into something a vendor can act on.
//
// Never the raw error: it names internal tables and column indexes, and the
// vendor's only useful next step is almost always the same one — check the
// column mapping and upload again.
func (s *Service) stagingFailureMessage(err error) string {
	if appErr, ok := apperr.As(err); ok && appErr != nil {
		if msg := appErr.LocalizedMsg("ar"); msg != "" {
			return msg
		}
	}
	return i18n.TDefault("w4_mod.w4str_204_204")
}
