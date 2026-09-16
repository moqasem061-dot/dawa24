package tools

import (
	"context"
	"encoding/json"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// Platform staff tools.
//
// These are the only tools whose queries cross tenant boundaries, and they are
// deliberately few and deliberately coarse: counts, registration records, and
// AI consumption. There is no tool here that reads one tenant's order book,
// prices or documents — a staff member who needs that opens the screen, where
// the access is attributable to a page view rather than to a paraphrase.
//
// Each is gated on the specific admin permission the equivalent screen needs,
// held by THIS admin. "Is staff" is not a permission and does not admit
// anything here; the platform assistant gate (platform.assistant.use) admits
// the assistant, and each tool then asks for its own key on top.

var adminScope = []rbac.Scope{rbac.ScopeAdmin}

func adminTools(r *Registry) []Tool {
	return []Tool{
		{
			Name:        "platform_overview",
			Description: "مؤشرات المنصة العامة والرئيسية: إجمالي المنشآت، الصيدليات، الموردين، طلبات الاعتماد المعلقة، المستخدمين، إجمالي الطلبات وحجم التداول (GMV) للفترة المحددة والإجمالي الكلي.",
			Params:      objectSchema(dateProps(nil)),
			Scopes:      adminScope,
			Permissions: []string{"platform.dashboard.view"},
			Handler:     r.platformOverview,
		},
	}
}

func (r *Registry) platformOverview(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	var args dateRangeArgs
	if err := decode(raw, &args); err != nil {
		return Result{}, err
	}
	rng, err := args.parse(30)
	if err != nil {
		return Result{}, err
	}
	summary, err := r.reader.PlatformOverview(ctx, actor, rng)
	if err != nil {
		return Result{}, err
	}
	if summary == nil {
		return Result{Note: "لا تتوفر مؤشرات لهذه الفترة."}, nil
	}
	return Result{Data: summary, Rows: 1}, nil
}
