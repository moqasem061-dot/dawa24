package components

import "strconv"

// itoa is a local shorthand, used inside templ expressions where importing
// strconv into every template would add noise for a single call.
func itoa(n int) string { return strconv.Itoa(n) }
