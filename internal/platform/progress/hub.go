// Package progress carries an import's live progress from wherever the work is
// happening to whoever is watching it.
//
// Every import tool on the platform already agrees on what progress IS —
// importrun.Progress, computed by shared/importprogress — and every one of them
// delivered it the same wasteful way: the browser asked twice a second, and the
// two endpoints that did stream (the smart order, the ingest wizard) ran a
// database query once a second FOR EVERY OPEN CONNECTION. Ten pharmacies
// watching ten imports is twenty queries a second on a machine with a
// twenty-connection pool, spent almost entirely re-reading a number that had not
// changed.
//
// The shape here is the other way round: nothing is asked, work announces
// itself.
//
//   - The worker publishes a snapshot when a number actually changes.
//   - A Hub fans that out to whoever is subscribed, in memory, for nothing.
//   - Where the publisher is a different process from the watcher — which is the
//     normal case, because imports run in cmd/worker and screens are served by
//     cmd/server — Redis pub/sub carries it across, one subscription per server
//     process rather than one per viewer.
//
// The database is still the truth. A subscriber reads the run once when it
// connects and on a slow safety tick, so a dropped message delays a bar by a few
// seconds rather than stranding it. What it no longer does is ask over and over
// for an answer nobody has changed.
package progress

import (
	"sync"
	"time"
)

// Snapshot is one moment of an import's progress.
//
// It is deliberately the shape import-progress.js already consumes, so the
// stream and the poll are interchangeable and the browser cannot tell which one
// it got.
type Snapshot struct {
	// ID is the run's public identifier, and the key everything is routed by.
	ID string `json:"id"`
	// Percent is the whole-run figure from shared/importprogress: one bar,
	// 0..100, never reaching 100 before the terminal state says so.
	Percent int `json:"percent"`
	// Message is the human phase caption, in the viewer's language.
	Message string `json:"message"`
	// Current and Total are rows processed and rows expected. Total is 0 while
	// the count is not yet known, which the bar renders as no counter rather
	// than as "0 / 0".
	Current int `json:"current"`
	Total   int `json:"total"`
	// State is the importrun state, for a client that wants more than a number.
	State string `json:"state"`
	// Done is written once, by the terminal state, and never inferred from the
	// arithmetic reaching the end of the last band.
	Done bool `json:"done"`
	// Error is set when the run failed, so the bar can stop where it got to and
	// say why instead of silently freezing.
	Error string `json:"error,omitempty"`
	// At is when the snapshot was taken. Subscribers use it to drop a message
	// that arrives out of order behind a newer one.
	At time.Time `json:"at"`
}

// Terminal reports whether nothing further will be published for this run.
func (s Snapshot) Terminal() bool { return s.Done || s.Error != "" }

// IsFailure reports whether the run stopped because something went wrong, which
// is the one terminal case that must NOT be shown as a completed bar.
func (s Snapshot) IsFailure() bool { return s.Error != "" }

// subscriberBuffer is how many snapshots a slow subscriber may fall behind
// before it starts losing the middle ones.
//
// Four. A progress bar only ever needs the LATEST value, so a subscriber that
// cannot keep up should skip rather than block the publisher — but a buffer of
// one drops the terminal snapshot behind a routine one, and the terminal
// snapshot is the only one that must arrive. Four is enough that the send never
// blocks in practice, and dropReplacingOldest guarantees the rest.
const subscriberBuffer = 4

// retention is how long the last snapshot of a finished run is kept.
//
// A browser that reconnects a second after the import ended must be told it
// ended, not told nothing. Two minutes covers a reload, a lost network and a
// phone waking up; past that the run is read from the database like any other
// piece of history.
const retention = 2 * time.Minute

// Hub fans snapshots out to the connections watching them.
//
// One per process. It holds no goroutines of its own: publishing walks the
// subscriber list, and expiry is folded into publish and subscribe so an idle
// hub costs nothing at all.
type Hub struct {
	mu     sync.Mutex
	rooms  map[string]*room
	nextID uint64
}

type room struct {
	last     Snapshot
	lastAt   time.Time
	watchers map[uint64]chan Snapshot
}

// NewHub returns an empty hub.
func NewHub() *Hub { return &Hub{rooms: make(map[string]*room)} }

// Publish records a snapshot and delivers it to everyone watching that run.
//
// It never blocks: a subscriber whose buffer is full has its oldest pending
// snapshot discarded to make room for this one, because the newest figure is
// the only one a progress bar wants and a publisher stalled on a slow browser
// would hold up the import itself.
func (h *Hub) Publish(s Snapshot) {
	if h == nil || s.ID == "" {
		return
	}
	if s.At.IsZero() {
		s.At = time.Now()
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.expireLocked()

	r := h.rooms[s.ID]
	if r == nil {
		r = &room{watchers: make(map[uint64]chan Snapshot)}
		h.rooms[s.ID] = r
	}
	// Out-of-order delivery is normal once messages cross a process boundary.
	// A snapshot older than the one already held says nothing new.
	if !r.lastAt.IsZero() && s.At.Before(r.last.At) {
		return
	}
	r.last, r.lastAt = s, time.Now()

	for _, ch := range r.watchers {
		dropReplacingOldest(ch, s)
	}
}

// Subscribe returns a channel of snapshots for one run, and the last snapshot
// already known, if any.
//
// The last snapshot is returned rather than pushed so the caller can render
// immediately and decide for itself whether the run is already finished — a
// browser that opens a stream for a completed import should be told so and
// disconnect, not hold a connection waiting for an event that will never come.
//
// The returned cancel must be called. It is idempotent.
func (h *Hub) Subscribe(id string) (ch <-chan Snapshot, last Snapshot, known bool, cancel func()) {
	if h == nil || id == "" {
		closed := make(chan Snapshot)
		close(closed)
		return closed, Snapshot{}, false, func() {}
	}

	h.mu.Lock()
	h.expireLocked()

	r := h.rooms[id]
	if r == nil {
		r = &room{watchers: make(map[uint64]chan Snapshot)}
		h.rooms[id] = r
	}
	h.nextID++
	key := h.nextID
	out := make(chan Snapshot, subscriberBuffer)
	r.watchers[key] = out
	last, known = r.last, !r.lastAt.IsZero()
	h.mu.Unlock()

	var once sync.Once
	return out, last, known, func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if rr := h.rooms[id]; rr != nil {
				if c, ok := rr.watchers[key]; ok {
					delete(rr.watchers, key)
					close(c)
				}
				if len(rr.watchers) == 0 && rr.lastAt.IsZero() {
					delete(h.rooms, id)
				}
			}
		})
	}
}

// Watchers reports how many connections are following a run, which is what a
// publisher consults before doing work only a watcher would see.
func (h *Hub) Watchers(id string) int {
	if h == nil {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if r := h.rooms[id]; r != nil {
		return len(r.watchers)
	}
	return 0
}

// expireLocked drops rooms whose last snapshot is older than retention and
// which nobody is watching. Called from publish and subscribe so the hub needs
// no sweeper goroutine: a room can only be created by one of those two, so a
// hub that is not being used cannot be growing.
func (h *Hub) expireLocked() {
	if len(h.rooms) == 0 {
		return
	}
	cutoff := time.Now().Add(-retention)
	for id, r := range h.rooms {
		if len(r.watchers) == 0 && !r.lastAt.IsZero() && r.lastAt.Before(cutoff) {
			delete(h.rooms, id)
		}
	}
}

// dropReplacingOldest sends without blocking, discarding the oldest pending
// snapshot when the buffer is full.
//
// Bounded rather than a retry loop: exactly one discard, then exactly one more
// attempt. A loop here could spin against a reader draining the same channel,
// and it would be spinning while holding the hub's lock — which is the one
// place on this path that must never be slow, because it is the import's own
// goroutine calling it.
func dropReplacingOldest(ch chan Snapshot, s Snapshot) {
	select {
	case ch <- s:
		return
	default:
	}
	select {
	case <-ch: // the oldest pending value; a bar only ever wants the newest
	default:
	}
	select {
	case ch <- s:
	default:
	}
}
