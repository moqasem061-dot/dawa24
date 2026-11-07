package test

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// deadcodeTool is the one pinned version. `make check-deadcode` runs this test,
// so the version and the ceiling live in exactly one place.
const deadcodeTool = "golang.org/x/tools/cmd/deadcode@v0.50.0"

// deadcodeCeiling is the measured count of unreachable functions (12 with the
// pinned tool). Lower it when dead code is removed; never raise it to make a
// new dead function pass.
// The 12 left are kept on purpose: the gateway Disabled fallback methods and the
// AvailabilityFunc adapter (interface obligations), the queue Client methods
// (the matching Enqueuer), the ingest test store method, and authctx.SameOrgOrForbidden
// (named in the admin guard test).
const deadcodeCeiling = 12

// TestDeadcodeRatchet enforces that unreachable dead code does not exceed the
// ceiling. When the tool cannot run it fails in CI and skips on a developer
// machine, so a broken tool can never pass the gate silently.
func TestDeadcodeRatchet(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping deadcode ratchet in short mode")
	}

	cmd := exec.CommandContext(t.Context(), "go", "run", deadcodeTool, "-test", "./...")
	cmd.Dir = ".."
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		stderr := ""
		if errors.As(err, &exitErr) {
			stderr = strings.TrimSpace(string(exitErr.Stderr))
		}
		if os.Getenv("CI") == "true" {
			t.Fatalf("deadcode did not run (pinned %s): %v\n%s", deadcodeTool, err, stderr)
		}
		t.Skipf("deadcode unavailable locally (needs network on first run): %v", err)
	}

	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "unreachable func:") {
			n++
		}
	}
	if n > deadcodeCeiling {
		t.Errorf("deadcode count %d exceeded ratchet ceiling %d", n, deadcodeCeiling)
	}
}
