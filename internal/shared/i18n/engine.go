package i18n

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// KeyEntry represents one localization key with default/custom text and metadata.
type KeyEntry struct {
	Key         string `json:"key"`
	Namespace   string `json:"namespace"`
	TextAR      string `json:"text_ar"`
	TextEN      string `json:"text_en"`
	Description string `json:"description,omitempty"`
	IsCustom    bool   `json:"is_custom"`
}

type engine struct {
	mu        sync.RWMutex
	defaults  map[string]KeyEntry
	overrides map[string]Text
}

var globalEngine = &engine{
	defaults:  make(map[string]KeyEntry),
	overrides: make(map[string]Text),
}

func init() {
	loadCatalogDefaults(globalEngine)
}

// T translates a key into the given language with optional format arguments.
func T(lang any, key string, args ...any) string {
	var l Lang
	switch v := lang.(type) {
	case Lang:
		l = v
	case string:
		l = ParseLang(v)
	default:
		l = Default
	}
	return globalEngine.translate(l, key, args...)
}

// TDefault translates a key into the default language (Arabic).
func TDefault(key string, args ...any) string {
	return globalEngine.translate(Default, key, args...)
}

// Lookup returns the translated text for key without formatting and whether it exists.
func Lookup(lang Lang, key string) (string, bool) {
	globalEngine.mu.RLock()
	if ov, ok := globalEngine.overrides[key]; ok {
		globalEngine.mu.RUnlock()
		val := ov.Get(lang)
		if val != "" {
			return val, true
		}
	} else {
		globalEngine.mu.RUnlock()
	}

	globalEngine.mu.RLock()
	def, ok := globalEngine.defaults[key]
	globalEngine.mu.RUnlock()
	if ok {
		if lang == EN && def.TextEN != "" {
			return def.TextEN, true
		} else if def.TextAR != "" {
			return def.TextAR, true
		} else if def.TextEN != "" {
			return def.TextEN, true
		}
	}
	return "", false
}

func (e *engine) translate(lang Lang, key string, args ...any) string {
	e.mu.RLock()
	// 1. Check custom overrides
	if ov, ok := e.overrides[key]; ok {
		e.mu.RUnlock()
		val := ov.Get(lang)
		if val != "" {
			return formatStr(val, args...)
		}
	} else {
		e.mu.RUnlock()
	}

	// 2. Check compiled defaults
	e.mu.RLock()
	def, ok := e.defaults[key]
	e.mu.RUnlock()
	if ok {
		var val string
		if lang == EN && def.TextEN != "" {
			val = def.TextEN
		} else if def.TextAR != "" {
			val = def.TextAR
		} else if def.TextEN != "" {
			val = def.TextEN
		}
		if val != "" {
			return formatStr(val, args...)
		}
	}

	// 3. Fallback to key itself if not found
	if len(args) > 0 {
		return formatStr(key, args...)
	}
	return key
}

func formatStr(tmpl string, args ...any) string {
	if len(args) == 0 {
		return tmpl
	}
	if strings.Contains(tmpl, "%") {
		return fmt.Sprintf(tmpl, args...)
	}
	for i, arg := range args {
		placeholder := fmt.Sprintf("{%d}", i)
		tmpl = strings.ReplaceAll(tmpl, placeholder, fmt.Sprint(arg))
	}
	return tmpl
}

// RegisterOverrides bulk sets runtime overrides from database.
func RegisterOverrides(m map[string]Text) {
	globalEngine.mu.Lock()
	defer globalEngine.mu.Unlock()
	for k, v := range m {
		if !v.IsEmpty() {
			globalEngine.overrides[k] = v
		} else {
			delete(globalEngine.overrides, k)
		}
	}
}

// SetOverride sets or updates a single key override.
func SetOverride(key string, text Text) {
	globalEngine.mu.Lock()
	defer globalEngine.mu.Unlock()
	if text.IsEmpty() {
		delete(globalEngine.overrides, key)
	} else {
		globalEngine.overrides[key] = text
	}
}

// RemoveOverride clears a key override and reverts it to default.
func RemoveOverride(key string) {
	globalEngine.mu.Lock()
	defer globalEngine.mu.Unlock()
	delete(globalEngine.overrides, key)
}

// GetAllKeyEntries returns all registered keys with current effective values.
func GetAllKeyEntries() []KeyEntry {
	globalEngine.mu.RLock()
	defer globalEngine.mu.RUnlock()

	keys := make([]string, 0, len(globalEngine.defaults))
	for k := range globalEngine.defaults {
		keys = append(keys, k)
	}
	for k := range globalEngine.overrides {
		if _, exists := globalEngine.defaults[k]; !exists {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	out := make([]KeyEntry, 0, len(keys))
	for _, k := range keys {
		entry, exists := globalEngine.defaults[k]
		if !exists {
			parts := strings.SplitN(k, ".", 2)
			ns := "common"
			if len(parts) > 1 {
				ns = parts[0]
			}
			entry = KeyEntry{
				Key:       k,
				Namespace: ns,
			}
		}
		if ov, ok := globalEngine.overrides[k]; ok {
			if ar := ov.Get(AR); ar != "" {
				entry.TextAR = ar
			}
			if en := ov.Get(EN); en != "" {
				entry.TextEN = en
			}
			entry.IsCustom = true
		}
		out = append(out, entry)
	}
	return out
}

// GetNamespaces returns a sorted unique list of all namespaces.
func GetNamespaces() []string {
	globalEngine.mu.RLock()
	defer globalEngine.mu.RUnlock()

	nsMap := make(map[string]bool)
	for _, v := range globalEngine.defaults {
		if v.Namespace != "" {
			nsMap[v.Namespace] = true
		}
	}
	var list []string
	for ns := range nsMap {
		list = append(list, ns)
	}
	sort.Strings(list)
	return list
}

// Translate resolves a key whose name is chosen at runtime.
//
// T is treated as a printf-style function by go vet -- it forwards its
// arguments to a formatter -- so passing a non-constant key trips the printf
// check even when there are no arguments at all. Callers that select a key from
// a fixed set at runtime use this instead: same lookup, no format arguments,
// and no reason for vet to inspect it.
func Translate(lang any, key string) string {
	var l Lang
	switch v := lang.(type) {
	case Lang:
		l = v
	case string:
		l = ParseLang(v)
	default:
		l = Default
	}
	return globalEngine.resolve(l, key)
}

// resolve looks a key up without the variadic formatting path.
//
// It is the same lookup translate performs -- override, then compiled default,
// then the key itself -- minus the argument substitution. Going through
// translate would put a non-constant string in a variadic call that go vet
// reads as a printf format, which is the whole reason Translate exists.
func (e *engine) resolve(lang Lang, key string) string {
	e.mu.RLock()
	ov, hasOverride := e.overrides[key]
	e.mu.RUnlock()
	if hasOverride {
		if val := ov.Get(lang); val != "" {
			return val
		}
	}

	e.mu.RLock()
	def, ok := e.defaults[key]
	e.mu.RUnlock()
	if ok {
		if lang == EN && def.TextEN != "" {
			return def.TextEN
		}
		if def.TextAR != "" {
			return def.TextAR
		}
		if def.TextEN != "" {
			return def.TextEN
		}
	}
	return key
}

// TKey translates a key that is computed rather than written literally.
//
// It exists because T is variadic on format arguments, so `go vet` reads
// `T(lang, "prefix."+value)` as a non-constant format string and fails the
// build. That diagnostic is right about T and wrong about this use: a key
// assembled from a status or a type is not a format string and has no
// arguments. TKey takes no arguments, so there is nothing to mis-format.
//
// A key with no entry renders as itself, which is a visible, greppable
// placeholder rather than an empty cell nobody notices.
func TKey(lang any, key string) string {
	var l Lang
	switch v := lang.(type) {
	case Lang:
		l = v
	case string:
		l = ParseLang(v)
	default:
		l = Default
	}
	if text, ok := Lookup(l, key); ok && text != "" {
		return text
	}
	return key
}
