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

// AlpineCall renders an x-data or directive value that calls a registered
// Alpine component (dawaData) or component method with JSON arguments:
//
//	x-data={ components.AlpineCall("dawaLinePager", len(lines)) }
//	@click={ components.AlpineCall("openEdit", item.ID, item.Name) }
//
// The @alpinejs/csp parser reads JSON literals, and JSON keeps every value
// data; fmt.Sprintf("f('%s')", name) let a quote in name rewrite the call.
// A json.RawMessage argument is passed through as an object literal.
func AlpineCall(name string, args ...any) string {
	b, err := json.Marshal(args)
	if err != nil {
		return name + "()"
	}
	return name + "(" + string(b[1:len(b)-1]) + ")"
}
