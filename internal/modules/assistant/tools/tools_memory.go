package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// MemoryStore defines the repository capabilities needed by memory tools.
type MemoryStore interface {
	SaveMemory(ctx context.Context, mem *assistant.Memory) error
	DeleteMemory(ctx context.Context, orgID, memoryID int64) error
	ListMemories(ctx context.Context, orgID int64, userID *int64, limit int) ([]*assistant.Memory, error)
}

func memoryTools(r *Registry) []Tool {
	return []Tool{
		{
			Name:        "memory_remember",
			Description: "تذكر وحفظ معلومة أو قاعدة أو تفضيل ثابت للمنشأة في الذاكرة الدائمة لاسترجاعها في المحادثات المستقبلية عبر كافة الجلسات.",
			Params: objectSchema(map[string]any{
				"content": strProp("المعلومة أو القاعدة أو التفضيل المطلوب حفظه بشكل واضح وموجز."),
				"category": enumProp("تصنيف المعلومة للمنشأة.",
					string(assistant.MemoryCategoryGeneral),
					string(assistant.MemoryCategoryProfile),
					string(assistant.MemoryCategoryProcurement),
					string(assistant.MemoryCategoryFinancial),
					string(assistant.MemoryCategoryLogistics),
					string(assistant.MemoryCategoryPreferences),
					string(assistant.MemoryCategoryContacts),
				),
			}, "content"),
			Scopes:      tradingScopes,
			Permissions: []string{assistant.GatePharmacy, assistant.GateVendor},
			Handler:     r.memoryRemember,
		},
		{
			Name:        "memory_forget",
			Description: "نسيان أو إلغاء تفعيل معلومة سابقة من ذاكرة المنشأة بناءً على طلب صريح من المستخدم.",
			Params: objectSchema(map[string]any{
				"memory_id": intProp("معرف المعلومة (ID) المطلوب نسيانها وإلغاؤها.", 1, 2147483647),
			}, "memory_id"),
			Scopes:      tradingScopes,
			Permissions: []string{assistant.GatePharmacy, assistant.GateVendor},
			Handler:     r.memoryForget,
		},
		{
			Name:        "memory_list",
			Description: "استعراض قائمة الحقائق والقواعد المخزنة في ذاكرة المنشأة الدائمة للتأكد منها أو مراجعتها.",
			Params:      objectSchema(nil),
			Scopes:      tradingScopes,
			Permissions: []string{assistant.GatePharmacy, assistant.GateVendor},
			Handler:     r.memoryList,
		},
	}
}

type memoryRememberArgs struct {
	Content  string `json:"content"`
	Category string `json:"category,omitempty"`
}

func (r *Registry) memoryRemember(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	if r.memories == nil {
		return Result{Note: "خدمة الذاكرة غير متاحة حالياً."}, nil
	}
	if actor.OrgID <= 0 {
		return Result{Note: "ذاكرة المنشأة متاحة فقط لحسابات المنشآت التجارية."}, nil
	}
	var args memoryRememberArgs
	if err := decode(raw, &args); err != nil {
		return Result{}, err
	}
	content := strings.TrimSpace(args.Content)
	if content == "" {
		return Result{}, badArgs("حقل content لا يمكن أن يكون فارغاً.")
	}
	if len(content) > 1000 {
		content = content[:1000]
	}

	cat := assistant.MemoryCategoryGeneral
	if args.Category != "" {
		cat = assistant.MemoryCategory(args.Category)
	}

	mem := &assistant.Memory{
		OrganizationID: actor.OrgID,
		Scope:          assistant.MemoryScopeOrganization,
		Category:       cat,
		Content:        content,
		Source:         assistant.MemorySourceUserExplicit,
		IsActive:       true,
	}

	if err := r.memories.SaveMemory(ctx, mem); err != nil {
		r.log.Error("failed to save memory via tool", "org_id", actor.OrgID, "error", err)
		return Result{}, err
	}

	return Result{
		Data: map[string]any{
			"status":    "remembered",
			"memory_id": mem.ID,
			"content":   mem.Content,
			"category":  string(mem.Category),
		},
		Rows: 1,
	}, nil
}

type memoryForgetArgs struct {
	MemoryID int64 `json:"memory_id"`
}

func (r *Registry) memoryForget(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	if r.memories == nil {
		return Result{Note: "خدمة الذاكرة غير متاحة حالياً."}, nil
	}
	if actor.OrgID <= 0 {
		return Result{Note: "ذاكرة المنشأة متاحة فقط لحسابات المنشآت التجارية."}, nil
	}
	var args memoryForgetArgs
	if err := decode(raw, &args); err != nil {
		return Result{}, err
	}
	if args.MemoryID <= 0 {
		return Result{}, badArgs("memory_id يجب أن يكون رقماً موجباً.")
	}

	if err := r.memories.DeleteMemory(ctx, actor.OrgID, args.MemoryID); err != nil {
		r.log.Error("failed to forget memory via tool", "org_id", actor.OrgID, "memory_id", args.MemoryID, "error", err)
		return Result{}, err
	}

	return Result{
		Data: map[string]any{
			"status":    "forgotten",
			"memory_id": args.MemoryID,
		},
		Rows: 1,
	}, nil
}

func (r *Registry) memoryList(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	if r.memories == nil {
		return Result{Note: "خدمة الذاكرة غير متاحة حالياً."}, nil
	}
	if actor.OrgID <= 0 {
		return Result{Note: "ذاكرة المنشأة متاحة فقط لحسابات المنشآت التجارية."}, nil
	}
	var args struct{}
	if err := decode(raw, &args); err != nil {
		return Result{}, err
	}

	var uidPtr *int64
	if actor.UserID > 0 {
		uid := actor.UserID
		uidPtr = &uid
	}

	list, err := r.memories.ListMemories(ctx, actor.OrgID, uidPtr, 30)
	if err != nil {
		r.log.Error("failed to list memories via tool", "org_id", actor.OrgID, "error", err)
		return Result{}, err
	}

	if len(list) == 0 {
		return Result{Note: "لا توجد ذكريات أو قواعد مسجلة لهذه المنشأة حالياً."}, nil
	}

	type memItem struct {
		ID       int64  `json:"id"`
		Category string `json:"category"`
		Scope    string `json:"scope"`
		Content  string `json:"content"`
	}
	items := make([]memItem, len(list))
	for i, m := range list {
		items[i] = memItem{
			ID:       m.ID,
			Category: string(m.Category),
			Scope:    string(m.Scope),
			Content:  m.Content,
		}
	}

	return Result{
		Data: map[string]any{
			"memories": items,
			"count":    len(items),
		},
		Rows: len(items),
	}, nil
}
