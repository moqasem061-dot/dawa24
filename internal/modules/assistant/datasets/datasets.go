// Package datasets is the assistant's governed query engine.
//
// The old read model was a fixed list of projections: one SQL statement per
// question somebody anticipated, each returning at most twenty-five rows. A
// question nobody anticipated had no answer, and a question about totals was
// answered by a model adding up twenty-five rows of a table that had four
// thousand.
//
// A dataset replaces that with a declaration: which table, which columns may be
// shown, filtered, grouped and summed, which permission admits the caller, and
// — the part that cannot be left out — the tenant predicate. The model asks in
// terms of those declared names; this package compiles the request into one
// parameterised statement. Aggregates run in the database over the whole
// table, so "how much did we spend with each supplier in 2025" is exact however
// many rows that is.
//
// What the model can never do, by construction:
//
//   - name a table, a column, a function or an operator that is not declared
//     here (every identifier in the SQL comes from this package's source);
//   - remove, weaken or bypass the tenant predicate (it is ANDed in by the
//     compiler, outside anything the request controls);
//   - put a value anywhere but a bound parameter;
//   - see a field whose permission the caller does not hold.
//
// It is still not the only line. The executor runs every compiled statement in
// a read-only transaction under a database role that can SELECT only the
// tables and columns datasets use (migration 214), so a mistake in a
// declaration cannot reach a password hash or write a row.
package datasets

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// Type is how a field's values are compared, grouped and rendered.
type Type string

const (
	Text   Type = "text"
	Enum   Type = "enum"
	Int    Type = "int"
	Number Type = "number"
	Money  Type = "money"
	Bool   Type = "bool"
	Date   Type = "date"
	Time   Type = "time"
	// Ref is a record reference. Selected, it renders as a signed handle; as a
	// filter it accepts only a handle the caller was issued.
	Ref Type = "ref"
)

// Field is one declared column.
type Field struct {
	Name  string
	Label string
	// SQL is a server-owned expression over the dataset's FROM clause.
	SQL  string
	Type Type
	// Values documents the known values of an Enum. Filters are not restricted
	// to it — a new status must not make a question unanswerable — but the
	// list is what describe_data shows the model.
	Values []string
	// RefKind is the handle kind of a Ref field.
	RefKind handles.Kind
	// Permissions, when set, are required in addition to the dataset's: any
	// one admits. A field the caller may not see is not described, cannot be
	// selected, filtered, grouped, sorted or aggregated.
	Permissions []string
	// Search includes the field in free-text search.
	Search bool
	// Hidden fields are not selected by default in row mode.
	Hidden bool
}

// Key identifies a dataset's records for get_record and links.
type Key struct {
	// SQL is the record id expression.
	SQL  string
	Kind handles.Kind
	// Entity makes rows clickable: the kind of dashboard page, and the field
	// whose value the answer will use to name the record.
	Entity     string
	LabelField string
}

// Dataset is one table the assistant may read, as the caller's dashboard sees
// it.
type Dataset struct {
	Name        string
	Label       string
	Description string
	Scope       rbac.Scope
	// Permissions admit the caller: any one. They are the keys of the
	// dashboard screen that lists the same records.
	Permissions []string
	// From is the FROM clause with its joins.
	From string
	// Tenant is the row predicate. It may use @org (the caller's organisation)
	// and @user (the caller). It is mandatory: an admin dataset states TRUE
	// explicitly, and Build refuses a pharmacy or vendor dataset whose
	// predicate does not mention @org or @user.
	Tenant string
	Fields []Field
	// Key is optional; datasets without one cannot be opened by get_record.
	Key *Key
	// Parents lets get_record list this dataset's rows under a parent record:
	// handle kind → the SQL expression holding the parent's id.
	Parents map[handles.Kind]string
	// DefaultSort is a field name, sorted descending, for row mode.
	DefaultSort string
	// TimeField is the field "period" questions filter on by default.
	TimeField string

	byName map[string]*Field
}

// Field returns a declared field.
func (d *Dataset) Field(name string) (*Field, bool) {
	f, ok := d.byName[name]
	return f, ok
}

// Catalog holds every dataset. A name may be declared once per dashboard: the
// buying datasets exist for pharmacies and for suppliers that buy, each under
// its own dashboard's permission keys.
type Catalog struct {
	sets []*Dataset
}

// identifier is what a dataset, field or metric name may look like. Names are
// server-owned, but they are echoed into result keys and column aliases, so
// they are held to a shape that needs no quoting.
var identifier = regexp.MustCompile(`^[a-z][a-z0-9_]{1,40}$`)

// tenantToken finds the named parameters a predicate may use.
var tenantToken = regexp.MustCompile(`@[a-z]+`)

// Build validates declarations and assembles a catalogue. It returns an error
// rather than panicking so tests can assert on it; New panics.
func Build(sets ...Dataset) (*Catalog, error) {
	c := &Catalog{}
	seen := map[string]bool{}
	for i := range sets {
		d := sets[i]
		if err := validate(&d); err != nil {
			return nil, fmt.Errorf("dataset %q: %w", d.Name, err)
		}
		key := string(d.Scope) + "/" + d.Name
		if seen[key] {
			return nil, fmt.Errorf("duplicate dataset %q", key)
		}
		seen[key] = true
		c.sets = append(c.sets, &d)
	}
	return c, nil
}

// New builds the catalogue and panics on a malformed declaration: a process
// that starts with a broken dataset either refuses every question about it or,
// worse, compiles something nobody reviewed.
func New(sets ...Dataset) *Catalog {
	c, err := Build(sets...)
	if err != nil {
		panic("assistant datasets: " + err.Error())
	}
	return c
}

func validate(d *Dataset) error {
	if !identifier.MatchString(d.Name) {
		return fmt.Errorf("invalid name")
	}
	if !rbac.ValidScope(d.Scope) {
		return fmt.Errorf("invalid scope %q", d.Scope)
	}
	if len(d.Permissions) == 0 {
		return fmt.Errorf("no permission")
	}
	if strings.TrimSpace(d.From) == "" || strings.TrimSpace(d.Tenant) == "" {
		return fmt.Errorf("FROM and tenant predicate are required")
	}
	for _, tok := range tenantToken.FindAllString(d.Tenant, -1) {
		if tok != "@org" && tok != "@user" {
			return fmt.Errorf("unknown tenant token %s", tok)
		}
	}
	if d.Scope != rbac.ScopeAdmin && !strings.Contains(d.Tenant, "@org") && !strings.Contains(d.Tenant, "@user") {
		return fmt.Errorf("a %s dataset must be scoped to @org or @user", d.Scope)
	}
	d.byName = make(map[string]*Field, len(d.Fields))
	for i := range d.Fields {
		f := &d.Fields[i]
		if !identifier.MatchString(f.Name) {
			return fmt.Errorf("invalid field name %q", f.Name)
		}
		if _, dup := d.byName[f.Name]; dup {
			return fmt.Errorf("duplicate field %q", f.Name)
		}
		if strings.TrimSpace(f.SQL) == "" || f.Label == "" {
			return fmt.Errorf("field %q needs SQL and a label", f.Name)
		}
		switch f.Type {
		case Text, Enum, Int, Number, Money, Bool, Date, Time:
		case Ref:
			if f.RefKind == "" {
				return fmt.Errorf("ref field %q has no handle kind", f.Name)
			}
		default:
			return fmt.Errorf("field %q has unknown type %q", f.Name, f.Type)
		}
		d.byName[f.Name] = f
	}
	for _, name := range []string{d.DefaultSort, d.TimeField} {
		if name != "" {
			if _, ok := d.byName[name]; !ok {
				return fmt.Errorf("default field %q is not declared", name)
			}
		}
	}
	if d.Key != nil {
		if d.Key.SQL == "" || d.Key.Kind == "" {
			return fmt.Errorf("key needs SQL and a handle kind")
		}
		if d.Key.LabelField != "" {
			if _, ok := d.byName[d.Key.LabelField]; !ok {
				return fmt.Errorf("key label field %q is not declared", d.Key.LabelField)
			}
		}
	}
	return nil
}

// All returns every dataset in declaration order.
func (c *Catalog) All() []*Dataset {
	return append([]*Dataset(nil), c.sets...)
}

// Get returns a dataset by dashboard and name regardless of caller. For tests;
// tools use For.
func (c *Catalog) Get(scope rbac.Scope, name string) (*Dataset, bool) {
	for _, d := range c.sets {
		if d.Scope == scope && d.Name == name {
			return d, true
		}
	}
	return nil, false
}

// For returns a dataset only when this caller may read it.
func (c *Catalog) For(actor authctx.Actor, name string) (*Dataset, bool) {
	d, ok := c.Get(actor.DashboardScope(), name)
	if !ok || !Readable(actor, d) {
		return nil, false
	}
	return d, true
}

// Readable reports whether the caller's dashboard and permissions admit a
// dataset.
func Readable(actor authctx.Actor, d *Dataset) bool {
	return actor.DashboardScope() == d.Scope && actor.CanAny(d.Permissions...)
}

// Visible reports whether the caller may see a field of a dataset they can
// read.
func Visible(actor authctx.Actor, f *Field) bool {
	return len(f.Permissions) == 0 || actor.CanAny(f.Permissions...)
}

// Available lists the datasets this caller may read, sorted by name.
func (c *Catalog) Available(actor authctx.Actor) []*Dataset {
	var out []*Dataset
	for _, d := range c.sets {
		if Readable(actor, d) {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ForKey returns the readable dataset whose records a handle kind names.
func (c *Catalog) ForKey(actor authctx.Actor, kind handles.Kind) (*Dataset, bool) {
	for _, d := range c.sets {
		if d.Key != nil && d.Key.Kind == kind && Readable(actor, d) {
			return d, true
		}
	}
	return nil, false
}

// Children lists the readable datasets that hang under a record of this kind.
func (c *Catalog) Children(actor authctx.Actor, kind handles.Kind) []*Dataset {
	var out []*Dataset
	for _, d := range c.sets {
		if _, ok := d.Parents[kind]; ok && Readable(actor, d) {
			out = append(out, d)
		}
	}
	return out
}
