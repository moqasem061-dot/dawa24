package provider

import (
	"fmt"
	"sort"
	"sync"
)

// DefaultProvider is the provider of the first integration (Orgasoft). This
// file is one of the only places outside the provider's own package allowed to
// name the provider (trap 28).
const DefaultProvider = "orgasoft"

var (
	registryMu sync.RWMutex
	registry   = map[string]Provider{}
)

// Register adds a provider under its Name(). A provider package calls this
// from its own init; the registry cannot import the provider packages without
// an import cycle, so the linkage is the blank import in provider/all (T093).
// Panics on a duplicate: two systems claiming one name is a wiring mistake a
// restart cannot paper over.
func Register(p Provider) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if p == nil {
		panic("provider: Register(nil)")
	}
	if _, dup := registry[p.Name()]; dup {
		panic(fmt.Sprintf("provider: duplicate registration for %q", p.Name()))
	}
	registry[p.Name()] = p
}

// Get returns the registered provider by name.
func Get(name string) (Provider, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	p, ok := registry[name]
	return p, ok
}

// Names returns the sorted registered provider names. The list is whatever the
// binary registered at startup, which is everything linked through provider/all.
func Names() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
