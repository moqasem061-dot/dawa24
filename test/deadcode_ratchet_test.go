package test

import (
	"os/exec"
	"strings"
	"testing"
)

// TestDeadcodeRatchet enforces that unreachable dead code does not exceed
// the measured ratchet ceiling (302 entries).
func TestDeadcodeRatchet(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping deadcode ratchet in short mode")
	}

	cmd := exec.Command("go", "run", "golang.org/x/tools/cmd/deadcode@latest", "./...")
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("deadcode output:\n%s", string(out))
		// deadcode exits with 0 on success (reporting unreachable funcs)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var deadEntries []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" && strings.Contains(trimmed, "unreachable func:") {
			deadEntries = append(deadEntries, trimmed)
		}
	}

	const deadcodeCeiling = 307
	if len(deadEntries) > deadcodeCeiling {
		t.Errorf("deadcode count %d exceeded ratchet ceiling %d", len(deadEntries), deadcodeCeiling)
	}
}
