package main

import (
	"io"
	"log/slog"
	"testing"

	"github.com/riverqueue/river"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// TestERPWorkersRegistered proves the enabled path registers both workers
// (chunk + cleanup) and returns the daily cleanup periodic job. The disabled
// path is main.go's own if-gate on cfg.ERP.Enabled: without it a disabled
// deployment would still register an erpsync consumer (rule 13).
func TestERPWorkersRegistered(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	workers := river.NewWorkers()
	count, periodics := registerERPWorkers(workers, database.New(), nil, logger)
	if count != 2 {
		t.Errorf("registered workers = %d, want 2 (chunk + cleanup)", count)
	}
	if len(periodics) != 1 {
		t.Errorf("periodic jobs = %d, want 1 (the daily cleanup)", len(periodics))
	}
}
