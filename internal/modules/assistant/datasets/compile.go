package datasets

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
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
			if f.Name == "type" {
				if syn := expandTypeSynonyms(v); len(syn) > 0 {
					if op == "eq" {
						return expr + "::text = ANY(" + c.bind(syn) + "::text[])", nil
					}
					if op == "ne" {
						return "NOT (" + expr + "::text = ANY(" + c.bind(syn) + "::text[]))", nil
					}
				}
			} else if f.Name == "status" {
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
			if f.Name == "type" {
				var expanded []string
				for _, v := range vs {
					if syn := expandTypeSynonyms(v); len(syn) > 0 {
						expanded = append(expanded, syn...)
					} else {
						expanded = append(expanded, v)
					}
				}
				vs = expanded
			} else if f.Name == "status" {
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

func (c *compiler) numberFilter(f *Field, expr, op string, raw json.RawMessage) (string, error) {
	cmp := map[string]string{"eq": "=", "gt": ">", "gte": ">=", "lt": "<", "lte": "<="}
	switch {
	case cmp[op] != "":
		v, err := numberValue(f, raw)
		if err != nil {
			return "", err
		}
		return expr + " " + cmp[op] + " " + c.bind(v) + "::numeric", nil
	case op == "ne":
		v, err := numberValue(f, raw)
		if err != nil {
			return "", err
		}
		return expr + " IS DISTINCT FROM " + c.bind(v) + "::numeric", nil
	case op == "between":
		var pair []json.RawMessage
		if err := json.Unmarshal(raw, &pair); err != nil || len(pair) != 2 {
			return "", invalid("field %q between needs [low, high]", f.Name)
		}
		lo, err := numberValue(f, pair[0])
		if err != nil {
			return "", err
		}
		hi, err := numberValue(f, pair[1])
		if err != nil {
			return "", err
		}
		return expr + " BETWEEN " + c.bind(lo) + "::numeric AND " + c.bind(hi) + "::numeric", nil
	case op == "in" || op == "not_in":
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil || len(items) == 0 || len(items) > MaxInValues {
			return "", invalid("field %q %s needs a list of 1-%d numbers", f.Name, op, MaxInValues)
		}
		vs := make([]string, len(items))
		for i, it := range items {
			v, err := numberValue(f, it)
			if err != nil {
				return "", err
			}
			vs[i] = v
		}
		cond := expr + " = ANY(" + c.bind(vs) + "::numeric[])"
		if op == "not_in" {
			cond = "NOT (" + cond + ")"
		}
		return cond, nil
	}
	return "", invalid("field %q supports eq, ne, gt, gte, lt, lte, between, in, not_in, is_null, not_null", f.Name)
}

// numberValue accepts a JSON number or a numeric string and returns the
// decimal text, bound and cast by the database. It never goes through float64:
// a total of 1234.10 must compare as exactly that.
func numberValue(f *Field, raw json.RawMessage) (string, error) {
	s := strings.TrimSpace(string(raw))
	if strings.HasPrefix(s, `"`) {
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", invalid("field %q needs a number", f.Name)
		}
	}
	if !decimalValue.MatchString(s) {
		return "", invalid("field %q needs a number", f.Name)
	}
	if f.Type == Int && strings.Contains(s, ".") {
		return "", invalid("field %q needs a whole number", f.Name)
	}
	return s, nil
}

// dateFilter compares calendar dates.
func (c *compiler) dateFilter(f *Field, expr, op string, raw json.RawMessage) (string, error) {
	cmp := map[string]string{"eq": "=", "gt": ">", "gte": ">=", "lt": "<", "lte": "<="}
	expr += "::date"
	if sym := cmp[op]; sym != "" {
		d, err := dayValue(f, raw)
		if err != nil {
			return "", err
		}
		return expr + " " + sym + " " + c.bind(d.Format("2006-01-02")) + "::date", nil
	}
	if op == "between" {
		lo, hi, err := dayPair(f, raw)
		if err != nil {
			return "", err
		}
		return expr + " BETWEEN " + c.bind(lo.Format("2006-01-02")) + "::date AND " +
			c.bind(hi.Format("2006-01-02")) + "::date", nil
	}
	return "", invalid("field %q supports eq, gt, gte, lt, lte, between, is_null, not_null with YYYY-MM-DD", f.Name)
}

// timeFilter compares instants against Cairo-local days or minutes.
//
// A date-only bound means the whole day, so the boundaries are:
//
//	gte D  → at or after D 00:00       gt D → at or after (D+1) 00:00
//	lt  D  → before D 00:00            lte D → before (D+1) 00:00
//	eq  D  → [D 00:00, D+1 00:00)      between [A,B] → [A 00:00, B+1 00:00)
//
// A bound with a time ("2025-03-01 14:30") is compared exactly.
func (c *compiler) timeFilter(f *Field, expr, op string, raw json.RawMessage) (string, error) {
	switch op {
	case "gte", "gt", "lt", "lte", "eq":
		t, dayOnly, err := instantValue(f, raw)
		if err != nil {
			return "", err
		}
		start := c.local(t)
		if !dayOnly {
			sym := map[string]string{"eq": "=", "gt": ">", "gte": ">=", "lt": "<", "lte": "<="}[op]
			return expr + " " + sym + " " + start, nil
		}
		next := c.local(t.AddDate(0, 0, 1))
		switch op {
		case "gte":
			return expr + " >= " + start, nil
		case "gt":
			return expr + " >= " + next, nil
		case "lt":
			return expr + " < " + start, nil
		case "lte":
			return expr + " < " + next, nil
		default:
			return "(" + expr + " >= " + start + " AND " + expr + " < " + next + ")", nil
		}
	case "between":
		lo, hi, err := dayPair(f, raw)
		if err != nil {
			return "", err
		}
		return "(" + expr + " >= " + c.local(lo) + " AND " + expr + " < " + c.local(hi.AddDate(0, 0, 1)) + ")", nil
	}
	return "", invalid("field %q supports eq, gt, gte, lt, lte, between, is_null, not_null", f.Name)
}

// local binds a Cairo wall-clock time and converts it to an instant in SQL, so
// daylight saving is the database's problem and not a hand-rolled offset.
func (c *compiler) local(t time.Time) string {
	return "(" + c.bind(t.Format("2006-01-02 15:04:05")) + "::timestamp AT TIME ZONE '" + Zone + "')"
}

func dayValue(f *Field, raw json.RawMessage) (time.Time, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return time.Time{}, invalid("field %q needs a date YYYY-MM-DD", f.Name)
	}
	t, err := time.Parse("2006-01-02", strings.TrimSpace(s))
	if err != nil {
		return time.Time{}, invalid("field %q needs a date YYYY-MM-DD", f.Name)
	}
	return t, nil
}

func dayPair(f *Field, raw json.RawMessage) (time.Time, time.Time, error) {
	var pair []json.RawMessage
	if err := json.Unmarshal(raw, &pair); err != nil || len(pair) != 2 {
		return time.Time{}, time.Time{}, invalid("field %q between needs [\"YYYY-MM-DD\", \"YYYY-MM-DD\"]", f.Name)
	}
	lo, err := dayValue(f, pair[0])
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	hi, err := dayValue(f, pair[1])
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if hi.Before(lo) {
		return time.Time{}, time.Time{}, invalid("field %q range ends before it starts", f.Name)
	}
	return lo, hi, nil
}

func instantValue(f *Field, raw json.RawMessage) (time.Time, bool, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return time.Time{}, false, invalid("field %q needs \"YYYY-MM-DD\" or \"YYYY-MM-DD HH:MM\"", f.Name)
	}
	s = strings.TrimSpace(s)
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, true, nil
	}
	if t, err := time.Parse("2006-01-02 15:04", s); err == nil {
		return t, false, nil
	}
	return time.Time{}, false, invalid("field %q needs \"YYYY-MM-DD\" or \"YYYY-MM-DD HH:MM\"", f.Name)
}

func (c *compiler) refFilter(f *Field, expr, op string, raw json.RawMessage) (string, error) {
	if c.opts.ResolveRef == nil {
		return "", invalid("field %q cannot be filtered here", f.Name)
	}
	var tokens []string
	switch op {
	case "eq":
		var t string
		if err := json.Unmarshal(raw, &t); err != nil {
			return "", invalid("field %q needs a reference returned by an earlier result", f.Name)
		}
		tokens = []string{t}
	case "in":
		if err := json.Unmarshal(raw, &tokens); err != nil || len(tokens) == 0 || len(tokens) > MaxInValues {
			return "", invalid("field %q in needs a list of references", f.Name)
		}
	default:
		return "", invalid("field %q supports eq and in", f.Name)
	}
	ids := make([]int64, len(tokens))
	for i, t := range tokens {
		id, err := c.opts.ResolveRef(f.RefKind, strings.TrimSpace(t))
		if err != nil {
			return "", ErrHandle
		}
		ids[i] = id
	}
	return expr + " = ANY(" + c.bind(ids) + "::bigint[])", nil
}

func (c *compiler) search(s string) (string, error) {
	if len([]rune(s)) > MaxSearchChars {
		return "", invalid("search text is too long")
	}
	var cols []string
	for i := range c.d.Fields {
		f := &c.d.Fields[i]
		if f.Search && Visible(c.actor, f) {
			cols = append(cols, "("+f.SQL+")::text ILIKE {p}")
		}
	}
	if len(cols) == 0 {
		return "", invalid("dataset %q has no searchable text; use filters", c.d.Name)
	}
	p := c.bind("%" + escapeLike(s) + "%")
	return "(" + strings.ReplaceAll(strings.Join(cols, " OR "), "{p}", p) + ")", nil
}

// ---------------------------------------------------------------------------
// SELECT
// ---------------------------------------------------------------------------

// render converts a value to the text the executor scans, in a form that needs
// no further interpretation: money to two decimals, instants as Cairo local
// time.
func render(t Type, expr string) string {
	switch t {
	case Money:
		return "round((" + expr + ")::numeric, 2)::text"
	case Number:
		return "(" + expr + ")::numeric::text"
	case Date:
		return "to_char((" + expr + ")::date, 'YYYY-MM-DD')"
	case Time:
		return "to_char((" + expr + ") AT TIME ZONE '" + Zone + "', 'YYYY-MM-DD HH24:MI')"
	default:
		return "(" + expr + ")::text"
	}
}

func (c *compiler) rows(req Request, where string) (*Plan, error) {
	selected, err := c.selection(req.Fields)
	if err != nil {
		return nil, err
	}
	plan := &Plan{Dataset: c.d, HasKey: c.d.Key != nil}
	exprs := make([]string, 0, len(selected)+2)
	for _, f := range selected {
		exprs = append(exprs, render(f.Type, f.SQL))
		plan.Columns = append(plan.Columns, Column{Name: f.Name, Label: f.Label, Type: f.Type, RefKind: f.RefKind})
	}
	tiebreak := ""
	if plan.HasKey {
		exprs = append(exprs, "("+c.d.Key.SQL+")::bigint")
		tiebreak = "(" + c.d.Key.SQL + ") DESC"
	}
	exprs = append(exprs, "count(*) OVER ()")

	order, err := c.rowOrder(req.Sort)
	if err != nil {
		return nil, err
	}
	if tiebreak != "" {
		order = append(order, tiebreak)
	}
	plan.SQL = "SELECT " + strings.Join(exprs, ", ") + " FROM " + c.d.From + where
	if len(order) > 0 {
		plan.SQL += " ORDER BY " + strings.Join(order, ", ")
	}
	return plan, nil
}

func (c *compiler) selection(names []string) ([]*Field, error) {
	if len(names) > MaxFields {
		return nil, invalid("at most %d fields", MaxFields)
	}
	var out []*Field
	seen := map[string]bool{}
	if len(names) == 0 {
		for i := range c.d.Fields {
			f := &c.d.Fields[i]
			if !f.Hidden && Visible(c.actor, f) {
				out = append(out, f)
			}
		}
		return out, nil
	}
	for _, n := range names {
		f, err := c.field(n)
		if err != nil {
			return nil, err
		}
		if !seen[f.Name] {
			seen[f.Name] = true
			out = append(out, f)
		}
	}
	return out, nil
}

func direction(dir string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(dir)) {
	case "", "desc":
		return "DESC", nil
	case "asc":
		return "ASC", nil
	}
	return "", invalid("sort direction must be asc or desc")
}

func (c *compiler) rowOrder(sorts []Sort) ([]string, error) {
	if len(sorts) > MaxSorts {
		return nil, invalid("at most %d sorts", MaxSorts)
	}
	var out []string
	for _, s := range sorts {
		f, err := c.field(s.By)
		if err != nil {
			return nil, err
		}
		if f.Type == Ref {
			return nil, invalid("field %q cannot be sorted", f.Name)
		}
		dir, err := direction(s.Dir)
		if err != nil {
			return nil, err
		}
		out = append(out, "("+f.SQL+") "+dir+" NULLS LAST")
	}
	if len(out) == 0 && c.d.DefaultSort != "" {
		if f, ok := c.d.Field(c.d.DefaultSort); ok {
			out = append(out, "("+f.SQL+") DESC NULLS LAST")
		}
	}
	return out, nil
}

// aggregateColumn is one grouped or summarised output with the raw expression
// used to order by it — ordering by the rendered text would sort 900 after
// 10000.
type aggregateColumn struct {
	Column
	render string
	raw    string
}

var grains = map[string]struct{ trunc, format string }{
	"day":     {"day", "YYYY-MM-DD"},
	"week":    {"week", `IYYY-"W"IW`},
	"month":   {"month", "YYYY-MM"},
	"quarter": {"quarter", `YYYY-"Q"Q`},
	"year":    {"year", "YYYY"},
}

func (c *compiler) groupColumn(spec string) (aggregateColumn, error) {
	name, grain, hasGrain := strings.Cut(strings.TrimSpace(spec), ":")
	f, err := c.field(name)
	if err != nil {
		return aggregateColumn{}, err
	}
	col := aggregateColumn{Column: Column{Name: f.Name, Label: f.Label, Type: f.Type}}
	switch f.Type {
	case Text, Enum, Bool, Int:
		if hasGrain {
			return col, invalid("field %q takes no time grain", f.Name)
		}
		col.raw = "(" + f.SQL + ")"
		col.render = render(f.Type, f.SQL)
		return col, nil
	case Date, Time:
		if !hasGrain {
			grain = "day"
		}
		g, ok := grains[strings.ToLower(grain)]
		if !ok {
			return col, invalid("time grain must be day, week, month, quarter or year")
		}
		local := "(" + f.SQL + ")::timestamp"
		if f.Type == Time {
			local = "((" + f.SQL + ") AT TIME ZONE '" + Zone + "')"
		}
		col.Name = f.Name + "_" + strings.ToLower(grain)
		col.Type = Text
		col.raw = "date_trunc('" + g.trunc + "', " + local + ")"
		col.render = "to_char(" + col.raw + ", '" + g.format + "')"
		return col, nil
	}
	return col, invalid("field %q cannot be grouped", f.Name)
}

func (c *compiler) metricColumn(spec string) (aggregateColumn, error) {
	fn, name, hasField := strings.Cut(strings.ToLower(strings.TrimSpace(spec)), ":")
	if fn == "count" && !hasField {
		return aggregateColumn{
			Column: Column{Name: "count", Label: "العدد", Type: Int},
			raw:    "count(*)", render: "count(*)::text",
		}, nil
	}
	if !hasField {
		return aggregateColumn{}, invalid("metric %q must be count or fn:field", spec)
	}
	f, err := c.field(name)
	if err != nil {
		return aggregateColumn{}, err
	}
	col := aggregateColumn{Column: Column{Name: fn + "_" + f.Name, Type: f.Type}}
	expr := "(" + f.SQL + ")"
	numeric := f.Type == Int || f.Type == Number || f.Type == Money
	switch fn {
	case "count_distinct":
		if f.Type == Number || f.Type == Money {
			return col, invalid("count_distinct needs a text, id or date field")
		}
		col.Label, col.Type = "عدد "+f.Label+" (بدون تكرار)", Int
		col.raw = "count(DISTINCT " + expr + ")"
		col.render = col.raw + "::text"
	case "sum", "avg":
		if !numeric {
			return col, invalid("%s needs a numeric field", fn)
		}
		col.Label = map[string]string{"sum": "إجمالي ", "avg": "متوسط "}[fn] + f.Label
		col.raw = fn + "(" + expr + ")"
		if fn == "avg" && f.Type == Int {
			col.Type = Number
		}
		col.render = "round(" + col.raw + "::numeric, 2)::text"
	case "min", "max":
		if !numeric && f.Type != Date && f.Type != Time {
			return col, invalid("%s needs a numeric or date field", fn)
		}
		col.Label = map[string]string{"min": "أقل ", "max": "أعلى "}[fn] + f.Label
		col.raw = fn + "(" + expr + ")"
		col.render = render(f.Type, col.raw)
	default:
		return col, invalid("metric must be count, count_distinct, sum, avg, min or max")
	}
	return col, nil
}

func (c *compiler) aggregate(req Request, where string) (*Plan, error) {
	if len(req.Fields) > 0 {
		return nil, invalid("fields cannot be combined with group_by or metrics")
	}
	if len(req.GroupBy) > MaxGroupBy {
		return nil, invalid("at most %d group_by entries", MaxGroupBy)
	}
	if len(req.Metrics) > MaxMetrics {
		return nil, invalid("at most %d metrics", MaxMetrics)
	}
	metrics := req.Metrics
	if len(metrics) == 0 {
		metrics = []string{"count"}
	}

	var groups, measures []aggregateColumn
	byName := map[string]aggregateColumn{}
	for _, g := range req.GroupBy {
		col, err := c.groupColumn(g)
		if err != nil {
			return nil, err
		}
		if _, dup := byName[col.Name]; dup {
			return nil, invalid("duplicate group %q", col.Name)
		}
		groups = append(groups, col)
		byName[col.Name] = col
	}
	for _, m := range metrics {
		col, err := c.metricColumn(m)
		if err != nil {
			return nil, err
		}
		if _, dup := byName[col.Name]; dup {
			return nil, invalid("duplicate metric %q", col.Name)
		}
		measures = append(measures, col)
		byName[col.Name] = col
	}

	plan := &Plan{Dataset: c.d, Aggregate: true}
	var exprs, groupBy []string
	for _, col := range append(append([]aggregateColumn{}, groups...), measures...) {
		exprs = append(exprs, col.render)
		plan.Columns = append(plan.Columns, col.Column)
	}
	for _, g := range groups {
		groupBy = append(groupBy, g.raw)
	}
	exprs = append(exprs, "count(*) OVER ()")

	order, err := aggregateOrder(req.Sort, byName, groups, measures)
	if err != nil {
		return nil, err
	}
	plan.SQL = "SELECT " + strings.Join(exprs, ", ") + " FROM " + c.d.From + where
	if len(groupBy) > 0 {
		plan.SQL += " GROUP BY " + strings.Join(groupBy, ", ")
	}
	if len(order) > 0 {
		plan.SQL += " ORDER BY " + strings.Join(order, ", ")
	}
	return plan, nil
}

func aggregateOrder(sorts []Sort, byName map[string]aggregateColumn, groups, measures []aggregateColumn) ([]string, error) {
	if len(sorts) > MaxSorts {
		return nil, invalid("at most %d sorts", MaxSorts)
	}
	var out []string
	for _, s := range sorts {
		col, ok := byName[strings.TrimSpace(strings.ReplaceAll(s.By, ":", "_"))]
		if !ok {
			return nil, invalid("sort must name a group or metric in this request")
		}
		dir, err := direction(s.Dir)
		if err != nil {
			return nil, err
		}
		out = append(out, col.raw+" "+dir+" NULLS LAST")
	}
	if len(groups) == 0 {
		return out, nil
	}
	if len(out) == 0 {
		// Time series read oldest first; everything else biggest first.
		if strings.HasPrefix(groups[0].raw, "date_trunc(") {
			out = append(out, groups[0].raw+" ASC")
		} else {
			out = append(out, measures[0].raw+" DESC NULLS LAST")
		}
	}
	// Stable paging across equal values.
	for _, g := range groups {
		out = append(out, g.raw+" ASC NULLS LAST")
	}
	return out, nil
}

// Describe summarises what a caller may ask of a dataset, for describe_data.
func Describe(actor authctx.Actor, d *Dataset) map[string]any {
	fields := make([]map[string]any, 0, len(d.Fields))
	for i := range d.Fields {
		f := &d.Fields[i]
		if !Visible(actor, f) {
			continue
		}
		entry := map[string]any{"name": f.Name, "label": f.Label, "type": string(f.Type)}
		if len(f.Values) > 0 {
			entry["values"] = f.Values
		}
		if f.Search {
			entry["searchable"] = true
		}
		fields = append(fields, entry)
	}
	out := map[string]any{
		"dataset":     d.Name,
		"label":       d.Label,
		"description": d.Description,
		"fields":      fields,
	}
	if d.TimeField != "" {
		out["time_field"] = d.TimeField
	}
	if d.Key != nil {
		out["opens_with_get_record"] = true
	}
	return out
}

// String renders a plan for logs and tests. Args are counted, never printed:
// they are the caller's own values.
func (p *Plan) String() string {
	return fmt.Sprintf("%s [%d args]", p.SQL, len(p.Args))
}
