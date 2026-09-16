package assistant

import (
	"sort"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// Deciding which records an answer actually refers to.
//
// A turn may read twenty-five orders to compute one total and then mention
// none of them. Attaching all twenty-five to the answer would put a wall of
// chips under a single number and would linkify text that never named a record.
//
// So the rule is: a record survives if the answer mentions it, and if nothing
// was mentioned the list is empty. That keeps the reference strip honest — every
// chip under an answer corresponds to something the answer actually said — and
// it makes the inline linking cheap, because the client is handed only labels
// that are known to be present.

// linkEntities resolves collected records for the caller's dashboard and keeps
// the ones the answer names.
func (s *Service) linkEntities(actor authctx.Actor, answer string, ents []Entity) []Entity {
	if len(ents) == 0 || strings.TrimSpace(answer) == "" {
		return nil
	}
	resolved := ResolveLinks(actor.DashboardScope(), ents)
	if len(resolved) == 0 {
		return nil
	}

	folded := FoldForMatch(answer)
	kept := make([]Entity, 0, len(resolved))
	for _, e := range resolved {
		if e.Kind == EntityExport || e.Kind == EntityProposal {
			// A file or a pending confirmation is shown whether or not the
			// answer names it: the user has to be able to act on it.
			e.mentionAt = len(folded) + len(kept)
			kept = append(kept, e)
			continue
		}
		if pos, ok := firstMention(folded, e); ok {
			e.mentionAt = pos
			kept = append(kept, e)
		}
	}
	// Order by where the answer first names them, so the chips read in the same
	// order as the sentence above them.
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].mentionAt < kept[j].mentionAt })
	return kept
}

// firstMention reports where in the answer a record is named, by its label or
// any of its aliases.
func firstMention(foldedAnswer string, e Entity) (int, bool) {
	best := -1
	for _, candidate := range append([]string{e.Label}, e.Aliases...) {
		needle := FoldForMatch(candidate)
		if len([]rune(needle)) < 3 {
			continue
		}
		if idx := strings.Index(foldedAnswer, needle); idx >= 0 && (best < 0 || idx < best) {
			best = idx
		}
	}
	return best, best >= 0
}

// FoldForMatch normalises text so a label written slightly differently still
// matches.
//
// Three things differ between what the database holds and what a model writes,
// and all three are cosmetic:
//
//   - Arabic presentation forms of the same letter (أ إ آ for ا, ة for ه, ى for
//     ي), which a model mixes freely inside one paragraph;
//   - Arabic-Indic digits (٠١٢…), which the assistant is asked to write and the
//     database never stores;
//   - runs of whitespace, tatweel, and case.
//
// Folding them away is the difference between "طلب رقم PO-1042" linking and
// not. It is used identically in the browser (capsuleFold), so the server's
// decision that a record is mentioned and the client's decision where to put
// the link cannot disagree.
func FoldForMatch(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	lastSpace := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r == 'ـ': // tatweel: decorative, never significant
			continue
		case r >= 'ً' && r <= 'ْ': // harakat
			continue
		case r >= '٠' && r <= '٩': // Arabic-Indic digits
			r = '0' + (r - '٠')
		case r >= '۰' && r <= '۹': // extended Arabic-Indic digits
			r = '0' + (r - '۰')
		case r == 'أ' || r == 'إ' || r == 'آ' || r == 'ٱ':
			r = 'ا'
		case r == 'ة':
			r = 'ه'
		case r == 'ى':
			r = 'ي'
		case r == 'ؤ':
			r = 'و'
		case r == 'ئ':
			r = 'ي'
		}
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == ' ' {
			if lastSpace {
				continue
			}
			lastSpace = true
			b.WriteRune(' ')
			continue
		}
		lastSpace = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}
