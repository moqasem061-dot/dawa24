package tools

import (
	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

var vendorScope = []rbac.Scope{rbac.ScopeVendor}

const permVendorOrder = "vendor.order.view"

func vendorStage3Tools(r *Registry) []Tool {
	return []Tool{
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionSalesInsights, key: "sales_insights",
			description: "تحليلات المبيعات ونمو الإيرادات: نمو المبيعات مقارنة بالـ 30 يوماً السابقة، نسبة إنجاز وتسليم الشحنات، متوسط قيمة الشحنة، وعدد الصيدليات النشطة.",
			scopes:      vendorScope, permissions: []string{permVendorOrder},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionInventoryHealth, key: "inventory_health",
			description: "تقييم وتحليل صحة المخزون: القيمة المالية التقديرية للمخزون بالجنيه، عدد الأصناف النافذة، الأصناف الحرجة دون حد الأمان، وإجمالي الوحدات بالمستودعات.",
			scopes:      vendorScope, permissions: []string{"vendor.inventory.view"},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionQuotaReport, key: "quota_report",
			description: "تقرير كوتة الأصناف: الحصص المخصصة للفروع، الكميات المستهلكة في الطلبات، والكميات المتبقية.",
			scopes:      vendorScope, permissions: []string{"vendor.quota.view"},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionImportRuns, key: "import_runs_list",
			description: "سجل عمليات استيراد الكتالوج وقوائم الأسعار: الملف، عدد السطور، السطور المدخلة، والمحدثة، والأخطاء.",
			scopes:      vendorScope, permissions: []string{"vendor.ingest.view"}, handleKind: handles.KindImportRun, handleField: "run",
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionImportRunDetails, key: "import_run_details",
			description: "تفاصيل عملية استيراد كتالوج محددة: مراجعة السطور الناجحة، المعلقة للمراجعة، أو التي بها أخطاء.",
			scopes:      vendorScope, permissions: []string{"vendor.ingest.view"}, handleKind: handles.KindImportRun, handleField: "run", detail: true,
		}, projectionDetailSchema("run")),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionSponsorshipStatus, key: "sponsorship_status",
			description: "باقات الرعاية والإعلانات المميزة: رصيد الرعاية المستخدم والمتبقي وفترة الفعالية.",
			scopes:      vendorScope, permissions: []string{"vendor.offer_package.view"},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionBatchExpiryReport, key: "batch_expiry_report",
			description: "تقرير دفعات الأدوية وتواريخ الصلاحية بالمستودعات: الأصناف القريبة من انتهاء الصلاحية (Near Expiry)، ورقم التشغيلة/الدفعة (Batch Number)، وتوزيع الكميات.",
			scopes:      vendorScope, permissions: []string{"vendor.inventory.view"},
			handleKind: handles.KindProduct, handleField: "product",
		}, projectionListSchema(map[string]any{
			"search": strProp("اسم الصنف أو رقم التشغيلة/الدفعة."),
			"status": enumProp("تصفية الصلاحية.", "near_expiry"),
		})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionDispatchSchedule, key: "dispatch_schedule",
			description: "جدول التوزيع والشحنات الجاهزة للتسليم: أوامر التوريد الجاهزة للشحن أو التسليم ومناطق التوصيل وحالة الشحن.",
			scopes:      vendorScope, permissions: []string{permVendorOrder},
			handleKind: handles.KindShipment, handleField: "shipment",
		}, projectionListSchema(map[string]any{
			"search": strProp("رقم الشحنة أو رقم الطلب أو اسم الصيدلية."),
			"status": enumProp("حالة الشحنة.", "confirmed", "processing", "ready_for_pickup", "shipped"),
		})),
	}
}
