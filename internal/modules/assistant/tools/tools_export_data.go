package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/assistant"
	"github.com/moqasem061-dot/dawa24/internal/modules/assistant/datasets"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

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
