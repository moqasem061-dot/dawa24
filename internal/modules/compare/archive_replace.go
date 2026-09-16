package compare

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Replacing a vendor's previous price lists with the batch that supersedes them.
//
// A bulk import is a replacement, not an addition: the pharmacy uploads this
// round of supplier sheets meaning them to stand in for the last one. The tool
// used to make the vendor do that by hand — the upload refused the batch and
// told them to go and archive the old lists first — which is busywork for a
// decision the upload has already made.
//
// What it must not become is a replacement that happens on the way IN.
// Archiving before the new workbooks have been read costs a vendor their whole
// working set the moment one of those workbooks turns out to be unreadable, and
// leaves them with nothing to compare while they sort the file out.
//
// So the old generation is archived only once the new one has been staged
// successfully. Staging is detached from the request (see upload_background.go)
// and takes seconds to minutes, so the wait for it is detached too: the
// supervisor below outlives the request that started it, exactly as the staging
// it watches does.
const (
	// replaceSettleInterval is how often the supervisor re-reads the batch.
	replaceSettleInterval = 3 * time.Second

	// replaceSettleTimeout bounds the whole wait. Longer than stageTimeout, so
	// a staging pass that runs all the way to its own limit is still observed
	// finishing rather than being abandoned a moment before it does.
	replaceSettleTimeout = stageTimeout + 2*time.Minute
)

// ReplacePreviousFiles archives previousIDs once every file in incomingIDs has
// finished staging and at least one of them came out ready to compare.
//
// It returns immediately. Nothing is archived when the whole incoming batch
// fails: a vendor whose upload did not work keeps what they had.
//
// previousIDs must already be scoped to the caller's tenant — they come from
// the same ListFiles read that decides what the vendor currently holds — which
// is why the archive below passes no owner filter of its own.
func (s *Service) ReplacePreviousFiles(ctx context.Context, previousIDs, incomingIDs []int64, reason string) {
	if s == nil || len(previousIDs) == 0 || len(incomingIDs) == 0 {
		return
	}
	if reason == "" {
		reason = i18n.T("ar", "compare.upload.replace_reason")
	}

	// The request that uploaded the batch is answered long before this
	// finishes, so the supervisor keeps the request's values — the tenant
	// binding row-level security reads, above all — and loses its cancellation.
	runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), replaceSettleTimeout)

	go func() {
		defer cancel()
		defer func() {
			if p := recover(); p != nil {
				s.log.ErrorContext(runCtx, "compare replacement supervisor panicked", "panic", p)
			}
		}()

		if !s.waitForStagedBatch(runCtx, incomingIDs) {
			s.log.WarnContext(runCtx, "compare replacement skipped: no incoming file became ready",
				"incoming", incomingIDs, "previous", previousIDs)
			return
		}

		archived, err := s.repo.BulkArchiveFiles(runCtx, previousIDs, nil, reason)
		if err != nil {
			s.log.ErrorContext(runCtx, "could not archive superseded compare files",
				"error", err, "previous", previousIDs)
			return
		}
		s.log.InfoContext(runCtx, "archived superseded compare files",
			"count", archived, "incoming", len(incomingIDs))
	}()
}

// waitForStagedBatch blocks until no file in ids is still processing, and
// reports whether at least one of them ended up ready.
//
// A file that has vanished counts as finished rather than holding the wait
// open; a file that failed counts as finished but not as ready, so a batch
// where every workbook failed leaves the previous generation alone.
func (s *Service) waitForStagedBatch(ctx context.Context, ids []int64) bool {
	ticker := time.NewTicker(replaceSettleInterval)
	defer ticker.Stop()

	for {
		statuses, err := s.StagingProgress(ctx, ids)
		if err == nil {
			pending := false
			ready := false
			for _, st := range statuses {
				if !st.Done {
					pending = true
					continue
				}
				if st.Status == string(FileReady) {
					ready = true
				}
			}
			if !pending {
				return ready
			}
		}

		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}
