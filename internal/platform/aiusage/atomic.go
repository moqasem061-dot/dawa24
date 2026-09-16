package aiusage

import "sync/atomic"

// atomic64 is a counter, named so the drop counter reads as what it is at every
// use site rather than as a bare atomic.Int64 whose meaning lives elsewhere.
type atomic64 struct{ v atomic.Int64 }

func (a *atomic64) add(n int64) int64 { return a.v.Add(n) }
func (a *atomic64) load() int64       { return a.v.Load() }
