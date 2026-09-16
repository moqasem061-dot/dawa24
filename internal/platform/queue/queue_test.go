package queue_test

import (
	"testing"
	"time"

	"github.com/riverqueue/river"

	"github.com/muhiya/dawa24-store/internal/platform/queue"
)

type dummyArgs struct{}

func (dummyArgs) Kind() string { return "dummy.job" }

func TestQueueConstants(t *testing.T) {
	if queue.DefaultJobTimeout != 30*time.Minute {
		t.Errorf("DefaultJobTimeout = %v; want 30m", queue.DefaultJobTimeout)
	}

	workers := river.NewWorkers()
	if workers == nil {
		t.Fatal("river.NewWorkers returned nil")
	}
}
