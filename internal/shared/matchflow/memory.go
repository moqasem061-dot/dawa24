package matchflow

// The decision cache, as a port every import tool can hold.
//
// The cache itself is one table — catalog.match_decisions, keyed by
// DecisionKey — and it has always been shared in the sense that any tool
// writing to it fills it for the others. It was not shared in the sense that
// mattered: only two of the four import paths ever read or wrote it. The
// saving-list import paid for every question on every upload, and the
// administrator's master-catalogue import did too, while the vendor import and
// the smart order were answering the same questions for free.
//
// That is a port shape rather than an argument about caching. Each of the two
// tools that had one declared its own vocabulary for the same rows, so a third
// tool could only join in by inventing a third. This is the vocabulary, in the
// package that already holds the question, the ceilings and the key.

import "context"

// Remembered is one answer as the cache stores it.
//
// A nil ProductID is "none of these", which is a real answer and worth
// remembering: it is what stops the next upload of the same price list paying
// to be told the same thing.
type Remembered struct {
	Key             string
	NormName        string
	ChosenProductID *int64
	Confidence      float64
	Reason          string
	PromptVersion   string
	Scope           string
	Source          string
}

// Memory is the decision cache and the alias ledger.
//
// Every method is allowed to fail without failing the run. A cache that is down
// makes an import slower and dearer; an import that refuses to run because the
// cache is down is worse than no cache at all.
type Memory interface {
	// Lookup returns the remembered answers for a batch of keys. Keys with no
	// answer are simply absent from the map.
	Lookup(ctx context.Context, keys []string) (map[string]Remembered, error)
	// Save records what the model decided. Callers filter by
	// MinMemoryConfidence before calling; this does not second-guess them.
	Save(ctx context.Context, decisions []Remembered) error
	// SaveAlias records an accepted match as an UNTRUSTED alias, source
	// 'ai_confirmed', which the deterministic alias tier deliberately excludes.
	// The row exists so a person accepting the match can promote it and so an
	// operator can see what the model has been deciding — not so the next
	// import trusts it.
	SaveAlias(ctx context.Context, productID int64, alias, source string, confidence float64) error
}

// Recall resolves what is already known, and reports what is left to ask.
//
// It is offered here rather than written out in each tool because the shape is
// identical everywhere and the two existing copies had already drifted on the
// detail that matters: whether a remembered answer below the apply floor counts
// as answered. It does — the model was asked and declined, and asking it again
// costs money to be declined again.
//
// A nil Memory answers nothing and reports every key as pending, which is what
// makes the cache optional at every call site rather than at some of them.
func Recall(ctx context.Context, m Memory, keys []string) map[string]Remembered {
	if m == nil || len(keys) == 0 {
		return nil
	}
	found, err := m.Lookup(ctx, keys)
	if err != nil {
		// A cache miss is never fatal, and neither is a cache error.
		return nil
	}
	return found
}
