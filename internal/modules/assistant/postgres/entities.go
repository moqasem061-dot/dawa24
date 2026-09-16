package postgres

import (
	"encoding/json"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
)

// Storing an answer's clickable references.
//
// They are written as JSONB rather than as rows in a join table, and that is a
// deliberate trade. These are not facts about the business — the order they
// point at may since have been deleted, renamed or moved to another branch —
// they are a snapshot of what one answer said at one moment. Normalising them
// would invite joins that quietly resurrect a record the caller can no longer
// see; keeping them inert means a stale link is a stale link, and the page it
// opens applies its own permissions exactly as it would for a typed URL.

// encodeEntities renders references for storage. A failure yields an empty
// array rather than an error: losing the links off one answer is not worth
// failing the write that persists the answer itself.
func encodeEntities(ents []assistant.Entity) []byte {
	if len(ents) == 0 {
		return []byte("[]")
	}
	raw, err := json.Marshal(ents)
	if err != nil {
		return []byte("[]")
	}
	return raw
}

// decodeEntities reads references back, tolerating rows written before the
// column existed.
func decodeEntities(raw []byte) []assistant.Entity {
	if len(raw) == 0 {
		return nil
	}
	var out []assistant.Entity
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}
