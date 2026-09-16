package tools

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// "Who delivers to me, and when."
//
// The question a pharmacy asks before it asks anything else, and the one the
// assistant answered worst: nothing in the read model knew about coverage, so
// the model either declined or — worse — reasoned from a supplier list and
// named one that cannot reach the branch.
//
// The answer comes from the platform's single coverage rule, reached through
// the probe the assistant declares and the composition root supplies. When no
// probe is wired the tool says the check is unavailable rather than guessing:
// the failure mode this replaces is a confident wrong answer, and returning to
// it on a wiring slip would defeat the purpose.

// coverageTools declares the coverage tool, for any buyer: a pharmacy and a
// supplier that buys both ask who delivers to their receiving branch. Which
// branches a supplier's own coverage reaches is its delivery_coverage dataset.
func coverageTools(r *Registry) []Tool {
	return []Tool{
		{
			Name: "coverage_check",
			Description: "من الموردين الذين يغطون فرعك ويستطيعون التوصيل إليه، " +
				"مع مواعيد التغطية في يوم محدد. لسؤال «مين بيوصّل لفرعي» أو «إمتى يوصلني».",
			Params: objectSchema(map[string]any{
				"day": enumProp("يوم الأسبوع المطلوب. الافتراضي اليوم الحالي.",
					"sunday", "monday", "tuesday", "wednesday",
					"thursday", "friday", "saturday"),
				"branch": strProp(
					"مرجع الفرع (ref) من query_data على branches. " +
						"اتركه فارغاً لاستخدام الفرع المحدد في الجلسة."),
			}),
			Scopes:      buyingScopes,
			Permissions: buyingKeys(rbac.BuyCatalogView, rbac.BuyOrderView),
			Timeout:     15 * time.Second,
			Handler:     r.coverageCheck,
		},
	}
}

func (r *Registry) coverageCheck(
	ctx context.Context, actor authctx.Actor, raw json.RawMessage,
) (Result, error) {
	var args struct {
		Day    string `json:"day,omitempty"`
		Branch string `json:"branch,omitempty"`
	}
	if err := decode(raw, &args); err != nil {
		return Result{}, err
	}

	day := time.Now().Weekday()
	if strings.TrimSpace(args.Day) != "" {
		parsed, ok := assistant.WeekdayFromArabic(strings.ToLower(strings.TrimSpace(args.Day)))
		if !ok {
			return Result{}, badArgs("قيمة day ليست يوماً من أيام الأسبوع.")
		}
		day = parsed
	}

	// The branch comes from a verified handle or from the session, never from a
	// bare id. A pharmacy asking about "branch 76" must not be able to learn
	// anything about a branch that is not theirs, and the handle is what makes
	// that structural rather than a WHERE clause somebody has to remember.
	branchID := int64(0)
	if strings.TrimSpace(args.Branch) != "" {
		id, err := r.resolveHandle(actor, handles.KindBranch, args.Branch)
		if err != nil {
			return Result{}, err
		}
		branchID = id
	} else if actor.BranchID != nil {
		branchID = *actor.BranchID
	}
	if branchID <= 0 {
		return Result{Note: "لم يُحدَّد فرع. اختر الفرع أولاً أو اذكره من قائمة الفروع."}, nil
	}

	if r.coverage == nil {
		// Say the check is unavailable. Answering "nobody covers you" here
		// would be indistinguishable from the real thing and completely wrong.
		return Result{Note: "خدمة فحص التغطية غير متاحة حالياً."}, nil
	}

	answer, err := r.coverage.VendorsServingBranch(ctx, branchID, day)
	if err != nil {
		return Result{}, err
	}
	if !answer.Evaluated {
		return Result{Note: "تعذّر تقييم التغطية لهذا الفرع. تأكد من تحديد موقع الفرع أو مدينته."}, nil
	}
	// A supplier buying is not delivered to by itself.
	vendors := answer.Vendors[:0]
	for _, v := range answer.Vendors {
		if v.ID != actor.OrganizationID {
			vendors = append(vendors, v)
		}
	}
	answer.Vendors = vendors
	if len(answer.Vendors) == 0 {
		return Result{
			Note: "لا يوجد مورّد يغطي هذا الفرع في اليوم المطلوب. " +
				"جرّب يوماً آخر أو راجع بيانات موقع الفرع.",
		}, nil
	}

	data := map[string]any{
		"day":       strings.ToLower(day.String()),
		"suppliers": answer.Vendors,
		"count":     len(answer.Vendors),
	}
	if answer.BranchName != "" {
		data["branch"] = answer.BranchName
	}
	return Result{Data: data, Rows: len(answer.Vendors)}, nil
}
