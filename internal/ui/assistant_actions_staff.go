package ui

import (
	"context"
	"fmt"
	"strconv"

	"github.com/muhiya/dawa24-store/internal/modules/assistant/actions"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/datasets"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Platform staff commands: organisation approvals and reported issues.

func (h *UIHandler) staffCommands() []assistantCommand {
	return []assistantCommand{
		organizationDecision(h, true),
		organizationDecision(h, false),
		{
			def: actions.Definition{
				Name: "issue_update", Label: "تحديث بلاغ", Risk: actions.RiskLow,
				Description: "Set a reported issue's status (pending, in_progress, resolved) with a response the reporter receives. issue is a ref from support_issues.",
				Params: []actions.Param{
					{Name: "issue", Type: actions.ParamRef, RefKind: datasets.KindIssue, Required: true},
					{Name: "status", Type: actions.ParamEnum, Values: []string{"pending", "in_progress", "resolved"}, Required: true},
					{Name: "response", Type: actions.ParamString, MaxLen: 1000},
				},
			},
			audience: audienceStaff, keys: []string{"workflow.request.update", "workflow.issue.update"},
			prepare: func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Preview, error) {
				if h.wfSvc == nil {
					return actions.Preview{}, actions.Refuse("خدمة البلاغات غير متاحة حالياً.")
				}
				issue, err := h.wfSvc.GetIssueByID(ctx, args.ID("issue"))
				if err != nil || issue == nil {
					return actions.Preview{}, actions.Refuse("لم يتم العثور على البلاغ المطلوب.")
				}
				p := actions.Preview{
					Title:   "تحديث بلاغ",
					Summary: fmt.Sprintf("من %s إلى %s", issue.Status, args.Str("status")),
					Details: []actions.Detail{{Label: "البلاغ", Value: truncateRunes(issue.Description, 160)}},
				}
				if resp := args.Str("response"); resp != "" {
					p.Details = append(p.Details, actions.Detail{Label: "الرد", Value: resp})
				}
				p.Warnings = []string{"سيُبلَّغ صاحب البلاغ بالرد."}
				return p, nil
			},
			execute: func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Outcome, error) {
				if msg := h.updateReportedIssue(ctx, args.ID("issue"), args.Str("status"), args.Str("response")); msg != "" {
					return actions.Outcome{}, refuseMessage(msg)
				}
				return actions.Outcome{Message: "تم تحديث البلاغ وإبلاغ صاحبه.", URL: "/admin/report-issues"}, nil
			},
		},
	}
}

func organizationDecision(h *UIHandler, approve bool) assistantCommand {
	name, label := "organization_approve", "اعتماد منشأة"
	if !approve {
		name, label = "organization_reject", "رفض منشأة"
	}
	load := func(ctx context.Context, id int64) (*org.Organization, error) {
		if h.orgSvc == nil {
			return nil, actions.Refuse("خدمة المنشآت غير متاحة حالياً.")
		}
		o, err := h.orgSvc.GetOrganization(database.AsSystem(ctx), id)
		if err != nil || o == nil {
			return nil, actions.Refuse("المنشأة غير موجودة.")
		}
		if o.Status != org.StatusPending {
			return nil, actions.Refuse("حالة المنشأة الآن %s، والقرار متاح فقط للمنشآت قيد المراجعة.", o.Status)
		}
		return o, nil
	}
	return assistantCommand{
		def: actions.Definition{
			Name: name, Label: label, Risk: actions.RiskHigh,
			Description: label + " pending registration. organization is a ref from the organizations dataset.",
			Params:      []actions.Param{{Name: "organization", Type: actions.ParamRef, RefKind: handles.KindOrgUnit, Required: true}},
		},
		audience: audienceStaff, keys: []string{"org.approval.decide"},
		prepare: func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Preview, error) {
			o, err := load(ctx, args.ID("organization"))
			if err != nil {
				return actions.Preview{}, err
			}
			name := o.TradeName.Get(i18n.AR)
			if name == "" {
				name = o.LegalName
			}
			p := actions.Preview{Title: label + ": " + name, Summary: fmt.Sprintf("النوع %s — رقم %s", o.Type, o.OrganizationNumber)}
			if approve {
				p.Warnings = []string{"تفعيل الحساب وتوثيق مستنداته وتجهيز اشتراكه، مع إشعار المالك."}
			}
			return p, nil
		},
		execute: func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Outcome, error) {
			id := args.ID("organization")
			if _, err := load(ctx, id); err != nil {
				return actions.Outcome{}, err
			}
			if approve {
				if err := h.approveOrganization(ctx, actor, id); err != nil {
					return actions.Outcome{}, refuseMessage(h.safeMessage(err, "ar"))
				}
				return actions.Outcome{Message: "تم اعتماد المنشأة.", URL: "/admin/organizations/" + strconv.FormatInt(id, 10)}, nil
			}
			if err := h.orgSvc.RejectOrganization(ctx, id); err != nil {
				return actions.Outcome{}, refuseMessage(h.safeMessage(err, "ar"))
			}
			return actions.Outcome{Message: "تم رفض المنشأة.", URL: "/admin/organizations/" + strconv.FormatInt(id, 10)}, nil
		},
	}
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
