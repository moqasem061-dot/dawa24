package ui

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

var orgLifecycleTestCases = []eventTestCase{
	{
		name: "Trade-name/profile change requested -> admins",
		trigger: func(h *UIHandler, ctx context.Context, adminID, _, _, testOrgID, _ int64) {
			h.notifyProfileChangeRequested(ctx, testOrgID, "البيانات التجارية")
		},
		expectedMinLogs: 1,
		targetUserID:    10,
		expectedPerm:    "admin.organizations.manage",
		titleSnippet:    "تعديل بيانات المنشأة",
	},
	{
		name: "Profile change approved -> org",
		trigger: func(h *UIHandler, ctx context.Context, _, _, _, testOrgID, _ int64) {
			h.notifyProfileChangeDecision(ctx, testOrgID, "البيانات التجارية", true, "")
		},
		expectedMinLogs: 1,
		expectedPerm:    "vendor.organization.view",
		titleSnippet:    "الموافقة على تعديل",
	},
	{
		name: "Profile change rejected -> org",
		trigger: func(h *UIHandler, ctx context.Context, _, _, _, testOrgID, _ int64) {
			h.notifyProfileChangeDecision(ctx, testOrgID, "البيانات التجارية", false, "السجل غير سارٍ")
		},
		expectedMinLogs: 1,
		expectedPerm:    "vendor.organization.view",
		titleSnippet:    "رفض تعديل بيانات",
	},
	{
		name: "Organization deletion requested -> admins",
		trigger: func(h *UIHandler, ctx context.Context, adminID, _, _, testOrgID, _ int64) {
			h.notifyOrgDeletionRequested(ctx, testOrgID, "إغلاق النشاط")
		},
		expectedMinLogs: 1,
		targetUserID:    10,
		expectedPerm:    "admin.organizations.manage",
		titleSnippet:    "حذف منشأة",
	},
	{
		name: "Organization deletion approved -> org owner",
		trigger: func(h *UIHandler, ctx context.Context, _, ownerID, _, testOrgID, _ int64) {
			h.notifyOrgDeletionDecision(ctx, testOrgID, true, "")
		},
		expectedMinLogs: 1,
		targetUserID:    20,
		titleSnippet:    "الموافقة على حذف",
	},
	{
		name: "Organization deletion rejected -> org owner",
		trigger: func(h *UIHandler, ctx context.Context, _, ownerID, _, testOrgID, _ int64) {
			h.notifyOrgDeletionDecision(ctx, testOrgID, false, "مستحقات معلقة")
		},
		expectedMinLogs: 1,
		targetUserID:    20,
		titleSnippet:    "رفض طلب حذف المنشأة",
	},
	{
		name: "Account deletion requested -> admins",
		trigger: func(h *UIHandler, ctx context.Context, adminID, _, staffID, _, _ int64) {
			h.notifyAccountDeletionRequested(ctx, staffID, "رغبة شخصية")
		},
		expectedMinLogs: 1,
		targetUserID:    10,
		expectedPerm:    "admin.users.manage",
		titleSnippet:    "حذف حساب مستخدم",
	},
	{
		name: "Account deletion approved -> user",
		trigger: func(h *UIHandler, ctx context.Context, _, _, staffID, _, _ int64) {
			h.notifyAccountDeletionDecision(ctx, staffID, true, "")
		},
		expectedMinLogs: 1,
		targetUserID:    21,
		titleSnippet:    "الموافقة على حذف حسابك",
	},
	{
		name: "Account deletion rejected -> user",
		trigger: func(h *UIHandler, ctx context.Context, _, _, staffID, _, _ int64) {
			h.notifyAccountDeletionDecision(ctx, staffID, false, "طلبات نشطة")
		},
		expectedMinLogs: 1,
		targetUserID:    21,
		titleSnippet:    "رفض طلب حذف حسابك",
	},
	{
		name: "Organization suspended -> org",
		trigger: func(h *UIHandler, ctx context.Context, _, _, _, testOrgID, _ int64) {
			h.notifyOrgSuspended(ctx, testOrgID, "مخالفة الشروط")
		},
		expectedMinLogs: 1,
		expectedPerm:    "vendor.organization.view",
		titleSnippet:    "إيقاف المنشأة مؤقتاً",
	},
	{
		name: "Organization reactivated -> org",
		trigger: func(h *UIHandler, ctx context.Context, _, _, _, testOrgID, _ int64) {
			h.notifyOrgReactivated(ctx, testOrgID)
		},
		expectedMinLogs: 1,
		expectedPerm:    "vendor.organization.view",
		titleSnippet:    "إعادة تفعيل المنشأة",
	},
	{
		name: "Document requested by admin -> org",
		trigger: func(h *UIHandler, ctx context.Context, _, _, _, testOrgID, _ int64) {
			h.notifyDocumentRequested(ctx, testOrgID, "ترخيص مزاولة المهنة", "مطلوب تحديث", 15)
		},
		expectedMinLogs: 1,
		expectedPerm:    "vendor.document.view",
		titleSnippet:    "طلب مستند رسمي",
	},
	{
		name: "Document rejected -> org",
		trigger: func(h *UIHandler, ctx context.Context, _, _, _, testOrgID, _ int64) {
			h.notifyDocumentRejected(ctx, testOrgID, "ترخيص المزاولة", "صورة غير واضحة")
		},
		expectedMinLogs: 1,
		expectedPerm:    "vendor.document.view",
		titleSnippet:    "رفض المستند",
	},
	{
		name: "Branch created -> org owner",
		trigger: func(h *UIHandler, ctx context.Context, _, ownerID, _, testOrgID, _ int64) {
			h.notifyBranchCreated(ctx, testOrgID, "فرع المعادي", "BR-01")
		},
		expectedMinLogs: 1,
		targetUserID:    20,
		titleSnippet:    "إضافة فرع جديد",
	},
	{
		name: "Branch disabled -> org owner",
		trigger: func(h *UIHandler, ctx context.Context, _, ownerID, _, testOrgID, _ int64) {
			h.notifyBranchDisabled(ctx, testOrgID, "فرع المعادي")
		},
		expectedMinLogs: 1,
		targetUserID:    20,
		titleSnippet:    "تعطيل فرع",
	},
	{
		name: "Branch institutional works changed -> org owner",
		trigger: func(h *UIHandler, ctx context.Context, _, ownerID, _, testOrgID, _ int64) {
			h.notifyBranchInstitutionalWorksChanged(ctx, testOrgID, "فرع المعادي")
		},
		expectedMinLogs: 1,
		targetUserID:    20,
		titleSnippet:    "الأعمال المؤسسية",
	},
	{
		name: "User added to org -> user and owner",
		trigger: func(h *UIHandler, ctx context.Context, _, _, staffID, testOrgID, _ int64) {
			h.notifyUserAddedToOrg(ctx, testOrgID, staffID, "صيدلي مسؤول")
		},
		expectedMinLogs: 2,
		titleSnippet:    "إضافة عضو جديد",
	},
	{
		name: "User removed from org -> user and owner",
		trigger: func(h *UIHandler, ctx context.Context, _, _, staffID, testOrgID, _ int64) {
			h.notifyUserRemovedFromOrg(ctx, testOrgID, staffID)
		},
		expectedMinLogs: 2,
		titleSnippet:    "إزالة عضو",
	},
	{
		name: "User role changed -> user",
		trigger: func(h *UIHandler, ctx context.Context, _, _, staffID, testOrgID, _ int64) {
			h.notifyUserRoleChanged(ctx, staffID, testOrgID, "مدير فرع")
		},
		expectedMinLogs: 1,
		targetUserID:    21,
		titleSnippet:    "تحديث الصلاحيات",
	},
	{
		name: "Password set by admin -> user",
		trigger: func(h *UIHandler, ctx context.Context, _, _, staffID, _, _ int64) {
			h.notifyUserPasswordSetByAdmin(ctx, staffID)
		},
		expectedMinLogs: 1,
		targetUserID:    21,
		titleSnippet:    "تعيين كلمة مرور جديدة",
	},
	{
		name: "Subscription expiring in 7 days -> org",
		trigger: func(h *UIHandler, ctx context.Context, _, _, _, testOrgID, _ int64) {
			h.notifySubscriptionExpiring(ctx, testOrgID, "الباقة الاحترافية", 7)
		},
		expectedMinLogs: 1,
		expectedPerm:    "vendor.billing.view",
		titleSnippet:    "اقتراب انتهاء باقة الاشتراك",
	},
	{
		name: "Subscription expired -> org",
		trigger: func(h *UIHandler, ctx context.Context, _, _, _, testOrgID, _ int64) {
			h.notifySubscriptionExpired(ctx, testOrgID, "الباقة الاحترافية")
		},
		expectedMinLogs: 1,
		expectedPerm:    "vendor.billing.view",
		titleSnippet:    "انتهت صلاحية باقة الاشتراك",
	},
	{
		name: "Wallet withdrawal approved -> org",
		trigger: func(h *UIHandler, ctx context.Context, _, ownerID, _, testOrgID, _ int64) {
			amt, _ := money.Parse("15000.00")
			h.notifyWalletWithdrawal(ctx, ownerID, testOrgID, amt, "approved")
		},
		expectedMinLogs: 1,
		expectedPerm:    "pharmacy.wallet.view",
		titleSnippet:    "سحب الرصيد",
	},
	{
		name: "Wallet withdrawal rejected -> org",
		trigger: func(h *UIHandler, ctx context.Context, _, ownerID, _, testOrgID, _ int64) {
			amt, _ := money.Parse("15000.00")
			h.notifyWalletWithdrawalRejected(ctx, ownerID, testOrgID, amt, "بيانات الحساب البنكي غير مطابقة")
		},
		expectedMinLogs: 1,
		expectedPerm:    "pharmacy.wallet.view",
		titleSnippet:    "رفض طلب سحب الرصيد",
	},
	{
		name: "Refund issued -> org",
		trigger: func(h *UIHandler, ctx context.Context, _, _, _, testOrgID, _ int64) {
			amt, _ := money.Parse("250.00")
			h.notifyRefundIssued(ctx, testOrgID, "ORD-999", amt, "إرجاع صنف تالف")
		},
		expectedMinLogs: 1,
		expectedPerm:    "vendor.billing.view",
		titleSnippet:    "استرداد مالي",
	},
	{
		name: "New review on organization -> org",
		trigger: func(h *UIHandler, ctx context.Context, _, _, _, testOrgID, _ int64) {
			h.notifyNewReview(ctx, testOrgID, 5, "خدمة ممتازة وسرعة توصيل")
		},
		expectedMinLogs: 1,
		expectedPerm:    "vendor.organization.view",
		titleSnippet:    "تقييم ورأي جديد",
	},
	{
		name: "Offline chat message -> recipient user",
		trigger: func(h *UIHandler, ctx context.Context, _, _, staffID, _, _ int64) {
			h.notifyOfflineChatMessage(ctx, staffID, "د. أحمد", "هل الصنف متوفر لديكم؟")
		},
		expectedMinLogs: 1,
		targetUserID:    21,
		titleSnippet:    "رسالة محادثة جديدة",
	},
	{
		name: "Job application received -> org",
		trigger: func(h *UIHandler, ctx context.Context, _, _, _, testOrgID, _ int64) {
			h.notifyJobApplicationReceived(ctx, testOrgID, "مدير صيدلية", "د. كريم", "01012345678")
		},
		expectedMinLogs: 1,
		expectedPerm:    "hr.job.manage",
		titleSnippet:    "طلب توظيف جديد",
	},
}
