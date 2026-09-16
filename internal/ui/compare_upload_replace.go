package ui

import (
	"context"
	"fmt"
	"mime/multipart"
	"sort"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// The two halves of "a bulk import replaces what came before it".
//
// trimToPlanQuota decides what a batch may contain; activeCompareFileIDs
// records what the batch is about to replace. The replacement itself is
// deferred until the new files have been staged successfully — see
// compare.Service.ReplacePreviousFiles.

// trimToPlanQuota keeps the first maxAllowed files of a batch and names the
// rest, instead of refusing the whole upload.
//
// It used to refuse: over the plan's list count, the upload came back with
// "archive or delete your old lists to proceed", which asked the vendor to do
// by hand the very thing the upload does for them — the previous lists are
// replaced automatically, so they were never what filled the quota. The client
// already trims the selection this way before sending it (compare_tool.js);
// this is the authoritative version of the same rule, for a client that did
// not run it.
//
// maxAllowed of 0 means unlimited.
func trimToPlanQuota(headers []*multipart.FileHeader, maxAllowed int, lang string) ([]*multipart.FileHeader, []string) {
	if maxAllowed <= 0 || len(headers) <= maxAllowed {
		return headers, nil
	}
	skipped := make([]string, 0, len(headers)-maxAllowed)
	for _, h := range headers[maxAllowed:] {
		skipped = append(skipped, h.Filename+" ("+
			fmt.Sprintf(i18n.T(lang, "compare.upload.quota_skipped"), maxAllowed)+")")
	}
	return headers[:maxAllowed], skipped
}

// trimToBatchLimit caps a single bulk upload batch to maxBatchLimit (e.g. 100 files).
func trimToBatchLimit(headers []*multipart.FileHeader, maxBatchLimit int, lang string) ([]*multipart.FileHeader, []string) {
	if maxBatchLimit <= 0 || len(headers) <= maxBatchLimit {
		return headers, nil
	}
	skipped := make([]string, 0, len(headers)-maxBatchLimit)
	for _, h := range headers[maxBatchLimit:] {
		skipped = append(skipped, h.Filename+" (تجاوز الحد الأقصى للدفعة: 100 ملف)")
	}
	return headers[:maxBatchLimit], skipped
}

// supersededFileIDs picks the fewest existing files that must be archived to
// fit `incoming` new ones inside the plan's limit — oldest first.
//
// Free space is used before anything is archived. A vendor holding 8 files on a
// 10-file plan who uploads 2 keeps all 8; the same vendor uploading 3 loses
// exactly one — the oldest — and not the other seven. Replacing the whole
// workspace on every upload is what this replaced: it archived eight files to
// make room for two, and the vendor lost six lists they had not replaced.
//
// Returns nothing when the plan is unlimited (maxAllowed 0), when everything
// fits, or when the snapshot could not be read: archiving on a guess is how a
// vendor loses files they never replaced.
func (h *UIHandler) supersededFileIDs(ctx context.Context, userID int64, orgID *int64, incoming, maxAllowed int) []int64 {
	if h.compareSvc == nil || incoming <= 0 || maxAllowed <= 0 {
		return nil
	}
	files, err := h.compareSvc.ListFiles(ctx, userID, orgID, nil)
	if err != nil {
		h.log.ErrorContext(ctx, "could not snapshot active compare files before upload", "error", err)
		return nil
	}

	overflow := len(files) + incoming - maxAllowed
	if overflow <= 0 {
		return nil // there is room; nothing is superseded.
	}
	if overflow > len(files) {
		overflow = len(files)
	}

	// Sort newest-first by CreatedAt so the oldest are guaranteed to be at the end.
	sort.Slice(files, func(i, j int) bool {
		if files[i] == nil || files[j] == nil {
			return false
		}
		return files[i].CreatedAt.After(files[j].CreatedAt)
	})

	// ListFiles is newest-first, so the oldest — the ones a new upload should
	// displace first — are at the end.
	ids := make([]int64, 0, overflow)
	for i := len(files) - 1; i >= 0 && len(ids) < overflow; i-- {
		if files[i] != nil {
			ids = append(ids, files[i].ID)
		}
	}
	return ids
}

// activeCompareFileIDs lists the compare files the vendor holds right now.
func (h *UIHandler) activeCompareFileIDs(ctx context.Context, userID int64, orgID *int64) []int64 {
	if h.compareSvc == nil {
		return nil
	}
	files, err := h.compareSvc.ListFiles(ctx, userID, orgID, nil)
	if err != nil {
		h.log.ErrorContext(ctx, "could not snapshot active compare files before upload", "error", err)
		return nil
	}
	ids := make([]int64, 0, len(files))
	for _, f := range files {
		if f != nil {
			ids = append(ids, f.ID)
		}
	}
	return ids
}
