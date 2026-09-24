package queue_test

import (
	"testing"
	"time"

	"github.com/riverqueue/river"

	"github.com/moqasem061-dot/dawa24/internal/platform/queue"
)

func TestQueueConstants(t *testing.T) {
	if queue.DefaultJobTimeout != 30*time.Minute {
		t.Errorf("DefaultJobTimeout = %v; want 30m", queue.DefaultJobTimeout)
	}

	workers := river.NewWorkers()
	if workers == nil {
		t.Fatal("river.NewWorkers returned nil")
	}
}
