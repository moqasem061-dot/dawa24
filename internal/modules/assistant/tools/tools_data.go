package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/datasets"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/timeutil"
)

// The data tools: one general way to read any declared table the caller may
// see, instead of one tool per anticipated question.
//
//	describe_data  what can be asked, and of which dataset
//	query_data     rows, or totals and groups computed in the database
//	get_record     one record in full, with the rows hanging under it
//	export_data    the same query as a spreadsheet, with no practical row cap
//
// Every call still passes the registry's ten steps. Inside them the dataset is
// re-checked against the live caller (dashboard, dataset permission, field
// permissions), references are verified handles, and the compiled statement
// runs as the read-only dataset role.

// DatasetRunner executes compiled plans. Implemented by assistant/postgres.
type DatasetRunner interface {
	RunDataset(ctx context.Context, plan *datasets.Plan, timeout time.Duration) (*datasets.Result, error)
}

// ExportStore keeps a generated file for its owner to download.
type ExportStore interface {
	SaveExport(ctx context.Context, actor authctx.Actor, file assistant.ExportFile) (token string, err error)
}

// Limits for the data tools.
const (
	queryRowLimit     = 200
	queryRowDefault   = 50
	queryGroupLimit   = 500
	queryGroupDefault = 100
	queryMaxOffset    = 10000
	queryTimeout      = 10 * time.Second
	relatedRowLimit   = 100
	exportRowLimit    = 50000
	exportTimeout     = 45 * time.Second
)

// SetDatasets wires the dataset engine. Without it the data tools report
// themselves unavailable instead of guessing.
func (r *Registry) SetDatasets(cat *datasets.Catalog, runner DatasetRunner, exports ExportStore) {
	r.datasets, r.datasetRunner, r.exports = cat, runner, exports
}

var allScopes = []rbac.Scope{rbac.ScopePharmacy, rbac.ScopeVendor, rbac.ScopeAdmin}
var anyGate = []string{assistant.GatePharmacy, assistant.GateVendor, assistant.GateAdmin}

func dataTools(r *Registry) []Tool {
	requestProps := map[string]any{
		"dataset": strProp("Dataset name from describe_data."),
		"fields":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Row mode only: fields to return. Omit for the default set."},
		"filters": map[string]any{
			"type": "array",
			"items": objectSchema(map[string]any{
				"field": strProp("Field name."),
				"op": enumProp("Operator.", "eq", "ne", "in", "not_in", "contains", "starts_with",
					"gt", "gte", "lt", "lte", "between", "is_null", "not_null"),
				"value": map[string]any{"description": "String, number, boolean, or array for in/not_in/between. Dates: \"YYYY-MM-DD\" (Cairo days). Ref fields take handles from earlier results."},
			}, "field", "op"),
		},
		"search":   strProp("Free text matched against the dataset's searchable fields."),
		"group_by": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Group fields; dates take a grain: \"created_at:month\" (day|week|month|quarter|year)."},
		"metrics":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "count | count_distinct:field | sum:field | avg:field | min:field | max:field. Computed over ALL matching rows."},
		"sort": map[string]any{"type": "array", "items": objectSchema(map[string]any{
			"by":  strProp("Row mode: a field. Aggregate mode: a group or metric, e.g. \"sum:total\"."),
			"dir": enumProp("Direction.", "asc", "desc"),
		}, "by")},
		"limit":  intProp("Rows (max 200) or groups (max 500) to return.", 1, queryGroupLimit),
		"offset": intProp("Skip rows for the next page.", 0, queryMaxOffset),
	}
	exportProps := map[string]any{}
	for k, v := range requestProps {
		if k != "limit" && k != "offset" {
			exportProps[k] = v
		}
	}
	exportProps["title"] = strProp("Short file title, e.g. \"مبيعات 2025 حسب العميل\".")
	exportProps["format"] = enumProp("File format, default xlsx.", "xlsx", "csv")

	return []Tool{
		{
			Name: "describe_data",
			Description: "List the datasets this user can query, or describe one dataset's fields, types and known values. " +
				"Call it before query_data when unsure which dataset or field answers the question.",
			Params: objectSchema(map[string]any{
				"dataset": strProp("Optional dataset name to describe in full."),
			}),
			Scopes: allScopes, Permissions: anyGate, Timeout: 5 * time.Second,
			Handler: r.describeData,
		},
		{
			Name: "query_data",
			Description: "Query a dataset. Without group_by/metrics it returns rows (default 50, max 200, paged). " +
				"With metrics and optional group_by it returns exact totals computed in the database over every matching row — " +
				"use this for any count, sum, average, ranking or trend instead of adding up rows.",
			Params: objectSchema(requestProps, "dataset"),
			Scopes: allScopes, Permissions: anyGate, Timeout: queryTimeout + 5*time.Second,
			Handler: r.queryData,
		},
		{
			Name:        "get_record",
			Description: "Open one record by the ref handle from a query_data row: every field, plus related rows (e.g. an order's lines, shipments and status history).",
			Params: objectSchema(map[string]any{
				"ref": strProp("The ref value from a previous result."),
			}, "ref"),
			Scopes: allScopes, Permissions: anyGate, Timeout: 20 * time.Second,
			Handler: r.getRecord,
		},
		{
			Name: "export_data",
			Description: "Export a query to a spreadsheet the user downloads (up to 50,000 rows). Use when the user asks for a file, " +
				"a full list, or more rows than fit in a reply. The file appears under the answer; do not paste its contents.",
			Params: objectSchema(exportProps, "dataset"),
			Scopes: allScopes, Permissions: anyGate, Timeout: exportTimeout + 10*time.Second,
			Handler: r.exportData,
		},
	}
}

func (r *Registry) dataReady() bool { return r.datasets != nil && r.datasetRunner != nil }

func (r *Registry) describeData(_ context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	if !r.dataReady() {
		return Result{Note: "مصدر البيانات غير متاح حالياً."}, nil
	}
	var args struct {
		Dataset string `json:"dataset"`
	}
	if err := decode(raw, &args); err != nil {
		return Result{}, err
	}
	today := timeutil.Now().Format("2006-01-02")
	if name := strings.TrimSpace(args.Dataset); name != "" {
		d, ok := r.datasets.For(actor, name)
		if !ok {
			return Result{}, badArgs("لا توجد مجموعة بيانات بهذا الاسم متاحة لك. استدعِ describe_data بدون معطيات لعرض المتاح.")
		}
		out := datasets.Describe(actor, d)
		out["today"] = today
		return Result{Data: out, Rows: 1}, nil
	}
	list := r.datasets.Available(actor)
	items := make([]map[string]string, 0, len(list))
	for _, d := range list {
		items = append(items, map[string]string{"dataset": d.Name, "label": d.Label, "description": d.Description})
	}
	return Result{Data: map[string]any{"today": today, "datasets": items}, Rows: len(items)}, nil
}

func (r *Registry) compile(actor authctx.Actor, req datasets.Request, opts datasets.Options) (*datasets.Plan, error) {
	d, ok := r.datasets.For(actor, strings.TrimSpace(req.Dataset))
	if !ok {
		return nil, badArgs("لا توجد مجموعة بيانات باسم %q متاحة لك. استدعِ describe_data لعرض المتاح.", req.Dataset)
	}
	opts.ResolveRef = func(kind handles.Kind, token string) (int64, error) {
		return r.resolveHandle(actor, kind, token)
	}
	plan, err := datasets.Compile(actor, d, req, opts)
	switch {
	case err == nil:
		return plan, nil
	case errors.Is(err, datasets.ErrHandle):
		return nil, errBadHandle
	case errors.Is(err, datasets.ErrInvalid):
		return nil, fmt.Errorf("%w: %s", errBadArgs, strings.TrimPrefix(err.Error(), datasets.ErrInvalid.Error()+": "))
	}
	return nil, err
}

func (r *Registry) queryData(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	if !r.dataReady() {
		return Result{Note: "مصدر البيانات غير متاح حالياً."}, nil
	}
	var req datasets.Request
	if err := decode(raw, &req); err != nil {
		return Result{}, err
	}
	opts := datasets.Options{MaxLimit: queryRowLimit, DefaultLimit: queryRowDefault, MaxOffset: queryMaxOffset}
	if req.Aggregate() {
		opts.MaxLimit, opts.DefaultLimit = queryGroupLimit, queryGroupDefault
	}
	plan, err := r.compile(actor, req, opts)
	if err != nil {
		return Result{}, err
	}
	res, err := r.datasetRunner.RunDataset(ctx, plan, queryTimeout)
	if err != nil {
		return Result{}, err
	}
	table, entities := r.table(actor, plan, res)
	table["dataset"] = plan.Dataset.Name
	table["total"] = res.Total
	table["returned"] = len(res.Rows)
	if next := plan.Offset + len(res.Rows); next < res.Total {
		table["has_more"] = true
		table["next_offset"] = next
	}
	if len(res.Rows) == 0 {
		return Result{Data: table, Note: "لا توجد صفوف مطابقة لهذه الشروط."}, nil
	}
	return Result{Data: table, Rows: len(res.Rows), Entities: entities}, nil
}

// table renders a result columnar — the names once, then value arrays — which
// fits several times more rows into the same bytes than one object per row.
// Record ids become handles in a leading "ref" column; ref fields become
// handles in place.
func (r *Registry) table(actor authctx.Actor, plan *datasets.Plan, res *datasets.Result) (map[string]any, []assistant.Entity) {
	names := make([]string, 0, len(plan.Columns)+1)
	if plan.HasKey {
		names = append(names, "ref")
	}
	labelAt := -1
	for i, c := range plan.Columns {
		names = append(names, c.Name)
		if plan.Dataset.Key != nil && c.Name == plan.Dataset.Key.LabelField {
			labelAt = i
		}
	}

	var entities []assistant.Entity
	rows := make([][]any, len(res.Rows))
	for i, values := range res.Rows {
		row := make([]any, 0, len(names))
		if plan.HasKey {
			row = append(row, r.issue(actor, plan.Dataset.Key.Kind, res.Keys[i]))
		}
		for j, c := range plan.Columns {
			v := values[j]
			if c.Type == datasets.Ref && v != nil {
				v = r.refHandle(actor, c.RefKind, v)
			}
			row = append(row, v)
		}
		rows[i] = row
		if plan.HasKey && plan.Dataset.Key.Entity != "" && labelAt >= 0 {
			if label, ok := values[labelAt].(string); ok {
				if e := assistant.RecordEntity(assistant.EntityKind(plan.Dataset.Key.Entity), res.Keys[i], label); e.ID > 0 {
					entities = append(entities, e)
				}
			}
		}
	}
	return map[string]any{"columns": names, "rows": rows}, entities
}

func (r *Registry) refHandle(actor authctx.Actor, kind handles.Kind, v any) any {
	n, ok := v.(json.Number)
	if !ok {
		return nil
	}
	id, err := n.Int64()
	if err != nil || id <= 0 {
		return nil
	}
	return r.issue(actor, kind, id)
}

func (r *Registry) getRecord(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	if !r.dataReady() {
		return Result{Note: "مصدر البيانات غير متاح حالياً."}, nil
	}
	var args struct {
		Ref string `json:"ref"`
	}
	if err := decode(raw, &args); err != nil {
		return Result{}, err
	}
	kind, ok := handles.KindOf(args.Ref)
	if !ok {
		return Result{}, errBadHandle
	}
	d, ok := r.datasets.ForKey(actor, kind)
	if !ok {
		return Result{}, errBadHandle
	}
	id, err := r.resolveHandle(actor, kind, args.Ref)
	if err != nil {
		return Result{}, err
	}

	var fields []string
	for i := range d.Fields {
		if datasets.Visible(actor, &d.Fields[i]) {
			fields = append(fields, d.Fields[i].Name)
		}
	}
	plan, err := r.compile(actor, datasets.Request{Dataset: d.Name, Fields: fields},
		datasets.Options{MaxLimit: 1, DefaultLimit: 1, KeyID: id})
	if err != nil {
		return Result{}, err
	}
	res, err := r.datasetRunner.RunDataset(ctx, plan, queryTimeout)
	if err != nil {
		return Result{}, err
	}
	if len(res.Rows) == 0 {
		return Result{Note: "السجل غير موجود أو لم يعد ضمن صلاحياتك."}, nil
	}
	table, entities := r.table(actor, plan, res)
	record := map[string]any{"dataset": d.Name}
	cols, row := table["columns"].([]string), table["rows"].([][]any)[0]
	for i, name := range cols {
		record[name] = row[i]
	}

	related := map[string]any{}
	rows := 1
	for _, child := range r.datasets.Children(actor, kind) {
		cp, err := r.compile(actor, datasets.Request{Dataset: child.Name},
			datasets.Options{MaxLimit: relatedRowLimit, DefaultLimit: relatedRowLimit, ParentKind: kind, ParentID: id})
		if err != nil {
			return Result{}, err
		}
		cr, err := r.datasetRunner.RunDataset(ctx, cp, queryTimeout)
		if err != nil {
			return Result{}, err
		}
		if len(cr.Rows) == 0 {
			continue
		}
		ct, ce := r.table(actor, cp, cr)
		ct["total"] = cr.Total
		related[child.Name] = ct
		rows += len(cr.Rows)
		entities = append(entities, ce...)
	}
	out := map[string]any{"record": record}
	if len(related) > 0 {
		out["related"] = related
	}
	return Result{Data: out, Rows: rows, Entities: entities}, nil
}

func (r *Registry) exportData(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	if !r.dataReady() || r.exports == nil {
		return Result{Note: "تصدير الملفات غير متاح حالياً."}, nil
	}
	var args struct {
		datasets.Request
		Title  string `json:"title"`
		Format string `json:"format"`
	}
	if err := decode(raw, &args); err != nil {
		return Result{}, err
	}
	format, err := oneOf("format", args.Format, "xlsx", "csv")
	if err != nil {
		return Result{}, err
	}
	if format == "" {
		format = "xlsx"
	}
	req := args.Request
	req.Limit, req.Offset = 0, 0
	plan, err := r.compile(actor, req, datasets.Options{MaxLimit: exportRowLimit, DefaultLimit: exportRowLimit})
	if err != nil {
		return Result{}, err
	}
	res, err := r.datasetRunner.RunDataset(ctx, plan, exportTimeout)
	if err != nil {
		return Result{}, err
	}
	if len(res.Rows) == 0 {
		return Result{Note: "لا توجد صفوف مطابقة لتصديرها."}, nil
	}

	title := exportTitle(args.Title, plan.Dataset.Label)
	file, err := assistant.BuildExport(title, format, plan.Columns, res.Rows)
	if err != nil {
		return Result{}, err
	}
	token, err := r.exports.SaveExport(ctx, actor, file)
	if err != nil {
		return Result{}, err
	}
	entity := assistant.ExportEntity(file, token)
	data := map[string]any{
		"file":      file.Filename,
		"rows":      len(res.Rows),
		"truncated": res.Total > len(res.Rows),
		"note":      "الملف مرفق أسفل الرد للتحميل. لا تعِد كتابة محتواه.",
	}
	return Result{Data: data, Rows: len(res.Rows), Entities: []assistant.Entity{entity}}, nil
}

func exportTitle(requested, fallback string) string {
	t := strings.TrimSpace(requested)
	if t == "" {
		t = fallback
	}
	if r := []rune(t); len(r) > 60 {
		t = string(r[:60])
	}
	return t
}
