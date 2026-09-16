package tools

import (
	"context"
	"encoding/json"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// Tools both trading dashboards share.
//
// They carry the permission keys of BOTH dashboards because Permissions is an
// any-of check and the scope gate runs first. A pharmacy user is admitted by
// pharmacy.branch.view and a vendor by vendor.branch.view; neither can be
// admitted by the other's key, because the permission resolver strips
// out-of-scope grants before an actor is ever built.

var tradingScopes = []rbac.Scope{rbac.ScopePharmacy, rbac.ScopeVendor}

func sharedTools(r *Registry) []Tool {
	return []Tool{
		{
			Name:        "wallet_summary",
			Description: "رصيد المحفظة الحالي وآخر معاملاتها.",
			Params:      objectSchema(nil),
			Scopes:      tradingScopes,
			Permissions: []string{"pharmacy.wallet.view", "vendor.wallet.view"},
			Handler:     r.walletSummary,
		},
		{
			Name:        "subscription_status",
			Description: "الاشتراك: الباقة والحالة وتاريخ الانتهاء والأيام المتبقية.",
			Params:      objectSchema(nil),
			Scopes:      tradingScopes,
			Permissions: []string{"pharmacy.subscription.view", "vendor.subscription.view"},
			Handler:     r.subscriptionStatus,
		},
		{
			Name:        "account_profile",
			Description: "الملف التعريفي للمنشأة والحساب: الاسم الرسمي، السجل التجاري، الرقم الضريبي، بيانات الاتصال، عدد الفروع، ودور المستخدم.",
			Params:      objectSchema(nil),
			Scopes:      tradingScopes,
			Permissions: []string{"pharmacy.branch.view", "vendor.branch.view", "pharmacy.dashboard.view", "vendor.dashboard.view"},
			Handler:     r.accountProfile,
		},
	}
}

func (r *Registry) walletSummary(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	var args struct{}
	if err := decode(raw, &args); err != nil {
		return Result{}, err
	}
	summary, err := r.reader.Wallet(ctx, actor)
	if err != nil {
		return Result{}, err
	}
	if summary == nil {
		return Result{Note: "لا توجد محفظة مفعّلة لهذا الحساب."}, nil
	}
	return Result{Data: summary, Rows: len(summary.Recent)}, nil
}

func (r *Registry) subscriptionStatus(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	var args struct{}
	if err := decode(raw, &args); err != nil {
		return Result{}, err
	}
	sub, err := r.reader.Subscription(ctx, actor)
	if err != nil {
		return Result{}, err
	}
	if sub == nil {
		return Result{Note: "لا يوجد اشتراك نشط لهذه المنشأة."}, nil
	}
	return Result{Data: sub, Rows: 1}, nil
}

func (r *Registry) accountProfile(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	var args struct{}
	if err := decode(raw, &args); err != nil {
		return Result{}, err
	}
	if r.projections == nil {
		return Result{Note: "بيانات ملف الحساب غير متاحة حالياً."}, nil
	}
	pageResult, err := r.projections.ReadProjection(ctx, actor, assistant.ProjectionQuery{
		Kind:  assistant.ProjectionAccountProfile,
		Limit: 1,
	})
	if err != nil {
		return Result{}, err
	}
	if len(pageResult.Rows) == 0 {
		return Result{Note: "لم يتم العثور على بيانات المنشأة."}, nil
	}
	return Result{Data: pageResult.Rows[0].Values, Rows: 1}, nil
}

// page is the shared shape every listing tool returns, so the model learns one
// pagination convention instead of a different one per tool.
func page[T any](p assistant.Page[T], key string) Result {
	if len(p.Rows) == 0 {
		return Result{Note: "لا توجد نتائج مطابقة."}
	}
	data := map[string]any{key: p.Rows, "count": len(p.Rows)}
	if p.Total > 0 {
		data["total_matching"] = p.Total
	}
	if p.HasMore {
		data["has_more"] = true
		data["next_offset"] = p.NextOffset
	}
	return Result{Data: data, Rows: len(p.Rows)}
}
