package assistant

import (
	"strings"
)

// Which read-model rows are records the user can open.
//
// One method per row shape, and nothing else. A row that declares itself here
// becomes clickable everywhere the assistant can return it — in a listing, in a
// detail response, nested inside an aggregate — because the collector finds it
// by type rather than by where it appeared.
//
// Label is the string the model will have written in its prose. That is why it
// is the order NUMBER and not the id: the id is never shown to a model, and the
// number is what the answer says. Where a row has no number, the name is the
// label, which is also what the model writes.

// EntityRef makes a branch referenceable.
func (r BranchRow) EntityRef() Entity {
	label := strings.TrimSpace(r.Name)
	if label == "" {
		return Entity{}
	}
	return Entity{
		Kind:     EntityBranch,
		ID:       r.ID,
		Label:    label,
		Title:    label,
		Subtitle: strings.TrimSpace(r.City),
	}
}

// RecordEntity makes a dataset row referenceable. The label is the value the
// answer will quote — an order or shipment number, a name — and numbered
// records also match the ways a model rewrites a number.
func RecordEntity(kind EntityKind, id int64, label string) Entity {
	label = strings.TrimSpace(label)
	if label == "" || id <= 0 {
		return Entity{}
	}
	e := Entity{Kind: kind, ID: id, Label: label, Title: label}
	switch kind {
	case EntityOrder:
		e.Title, e.Aliases = "طلب شراء "+label, numberAliases(label)
	case EntityShipment:
		e.Title, e.Aliases = "شحنة "+label, numberAliases(label)
	}
	return e
}

// numberAliases returns the other ways a model may write a reference number.
//
// It writes "#PO-1042" as often as "PO-1042", and Arabic prose routinely drops
// the prefix entirely and says "طلب ١٠٤٢". Only the unambiguous variants are
// offered: the bare numeric tail is included solely when it is long enough that
// matching it cannot collide with an ordinary quantity or price in the same
// sentence.
func numberAliases(label string) []string {
	var out []string
	if !strings.HasPrefix(label, "#") {
		out = append(out, "#"+label)
	}
	if tail := numericTail(label); len(tail) >= 4 && tail != label {
		out = append(out, tail)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// numericTail returns the trailing run of digits in a reference, if any.
func numericTail(s string) string {
	end := len(s)
	i := end
	for i > 0 {
		c := s[i-1]
		if c < '0' || c > '9' {
			break
		}
		i--
	}
	if i == end {
		return ""
	}
	return s[i:end]
}
