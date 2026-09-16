package tools

import (
	"github.com/muhiya/dawa24-store/internal/modules/assistant"
)

func adminStage3Tools(r *Registry) []Tool {
	return []Tool{
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionApprovals, key: "approvals_pending",
			description: "طلبات اعتماد المنشآت والوثائق المعلقة: المنشآت بانتظار المراجعة والتدقيق الإداري والترخيص.",
			scopes:      adminScope, permissions: []string{"org.approval.view"},
		}, projectionListSchema(map[string]any{"search": strProp("اسم المنشأة المطلوب فحصها.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionDeletionRequests, key: "deletion_requests",
			description: "طلبات حذف وتعطيل الحسابات: مراجعة طلبات المنشآت الراغبة في حذف حساباتها وأسبابها.",
			scopes:      adminScope, permissions: []string{"org.organization.view"},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionFinance, key: "finance_overview",
			description: "المؤشرات المالية العامة للمنصة: إجمالي أرصدة المحافظ، الفواتير، المدفوعات، وطلبات السحب المعلقة.",
			scopes:      adminScope, permissions: []string{"billing.finance.view", "billing.wallet.read", "billing.invoice.view"},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionVisitors, key: "visitors_report",
			description: "تقرير الزيارات والتحليلات: إجمالي الزيارات، المدن الأكثر نشاطاً، والزوار الفريدين.",
			scopes:      adminScope, permissions: []string{"platform.analytics.view"},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionHealth, key: "platform_health",
			description: "مؤشرات الجاهزية وصحة المنظومة: معدل الأخطاء في آخر 24 ساعة، طوابير المهام الخلفية، وحالة التشغيل.",
			scopes:      adminScope, permissions: []string{"platform.dashboard.view"},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionMatchDecisions, key: "match_decisions_search",
			description: "ذاكرة مطابقة الكتالوج: قرارات المطابقة الآلية للأدوية ومستوى الثقة وسجل الاستخدام.",
			scopes:      adminScope, permissions: []string{"catalog.match_decision.view"},
		}, projectionListSchema(map[string]any{"search": strProp("اسم الصنف أو مفتاح القرار.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionInstitutionalGraph, key: "institutional_graph",
			description: "شبكة ربط الأعمال المؤسسية: ارتباطات الفروع بين المشترين والموردين وشبكات التوريد.",
			scopes:      adminScope, permissions: []string{"org.institutional_work.view"},
		}, projectionListSchema(map[string]any{"search": strProp("اسم العمل أو المنشأة.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionSecurityEvents, key: "platform_security_overview",
			description: "مؤشرات الأمان والأنشطة الرقابية: محاولات تسجيل الدخول الفاشلة، عمليات تدقيق الصلاحيات المرفوضة، وتنبيهات الأنشطة الإدارية.",
			scopes:      adminScope, permissions: []string{"platform.activity_log.view", "platform.dashboard.view"},
		}, projectionListSchema(nil)),
	}
}
