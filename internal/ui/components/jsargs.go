package components

import "encoding/json"

// JSArgs encodes the arguments of a data-on-* call for its data-args
// attribute (see static/js/actions.js):
//
//	<button data-on-click="openRenameModal" data-args={ components.JSArgs(f.ID, f.Name) }>
//
// Values travel as JSON inside an HTML-escaped attribute, so a name holding a
// quote stays data. Building the call with fmt.Sprintf("f('%s')", name) put
// that name straight into JavaScript.
func JSArgs(args ...any) string {
	b, err := json.Marshal(args)
	if err != nil {
		return "[]"
	}
	return string(b)
}
