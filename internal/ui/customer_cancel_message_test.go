package ui

import (
	"strings"
	"testing"
)

// B4: a cancellation that is refused for a reason other than a refusal shows the
// error text in the message, not the raw format verb.
func TestCancelRefusalFailureFormatsTheErrorDetail(t *testing.T) {
	f := cancelRefusalFailure("en", 7, nil, "database is busy")
	if strings.Contains(f.Message, "%s") {
		t.Fatalf("message still has a format verb: %q", f.Message)
	}
	if !strings.Contains(f.Message, "database is busy") {
		t.Fatalf("message %q does not carry the error detail", f.Message)
	}
}
