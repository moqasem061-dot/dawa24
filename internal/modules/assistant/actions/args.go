package actions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
)

// Args are an action's arguments after validation: refs are verified row ids,
// numbers are range-checked, strings are bounded. Executors read them through
// the typed getters and never see what the model wrote.
type Args map[string]any

// ID returns a verified reference id, or zero.
func (a Args) ID(name string) int64 { return a.Int(name) }

// Int returns an integer argument, or zero.
func (a Args) Int(name string) int64 {
	switch v := a[name].(type) {
	case int64:
		return v
	case json.Number:
		n, _ := v.Int64()
		return n
	}
	return 0
}

// Str returns a string or enum argument, or "".
func (a Args) Str(name string) string {
	s, _ := a[name].(string)
	return s
}

// Decimal returns a number argument as exact decimal text, or "".
func (a Args) Decimal(name string) string {
	switch v := a[name].(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	}
	return ""
}

// Bool returns a boolean argument.
func (a Args) Bool(name string) bool {
	b, _ := a[name].(bool)
	return b
}

// Has reports whether an optional argument was supplied.
func (a Args) Has(name string) bool {
	_, ok := a[name]
	return ok
}

// ErrInvalid wraps an argument the model can correct.
type ErrInvalid struct{ Message string }

func (e *ErrInvalid) Error() string { return e.Message }

// ErrBadRef is a reference that did not verify. It says nothing more.
var ErrBadRef = fmt.Errorf("invalid reference")

func invalid(format string, a ...any) error { return &ErrInvalid{Message: fmt.Sprintf(format, a...)} }

var decimal = regexp.MustCompile(`^-?\d{1,12}(\.\d{1,4})?$`)

// Decode validates raw model arguments against a definition. Unknown fields are
// refused, required ones enforced, refs verified through resolve.
func Decode(def Definition, raw json.RawMessage, resolve func(handles.Kind, string) (int64, error)) (Args, error) {
	fields := map[string]json.RawMessage{}
	if trimmed := bytes.TrimSpace(raw); len(trimmed) > 0 && string(trimmed) != "null" {
		dec := json.NewDecoder(bytes.NewReader(trimmed))
		if err := dec.Decode(&fields); err != nil {
			return nil, invalid("args must be a JSON object")
		}
		if dec.More() {
			return nil, invalid("args contain trailing data")
		}
	}
	declared := map[string]Param{}
	for _, p := range def.Params {
		declared[p.Name] = p
	}
	for name := range fields {
		if _, ok := declared[name]; !ok {
			return nil, invalid("%s takes no argument %q", def.Name, name)
		}
	}

	out := Args{}
	for _, p := range def.Params {
		v, present := fields[p.Name]
		if !present || string(bytes.TrimSpace(v)) == "null" {
			if p.Required {
				return nil, invalid("%s requires %q", def.Name, p.Name)
			}
			continue
		}
		val, err := decodeParam(p, v, resolve)
		if err != nil {
			return nil, err
		}
		out[p.Name] = val
	}
	return out, nil
}

func decodeParam(p Param, raw json.RawMessage, resolve func(handles.Kind, string) (int64, error)) (any, error) {
	switch p.Type {
	case ParamRef:
		var token string
		if err := json.Unmarshal(raw, &token); err != nil || strings.TrimSpace(token) == "" {
			return nil, invalid("%q needs a ref from an earlier result", p.Name)
		}
		id, err := resolve(p.RefKind, strings.TrimSpace(token))
		if err != nil || id <= 0 {
			return nil, ErrBadRef
		}
		return id, nil
	case ParamInt:
		var n json.Number
		if err := json.Unmarshal(raw, &n); err != nil {
			return nil, invalid("%q needs a whole number", p.Name)
		}
		i, err := n.Int64()
		if err != nil || float64(i) < p.Min || float64(i) > p.Max {
			return nil, invalid("%q must be a whole number between %v and %v", p.Name, p.Min, p.Max)
		}
		return i, nil
	case ParamNumber:
		var n json.Number
		if err := json.Unmarshal(raw, &n); err != nil {
			var s string
			if json.Unmarshal(raw, &s) != nil {
				return nil, invalid("%q needs a number", p.Name)
			}
			n = json.Number(strings.TrimSpace(s))
		}
		f, err := n.Float64()
		if err != nil || !decimal.MatchString(n.String()) || math.IsNaN(f) || f < p.Min || f > p.Max {
			return nil, invalid("%q must be a number between %v and %v", p.Name, p.Min, p.Max)
		}
		return n.String(), nil
	case ParamString:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, invalid("%q needs text", p.Name)
		}
		s = strings.TrimSpace(s)
		limit := p.MaxLen
		if limit <= 0 {
			limit = 500
		}
		if utf8.RuneCountInString(s) > limit {
			return nil, invalid("%q is longer than %d characters", p.Name, limit)
		}
		return s, nil
	case ParamEnum:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, invalid("%q needs one of %s", p.Name, strings.Join(p.Values, ", "))
		}
		for _, allowed := range p.Values {
			if s == allowed {
				return s, nil
			}
		}
		return nil, invalid("%q must be one of %s", p.Name, strings.Join(p.Values, ", "))
	case ParamBool:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return nil, invalid("%q needs true or false", p.Name)
		}
		return b, nil
	}
	return nil, invalid("%q has an unsupported type", p.Name)
}

// Signature renders a definition compactly for the model: name(arg: type, ...).
func Signature(def Definition) string {
	parts := make([]string, 0, len(def.Params))
	for _, p := range def.Params {
		t := string(p.Type)
		switch p.Type {
		case ParamRef:
			t = "ref"
		case ParamEnum:
			t = strings.Join(p.Values, "|")
		case ParamInt, ParamNumber:
			t = fmt.Sprintf("%s %v-%v", p.Type, p.Min, p.Max)
		}
		opt := ""
		if !p.Required {
			opt = "?"
		}
		parts = append(parts, fmt.Sprintf("%s%s: %s", p.Name, opt, t))
	}
	return def.Name + "(" + strings.Join(parts, ", ") + ")"
}

// Marshal stores validated arguments. Ids stay server-side: this JSON lives in
// the pending action row and is never sent to the model or the browser.
func (a Args) Marshal() ([]byte, error) {
	keys := make([]string, 0, len(a))
	for k := range a {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ordered := make(map[string]any, len(a))
	for _, k := range keys {
		ordered[k] = a[k]
	}
	return json.Marshal(ordered)
}

// UnmarshalArgs restores stored arguments with exact numbers.
func UnmarshalArgs(data []byte) (Args, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var out Args
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}
