package datasets

import (
	"fmt"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

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
