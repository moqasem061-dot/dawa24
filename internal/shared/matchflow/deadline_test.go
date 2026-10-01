package matchflow_test

import (
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

func TestDeadlineClass(t *testing.T) {
	tests := []struct {
		name      string
		questions int
		want      time.Duration
	}{
		{"zero", 0, 30 * time.Second},
		{"50 questions", 50, 30 * time.Second},
		{"100 questions", 100, 30 * time.Second},
		{"101 questions", 101, 90 * time.Second},
		{"500 questions", 500, 90 * time.Second},
		{"1000 questions", 1000, 90 * time.Second},
		{"1001 questions", 1001, 180 * time.Second},
		{"5000 questions", 5000, 180 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchflow.DeadlineClass(tt.questions)
			if got != tt.want {
				t.Errorf("DeadlineClass(%d) = %v, want %v", tt.questions, got, tt.want)
			}
		})
	}
}

func TestAttemptDeadline(t *testing.T) {
	tests := []struct {
		name      string
		remaining time.Duration
		want      time.Duration
	}{
		{"100s remaining", 100 * time.Second, 15 * time.Second},
		{"20s remaining", 20 * time.Second, 15 * time.Second},
		{"16s remaining", 16 * time.Second, 15 * time.Second},
		{"10s remaining", 10 * time.Second, 9 * time.Second},
		{"2s remaining", 2 * time.Second, 1 * time.Second},
		{"1s remaining", 1 * time.Second, 0},
		{"0s remaining", 0, 0},
		{"negative remaining", -5 * time.Second, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchflow.AttemptDeadline(tt.remaining)
			if got != tt.want {
				t.Errorf("AttemptDeadline(%v) = %v, want %v", tt.remaining, got, tt.want)
			}
		})
	}
}
