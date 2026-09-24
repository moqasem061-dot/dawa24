package datasets

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/assistant/handles"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

// Zone is the platform's business time zone. Dates the model writes and the
// days, weeks and months results are grouped into are Cairo days, not UTC ones:
// an order placed at 01:00 on the first of the month belongs to that month.
const Zone = "Africa/Cairo"

// Options are the server's side of a compilation: limits, and how references
// are verified. None of them come from the request.
type Options struct {
	// MaxLimit caps rows (or groups) returned.
	MaxLimit int
	// DefaultLimit applies when the request names none.
	DefaultLimit int
	// MaxOffset caps paging depth.
	MaxOffset int
	// ResolveRef verifies a handle of the given kind for the live caller.
	ResolveRef func(kind handles.Kind, token string) (int64, error)
	// KeyID restricts the result to one record (get_record).
	KeyID int64
	// ParentKind and ParentID restrict the result to rows under one record.
	ParentKind handles.Kind
	ParentID   int64
}

// Column describes one output column of a plan.
type Column struct {
	Name    string       `json:"name"`
	Label   string       `json:"label"`
	Type    Type         `json:"type"`
	RefKind handles.Kind `json:"-"`
}

// Plan is a compiled request.
type Plan struct {
	Dataset   *Dataset
	SQL       string
	Args      []any
	Columns   []Column
	Aggregate bool
	// HasKey means every row carries the record id after Columns, followed by
	// the window total.
	HasKey bool
	Limit  int
	Offset int
}

type compiler struct {
	actor authctx.Actor
	d     *Dataset
	opts  Options
	args  []any
}

func (c *compiler) bind(v any) string {
	c.args = append(c.args, v)
	return "$" + strconv.Itoa(len(c.args))
}

// Compile turns a request into one parameterised statement.
func Compile(actor authctx.Actor, d *Dataset, req Request, opts Options) (*Plan, error) {
	if d == nil || !Readable(actor, d) {
		return nil, invalid("unknown dataset %q", req.Dataset)
	}
	c := &compiler{actor: actor, d: d, opts: opts}

	where, err := c.where(req)
	if err != nil {
		return nil, err
	}
	limit, offset, err := c.window(req)
	if err != nil {
		return nil, err
	}

	var plan *Plan
	if req.Aggregate() {
		plan, err = c.aggregate(req, where)
	} else {
		plan, err = c.rows(req, where)
	}
	if err != nil {
		return nil, err
	}
	plan.SQL += " LIMIT " + c.bind(limit) + " OFFSET " + c.bind(offset)
	plan.Args, plan.Limit, plan.Offset = c.args, limit, offset
	return plan, nil
}

func (c *compiler) window(req Request) (int, int, error) {
	limit := req.Limit
	switch {
	case limit < 0:
		return 0, 0, invalid("limit cannot be negative")
	case limit == 0:
		limit = c.opts.DefaultLimit
	}
	if limit <= 0 || limit > c.opts.MaxLimit {
		limit = c.opts.MaxLimit
	}
	if req.Offset < 0 || req.Offset > c.opts.MaxOffset {
		return 0, 0, invalid("offset must be between 0 and %d", c.opts.MaxOffset)
	}
	return limit, req.Offset, nil
}

// ---------------------------------------------------------------------------
// WHERE
// ---------------------------------------------------------------------------

func (c *compiler) where(req Request) (string, error) {
	tenant, err := c.tenant()
	if err != nil {
		return "", err
	}
	parts := []string{"(" + tenant + ")"}

	if c.opts.KeyID > 0 {
		if c.d.Key == nil {
			return "", invalid("dataset %q has no records to open", c.d.Name)
		}
		parts = append(parts, "("+c.d.Key.SQL+") = "+c.bind(c.opts.KeyID))
	}
	if c.opts.ParentKind != "" {
		expr, ok := c.d.Parents[c.opts.ParentKind]
		if !ok || c.opts.ParentID <= 0 {
			return "", invalid("dataset %q is not related to that record", c.d.Name)
		}
		parts = append(parts, "("+expr+") = "+c.bind(c.opts.ParentID))
	}

	if len(req.Filters) > MaxFilters {
		return "", invalid("at most %d filters", MaxFilters)
	}
	for _, f := range req.Filters {
		cond, err := c.filter(f)
		if err != nil {
			return "", err
		}
		parts = append(parts, cond)
	}
	if s := strings.TrimSpace(req.Search); s != "" {
		cond, err := c.search(s)
		if err != nil {
			return "", err
		}
		parts = append(parts, cond)
	}
	return " WHERE " + strings.Join(parts, " AND "), nil
}

// tenant renders the dataset's predicate with the caller's own ids. An
// organisation-scoped dataset asked by a caller with no organisation is
// refused, never run with a zero that might match something.
func (c *compiler) tenant() (string, error) {
	var failure error
	orgArg, userArg := "", ""
	effectiveOrgID := c.actor.OrgID
	if effectiveOrgID <= 0 {
		effectiveOrgID = c.actor.OrganizationID
	}
	sql := tenantToken.ReplaceAllStringFunc(c.d.Tenant, func(tok string) string {
		switch tok {
		case "@org":
			if effectiveOrgID <= 0 {
				failure = invalid("this dataset needs an organisation")
				return "NULL"
			}
			if orgArg == "" {
				orgArg = c.bind(effectiveOrgID)
			}
			return orgArg
		case "@user":
			if c.actor.UserID <= 0 {
				failure = invalid("this dataset needs a signed-in user")
				return "NULL"
			}
			if userArg == "" {
				userArg = c.bind(c.actor.UserID)
			}
			return userArg
		}
		failure = invalid("unsupported tenant token")
		return "NULL"
	})
	return sql, failure
}

func (c *compiler) field(name string) (*Field, error) {
	f, ok := c.d.Field(strings.TrimSpace(name))
	if !ok || !Visible(c.actor, f) {
		return nil, invalid("dataset %q has no field %q", c.d.Name, name)
	}
	return f, nil
}

var decimalValue = regexp.MustCompile(`^-?\d{1,13}(\.\d{1,4})?$`)

func (c *compiler) filter(flt Filter) (string, error) {
	f, err := c.field(flt.Field)
	if err != nil {
		return "", err
	}
	op := strings.ToLower(strings.TrimSpace(flt.Op))
	expr := "(" + f.SQL + ")"

	switch op {
	case "is_null":
		return expr + " IS NULL", nil
	case "not_null":
		return expr + " IS NOT NULL", nil
	}

	switch f.Type {
	case Text, Enum:
		return c.textFilter(f, expr, op, flt.Value)
	case Int, Number, Money:
		return c.numberFilter(f, expr, op, flt.Value)
	case Bool:
		if op != "eq" {
			return "", invalid("field %q supports eq only", f.Name)
		}
		var b bool
		if err := json.Unmarshal(flt.Value, &b); err != nil {
			return "", invalid("field %q needs true or false", f.Name)
		}
		return expr + " = " + c.bind(b), nil
	case Date:
		return c.dateFilter(f, expr, op, flt.Value)
	case Time:
		return c.timeFilter(f, expr, op, flt.Value)
	case Ref:
		return c.refFilter(f, expr, op, flt.Value)
	}
	return "", invalid("field %q cannot be filtered", f.Name)
}

func expandTypeSynonyms(v string) []string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "pharmacy", "customer", "chain_pharmacy", "individual", "صيدلية", "صيدليات", "عميل", "عملاء":
		return []string{"customer", "pharmacy", "chain_pharmacy", "individual"}
	case "supplier", "vendor", "company", "agency", "مورد", "موردين", "موردون", "شركة":
		return []string{"vendor", "supplier", "company", "agency"}
	default:
		return nil
	}
}

func expandStatusSynonyms(v string) []string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "pending", "new", "under_review", "جديد", "معلق", "قيد المراجعة", "تحت المراجعة":
		return []string{"pending", "under_review"}
	case "approved", "active", "معتمد", "مفعل", "نشط":
		return []string{"approved", "active"}
	case "suspended", "blocked", "inactive", "محظور", "موقوف", "معطل":
		return []string{"suspended", "blocked", "inactive"}
	case "rejected", "مرفوض":
		return []string{"rejected"}
	default:
		return nil
	}
}

func (c *compiler) textFilter(f *Field, expr, op string, raw json.RawMessage) (string, error) {
	switch op {
	case "eq", "ne", "contains", "starts_with":
		v, err := stringValue(f, raw)
		if err != nil {
			return "", err
		}
		if c.d != nil && c.d.Name == "organizations" {
			switch f.Name {
			case "type":
				if syn := expandTypeSynonyms(v); len(syn) > 0 {
					if op == "eq" {
						return expr + "::text = ANY(" + c.bind(syn) + "::text[])", nil
					}
					if op == "ne" {
						return "NOT (" + expr + "::text = ANY(" + c.bind(syn) + "::text[]))", nil
					}
				}
			case "status":
				if syn := expandStatusSynonyms(v); len(syn) > 0 {
					if op == "eq" {
						return expr + "::text = ANY(" + c.bind(syn) + "::text[])", nil
					}
					if op == "ne" {
						return "NOT (" + expr + "::text = ANY(" + c.bind(syn) + "::text[]))", nil
					}
				}
			}
		}
		switch op {
		case "eq":
			return expr + "::text = " + c.bind(v), nil
		case "ne":
			return expr + "::text IS DISTINCT FROM " + c.bind(v), nil
		case "contains":
			return expr + "::text ILIKE " + c.bind("%"+escapeLike(v)+"%"), nil
		default:
			return expr + "::text ILIKE " + c.bind(escapeLike(v)+"%"), nil
		}
	case "in", "not_in":
		var vs []string
		if err := json.Unmarshal(raw, &vs); err != nil || len(vs) == 0 || len(vs) > MaxInValues {
			return "", invalid("field %q %s needs a list of 1-%d strings", f.Name, op, MaxInValues)
		}
		for _, v := range vs {
			if len(v) > MaxValueChars {
				return "", invalid("a value for %q is too long", f.Name)
			}
		}
		if c.d != nil && c.d.Name == "organizations" {
			switch f.Name {
			case "type":
				var expanded []string
				for _, v := range vs {
					if syn := expandTypeSynonyms(v); len(syn) > 0 {
						expanded = append(expanded, syn...)
					} else {
						expanded = append(expanded, v)
					}
				}
				vs = expanded
			case "status":
				var expanded []string
				for _, v := range vs {
					if syn := expandStatusSynonyms(v); len(syn) > 0 {
						expanded = append(expanded, syn...)
					} else {
						expanded = append(expanded, v)
					}
				}
				vs = expanded
			}
		}
		if op == "in" {
			return expr + "::text = ANY(" + c.bind(vs) + "::text[])", nil
		}
		return "NOT (" + expr + "::text = ANY(" + c.bind(vs) + "::text[]))", nil
	}
	return "", invalid("field %q supports eq, ne, in, not_in, contains, starts_with, is_null, not_null", f.Name)
}

func stringValue(f *Field, raw json.RawMessage) (string, error) {
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", invalid("field %q needs a string value", f.Name)
	}
	if len(v) > MaxValueChars {
		return "", invalid("the value for %q is too long", f.Name)
	}
	return v, nil
}

// escapeLike makes a value match literally inside ILIKE, whose default escape
// character is a backslash.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
