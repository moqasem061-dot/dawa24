package matchflow

import "time"

// DispatchShare is the proportion of total run deadline within which new
// attempts may be dispatched (85%). The remaining 15% is reserved for
// in-flight requests to complete and terminal state publication.
const DispatchShare = 0.85

// MaxAttemptTimeout is the upper bound for any single AI attempt (15 s).
const MaxAttemptTimeout = 15 * time.Second

// TerminalPublishSlack is the grace period added to deadline for terminal state publishing.
const TerminalPublishSlack = 2 * time.Second

// DeadlineClass determines the overall time budget for an AI run based on
// the number of eligible distinct questions.
//
//	<= 100  questions:  30 s
//	<= 1000 questions:  90 s
//	> 1000  questions: 180 s
func DeadlineClass(questions int) time.Duration {
	switch {
	case questions <= 100:
		return 30 * time.Second
	case questions <= 1000:
		return 90 * time.Second
	default:
		return 180 * time.Second
	}
}

// AttemptDeadline calculates the timeout for a single attempt given the
// remaining run duration: min(15s, remaining - 1s). If remaining <= 1s, it returns 0.
func AttemptDeadline(remaining time.Duration) time.Duration {
	budget := remaining - time.Second
	if budget <= 0 {
		return 0
	}
	if budget < MaxAttemptTimeout {
		return budget
	}
	return MaxAttemptTimeout
}
