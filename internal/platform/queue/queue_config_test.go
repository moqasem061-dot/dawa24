package queue_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/config"
)

// queueRef finds `Queue: "name"` in a river.InsertOpts literal.
var queueRef = regexp.MustCompile(`Queue:\s*"([a-z_]+)"`)

// Every queue a job is inserted into must be a queue the worker polls.
//
// River claims jobs ONLY from the queues named in river.Config.Queues. A job
// inserted into a queue that is not listed there is not rejected and does not
// error — it is written to river_job, left in state `available`, and never
// looked at again. The worker that would have handled it is registered and
// idle, so nothing in the logs says anything is wrong either.
//
// That is exactly what had happened to "smartorder": queue/jobs.go inserts
// SmartOrderRunArgs into it, cmd/worker registers a worker for it, and the
// config listed imports, ai, notifications, projections and maintenance. The
// web process runs smart orders inline, which is the only reason it had not
// surfaced — deploying cmd/worker as the thing that actually runs them would
// have produced a queue of runs that never start and never fail.
//
// A source scan rather than a registry walk, because the failure is a
// MISMATCH between two lists that are written in different packages, and only
// reading both can catch it.
func TestEveryInsertedQueueIsConfigured(t *testing.T) {
	// config.Load validates the deployment's required settings, none of which
	// this test needs — it reads a map of queue names, and connects to nothing.
	// Supplying placeholders keeps the gate RUNNING rather than skipping, which
	// is the whole point: a gate that skips in CI protects nothing.
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/db?sslmode=disable")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("SESSION_SECRET", strings.Repeat("x", 48))

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if len(cfg.Worker.Queues) == 0 {
		t.Fatal("no worker queues configured at all")
	}

	root := filepath.Join("..", "..", "..")
	found := map[string][]string{}

	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // an unreadable directory is not this test's business
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", "node_modules", "tmp", ".gocache", ".gotmp":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		for _, m := range queueRef.FindAllStringSubmatch(string(src), -1) {
			name := m[1]
			found[name] = append(found[name], path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(found) == 0 {
		t.Fatal("found no queue references at all; the pattern has drifted from the code")
	}

	for name, sites := range found {
		if _, ok := cfg.Worker.Queues[name]; !ok {
			t.Errorf("jobs are inserted into queue %q, which no worker polls.\n"+
				"  Add it to config.Worker.Queues, or the jobs will sit in river_job for ever.\n"+
				"  Inserted at: %s", name, strings.Join(sites, ", "))
		}
	}
}
