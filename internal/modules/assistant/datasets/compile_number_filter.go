package datasets

import (
	"encoding/json"
	"strings"
	"time"
)

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
