package components

import "regexp"

// closeResets matches an AlpineClose that resets a flag, optionally behind a
// guard: "open = false", "!busy && (open = false)".
var closeResets = regexp.MustCompile(`^\s*(?:!?[A-Za-z_$][\w$.]*\s*&&\s*\(\s*)?([A-Za-z_$][\w$]*)\s*=\s*false\s*\)?\s*$`)

// modalOpenFlag is the Alpine flag that opens a Modal: AlpineOpen, or the
// flag its AlpineClose resets. Pages that only set AlpineClose still flip that
// flag to true to open the dialog, and nothing used to act on it.
func modalOpenFlag(props ModalProps) string {
	if props.AlpineOpen != "" {
		return props.AlpineOpen
	}
	if m := closeResets.FindStringSubmatch(props.AlpineClose); m != nil {
		return m[1]
	}
	return ""
}
