package assistant

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// Turning an answer into something you can click.
//
// The assistant answers from rows it read out of the database, and every one of
// those rows is a record the user already has a screen for. Before this, an
// answer naming "طلب رقم PO-2025-0012" was a dead end: the reader had the
// number and had to go and find the order themselves.
//
// The mechanism here is deliberately general rather than a special case for
// orders. A read-model row declares itself referenceable by implementing
// Linkable; Collect walks whatever a tool returned and finds every such row,
// however deeply nested; ResolveLinks turns each one into a URL for the
// dashboard the caller is actually on. A tool added tomorrow whose rows
// implement Linkable becomes clickable by existing, with no change here, in the
// registry, or in the browser.
//
// Two rules keep it honest:
//
//   - An entity with no page on the caller's own dashboard produces no link.
//     A link that lands on a 404 or a permission refusal is worse than plain
//     text, so kinds with no destination are dropped rather than pointed at
//     something approximate.
//   - The URL is built HERE, from the row id the server itself read, never from
//     anything the model wrote. The model never sees an id and cannot influence
//     a href.

// EntityKind names a class of record that has a dashboard page.
type EntityKind string

const (
	EntityOrder        EntityKind = "order"
	EntityShipment     EntityKind = "shipment"
	EntityProduct      EntityKind = "product"
	EntityOffer        EntityKind = "offer"
	EntityOrganization EntityKind = "organization"
	EntityBranch       EntityKind = "branch"
)

// EntityAction is a secondary destination for a record — the invoice behind an
// order, for instance. Rendered as a small button beside the record's chip.
type EntityAction struct {
	Label string `json:"label"`
	URL   string `json:"url"`
	Icon  string `json:"icon,omitempty"`
}

// Entity is one referenceable record, ready for the browser.
//
// Label is what the model will have written in its prose — an order number, a
// product name — and is what the client matches on to place the link inline.
// Title and Subtitle are for the chip beneath the answer, which is the fallback
// when the model phrased something differently than the row spells it.
type Entity struct {
	Kind     EntityKind     `json:"kind"`
	ID       int64          `json:"id"`
	Label    string         `json:"label"`
	Aliases  []string       `json:"aliases,omitempty"`
	Title    string         `json:"title"`
	Subtitle string         `json:"subtitle,omitempty"`
	URL      string         `json:"url,omitempty"`
	Actions  []EntityAction `json:"actions,omitempty"`
	// Proposal is set on an EntityProposal: a pending action to confirm.
	Proposal *ProposalCard `json:"proposal,omitempty"`

	// mentionAt is where in the answer this record was first named. It orders
	// the reference chips to follow the prose and is never serialised.
	mentionAt int
}

// Linkable is implemented by a read-model row that points at a real record.
//
// The returned Entity carries no URL: which page a record has depends on the
// dashboard the caller is on, and a row shape does not know that. ResolveLinks
// fills it in.
type Linkable interface {
	EntityRef() Entity
}

// MaxEntitiesPerTurn bounds how many records one answer may carry.
//
// Every entity costs a chip in the drawer and a pass over the answer's text, so
// an unbounded list would turn a twenty-five-row listing into a wall of chips.
// Twenty-five is one full page from any tool.
const MaxEntitiesPerTurn = 25

// collectLimits bounds the reflective walk so a pathological result cannot make
// it expensive. Tool results are already capped at 6 KB of JSON, so these are
// generous.
const (
	maxCollectDepth = 8
	maxCollectNodes = 4000
)

// CollectEntities finds every referenceable record inside a tool result.
//
// It walks the value reflectively rather than asking each tool to declare what
// it returned, because the tools return different shapes — a Page of rows, a
// detail struct with a row inside it, a map of named slices — and a rule that
// each tool must remember to declare its own entities is a rule that gets
// forgotten the first time somebody adds a tool.
func CollectEntities(v any) []Entity {
	if v == nil {
		return nil
	}
	c := &entityCollector{seen: make(map[string]bool)}
	c.walk(reflect.ValueOf(v), 0)
	return c.out
}

type entityCollector struct {
	out   []Entity
	seen  map[string]bool
	nodes int
}

func (c *entityCollector) add(e Entity) {
	if e.Kind == "" || e.ID <= 0 {
		return
	}
	key := string(e.Kind) + ":" + strconv.FormatInt(e.ID, 10)
	if c.seen[key] {
		return
	}
	c.seen[key] = true
	c.out = append(c.out, e)
}

// take reports whether this value is itself a referenceable row, in which case
// the walk stops rather than descending into its fields.
func (c *entityCollector) take(v reflect.Value) bool {
	if v.Kind() != reflect.Struct {
		return false
	}
	if l, ok := v.Interface().(Linkable); ok {
		c.add(l.EntityRef())
		return true
	}
	// A row may declare EntityRef on its pointer receiver. Addressability is
	// not guaranteed for a value pulled out of an interface, so make a copy we
	// can take the address of.
	ptr := reflect.New(v.Type())
	ptr.Elem().Set(v)
	if l, ok := ptr.Interface().(Linkable); ok {
		c.add(l.EntityRef())
		return true
	}
	return false
}

func (c *entityCollector) walk(v reflect.Value, depth int) {
	if depth > maxCollectDepth || c.nodes > maxCollectNodes || len(c.out) >= MaxEntitiesPerTurn {
		return
	}
	c.nodes++

	switch v.Kind() {
	case reflect.Invalid:
		return
	case reflect.Interface, reflect.Pointer:
		if v.IsNil() {
			return
		}
		c.walk(v.Elem(), depth+1)
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			c.walk(v.Index(i), depth+1)
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			c.walk(v.MapIndex(k), depth+1)
		}
	case reflect.Struct:
		if c.take(v) {
			return
		}
		for i := 0; i < v.NumField(); i++ {
			if !v.Type().Field(i).IsExported() {
				continue
			}
			c.walk(v.Field(i), depth+1)
		}
	}
}

// MergeEntities appends new entities to an accumulator, dropping duplicates and
// stopping at the per-turn ceiling. Used across the tool rounds of one turn.
func MergeEntities(into []Entity, more ...Entity) []Entity {
	seen := make(map[string]bool, len(into))
	for _, e := range into {
		seen[string(e.Kind)+":"+strconv.FormatInt(e.ID, 10)] = true
	}
	for _, e := range more {
		if len(into) >= MaxEntitiesPerTurn {
			break
		}
		key := string(e.Kind) + ":" + strconv.FormatInt(e.ID, 10)
		if seen[key] {
			continue
		}
		seen[key] = true
		into = append(into, e)
	}
	return into
}

// ResolveLinks fills in each entity's destination for one dashboard, dropping
// the ones that have no page there.
//
// Dropping is the important half. A vendor has no page for a pharmacy's order
// and a pharmacy has none for an organisation record; pointing either at a
// plausible-looking URL would produce a link that refuses on click, which reads
// to the user as the assistant being wrong rather than as a permission.
func ResolveLinks(scope rbac.Scope, ents []Entity) []Entity {
	out := make([]Entity, 0, len(ents))
	for _, e := range ents {
		if strings.TrimSpace(e.Label) == "" {
			continue
		}
		if e.Kind == EntityExport || e.Kind == EntityProposal {
			// Not dashboard records: the server built their destination
			// when it created them, and they belong to this caller already.
			if e.Kind == EntityProposal || strings.HasPrefix(e.URL, ExportPath("")) {
				out = append(out, e)
			}
			continue
		}
		url, actions := destinationFor(scope, e.Kind, e.ID)
		if url == "" {
			continue
		}
		e.URL = url
		e.Actions = actions
		out = append(out, e)
	}
	return out
}

// destinationFor is the whole routing table, in one place.
//
// It names paths that exist in internal/ui/*_routes.go. When a page moves, this
// is the single thing to change, and the assistant follows.
func destinationFor(scope rbac.Scope, kind EntityKind, id int64) (string, []EntityAction) {
	n := strconv.FormatInt(id, 10)
	switch scope {
	case rbac.ScopePharmacy:
		switch kind {
		case EntityOrder:
			return "/orders/" + n, []EntityAction{{
				Label: "الفاتورة", URL: "/orders/" + n + "/invoice/print", Icon: "invoice",
			}}
		case EntityProduct:
			return "/customer/catalog/" + n, nil
		case EntityOffer:
			return "/customer/offers/" + n, nil
		case EntityBranch:
			return "/customer/branches", nil
		}
	case rbac.ScopeVendor:
		switch kind {
		case EntityOrder:
			return "/orders/" + n, []EntityAction{{
				Label: "الفاتورة", URL: "/orders/" + n + "/invoice/print", Icon: "invoice",
			}}
		case EntityShipment:
			// The vendor dashboard has no per-shipment page, so the link goes
			// to the list and the fragment lands on the row. The anchor is on
			// the card in pages/vendor_orders.templ; without it this would drop
			// the reader at the top of a list to search by hand.
			return "/vendor/orders#shipment-" + n, nil
		case EntityOffer:
			return "/vendor/offers/" + n + "/edit", nil
		case EntityProduct:
			// A vendor's own listing has no private detail page. The public
			// product page is the same row, and is how a buyer sees it — which
			// is usually what a supplier asking about their catalogue wants to
			// check.
			return "/catalog/" + n, nil
		case EntityBranch:
			return "/vendor/branches", nil
		}
	case rbac.ScopeAdmin:
		switch kind {
		case EntityOrganization:
			return "/admin/organizations/" + n, nil
		case EntityProduct:
			return "/admin/products/" + n, nil
		}
	}
	return "", nil
}
