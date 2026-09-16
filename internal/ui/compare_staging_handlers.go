package ui

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Telling the compare screen when a freshly uploaded batch is readable.
//
// The upload now returns as soon as the files are recorded, and their rows are
// read by a goroutine that outlives the request. That is what stops a ten-file
// batch from holding the browser open for minutes — and it means the screen
// receiving the redirect can no longer assume the columns have been detected.
//
// The column-mapping wizard in particular must not open on a file nobody has
// parsed: it would offer a mapping for a spreadsheet with no header row and no
// preview. It asks here first, and shows the shared progress dialog until every
// file in the batch has left `processing`.

// maxStagingBatch bounds how many ids one request may ask about.
//
// The upload quota is ten files on the largest plan, so anything past a couple
// of dozen is not a real batch — it is somebody probing the endpoint with a
// long list to make it do work on their behalf.
const maxStagingBatch = 32

// CompareStagingStatus reports whether an uploaded batch has finished parsing.
//
// Route: GET /compare/files/staging?ids=1,2,3
//
// Ownership is checked per file through the same helper every other compare
// route uses, so this cannot be used to learn that somebody else's file exists.
func (h *UIHandler) CompareStagingStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	actor, ok := authctx.From(ctx)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   i18n.T(lang, "common.unauthorized"),
		})
		return
	}
	if h.compareSvc == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false})
		return
	}

	ids := parseStagingIDs(r.URL.Query().Get("ids"))
	if len(ids) == 0 {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true, "ready": true, "files": []any{}, "percent": 100,
		})
		return
	}

	// Ownership first, every file, before anything is reported. A batch
	// containing one id the caller does not own is refused whole rather than
	// answered for the rest — a partial answer is a way of asking "does this id
	// exist" and getting a reply.
	// A platform admin uploads temp warehouses ON BEHALF of an organisation, so
	// the file is not theirs by the ownership rule every other compare route
	// uses. They may already read and edit these rows through the admin screens;
	// refusing them a readiness check would only mean the admin panel could not
	// tell when its own upload had finished.
	isAdmin := actor.IsPlatformAdmin()
	for _, id := range ids {
		file, err := h.compareSvc.GetFile(ctx, id)
		if err != nil || file == nil {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false})
			return
		}
		if !isAdmin && !h.checkFileOwnership(actor, file) {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false})
			return
		}
	}

	statuses, err := h.compareSvc.StagingProgress(ctx, ids)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false})
		return
	}

	done := 0
	for _, st := range statuses {
		if st.Done {
			done++
		}
	}
	// Percent of FILES finished, which is the only honest figure available: the
	// rows of a file being read are not counted until it is done, and inventing
	// a within-file percentage would be a bar that moves for no reason.
	percent := 100
	if len(statuses) > 0 {
		percent = done * 100 / len(statuses)
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"ready":   done == len(statuses),
		"files":   statuses,
		"done":    done,
		"total":   len(statuses),
		"percent": percent,
	})
}

// parseStagingIDs reads the comma-separated id list, ignoring anything that is
// not a positive integer.
func parseStagingIDs(raw string) []int64 {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]int64, 0, len(parts))
	seen := make(map[int64]struct{}, len(parts))
	for _, p := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64)
		if err != nil || id <= 0 {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
		if len(out) >= maxStagingBatch {
			break
		}
	}
	return out
}
