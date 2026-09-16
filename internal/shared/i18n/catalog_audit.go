package i18n

func loadAuditKeys(e *engine) {
	// Organizations
	addKey(e, "audit.action.org.approve", "audit", "اعتماد منشأة", "Approve Organization", "Audit log action")
	addKey(e, "audit.action.org.reject", "audit", "رفض منشأة", "Reject Organization", "Audit log action")
	addKey(e, "audit.action.org.suspend", "audit", "إيقاف منشأة", "Suspend Organization", "Audit log action")
	addKey(e, "audit.action.org.reactivate", "audit", "إعادة تفعيل منشأة", "Reactivate Organization", "Audit log action")
	addKey(e, "audit.action.org.registered", "audit", "تسجيل منشأة", "Register Organization", "Audit log action")
	addKey(e, "audit.action.org.change_request.approve", "audit", "اعتماد تعديل بيانات المنشأة", "Approve Profile Change Request", "Audit log action")
	addKey(e, "audit.action.org.change_request.reject", "audit", "رفض تعديل بيانات المنشأة", "Reject Profile Change Request", "Audit log action")
	addKey(e, "audit.action.org.deletion.approve", "audit", "الموافقة على حذف المنشأة", "Approve Org Deletion", "Audit log action")
	addKey(e, "audit.action.org.deletion.reject", "audit", "رفض حذف المنشأة", "Reject Org Deletion", "Audit log action")

	// Users
	addKey(e, "audit.action.user.create_staff", "audit", "إضافة موظف إدارة", "Create Staff User", "Audit log action")
	addKey(e, "audit.action.user.role_assigned", "audit", "تعيين دور وصلاحية", "Assign Role", "Audit log action")
	addKey(e, "audit.action.user.suspend", "audit", "إيقاف حساب مستخدم", "Suspend User", "Audit log action")
	addKey(e, "audit.action.user.reactivate", "audit", "إعادة تفعيل حساب مستخدم", "Reactivate User", "Audit log action")
	addKey(e, "audit.action.user.reset_mfa", "audit", "إعادة ضبط المصادقة الثنائية (MFA)", "Reset MFA", "Audit log action")
	addKey(e, "audit.action.identity.user.mfa_reset", "audit", "إعادة ضبط المصادقة الثنائية (MFA)", "Reset MFA", "Audit log action")
	addKey(e, "audit.action.identity.user.role_assigned", "audit", "تعيين دور وصلاحية", "Assign Role", "Audit log action")
	addKey(e, "audit.action.identity.user.status_changed", "audit", "تعديل حالة المستخدم", "Change User Status", "Audit log action")
	addKey(e, "audit.action.user.password_set", "audit", "تعيين كلمة المرور", "Set Password", "Audit log action")
	addKey(e, "audit.action.user.deletion.approve", "audit", "الموافقة على حذف حساب", "Approve User Deletion", "Audit log action")
	addKey(e, "audit.action.user.deletion.reject", "audit", "رفض حذف حساب", "Reject User Deletion", "Audit log action")

	// Catalogue
	addKey(e, "audit.action.catalog.product.create", "audit", "إضافة منتج للكتالوج", "Create Product", "Audit log action")
	addKey(e, "audit.action.catalog.product.edit", "audit", "تعديل منتج", "Edit Product", "Audit log action")
	addKey(e, "audit.action.catalog.product.update", "audit", "تعديل منتج", "Update Product", "Audit log action")
	addKey(e, "audit.action.catalog.product.delete", "audit", "حذف منتج", "Delete Product", "Audit log action")
	addKey(e, "audit.action.catalog.variant.create", "audit", "إضافة عرض توريد صنف", "Create Variant", "Audit log action")
	addKey(e, "audit.action.catalog.variant.edit", "audit", "تعديل عرض توريد صنف", "Edit Variant", "Audit log action")
	addKey(e, "audit.action.catalog.variant.update", "audit", "تعديل عرض توريد صنف", "Update Variant", "Audit log action")
	addKey(e, "audit.action.catalog.variant.delete", "audit", "حذف عرض توريد صنف", "Delete Variant", "Audit log action")
	addKey(e, "audit.action.catalog.variant.toggle", "audit", "تغيير حالة عرض التوريد", "Toggle Variant Status", "Audit log action")
	addKey(e, "audit.action.catalog.bulk.activate_all", "audit", "تفعيل شامل لكافة الأصناف", "Bulk Activate All Variants", "Audit log action")
	addKey(e, "audit.action.catalog.bulk.delete_all", "audit", "حذف شامل لكافة الأصناف", "Bulk Delete All Products", "Audit log action")

	// Commerce & Billing
	addKey(e, "audit.action.commerce.order.status_change", "audit", "تحديث حالة الطلب", "Order Status Change", "Audit log action")
	addKey(e, "audit.action.commerce.refund", "audit", "استرداد مالي لطلب", "Order Refund", "Audit log action")
	addKey(e, "audit.action.payment.adjust", "audit", "تسوية رصيد محفظة", "Wallet Adjustment", "Audit log action")
	addKey(e, "audit.action.billing.wallet.adjusted", "audit", "تسوية رصيد محفظة", "Wallet Adjustment", "Audit log action")
	addKey(e, "audit.action.commerce.deposit.approve", "audit", "اعتماد إيداع محفظة", "Approve Deposit", "Audit log action")
	addKey(e, "audit.action.commerce.deposit.reject", "audit", "رفض إيداع محفظة", "Reject Deposit", "Audit log action")
	addKey(e, "audit.action.commerce.withdrawal.approve", "audit", "اعتماد سحب رصيد", "Approve Withdrawal", "Audit log action")
	addKey(e, "audit.action.commerce.withdrawal.reject", "audit", "رفض سحب رصيد", "Reject Withdrawal", "Audit log action")

	// Promo
	addKey(e, "audit.action.promo.offer.approve", "audit", "اعتماد عرض ترويجي", "Approve Offer", "Audit log action")
	addKey(e, "audit.action.promo.offer.reject", "audit", "رفض عرض ترويجي", "Reject Offer", "Audit log action")
	addKey(e, "audit.action.promo.sponsorship.approve", "audit", "اعتماد طلب رعاية", "Approve Sponsorship", "Audit log action")
	addKey(e, "audit.action.promo.sponsorship.reject", "audit", "رفض طلب رعاية", "Reject Sponsorship", "Audit log action")
	addKey(e, "audit.action.promo.ad.approve", "audit", "اعتماد إعلان ترويجي", "Approve Ad", "Audit log action")
	addKey(e, "audit.action.promo.ad.reject", "audit", "رفض إعلان ترويجي", "Reject Ad", "Audit log action")

	// Reference data
	addKey(e, "audit.action.reference.city.create", "audit", "إضافة مدينة / مركز", "Create City", "Audit log action")
	addKey(e, "audit.action.reference.city.edit", "audit", "تعديل مدينة / مركز", "Edit City", "Audit log action")
	addKey(e, "audit.action.reference.city.toggle", "audit", "تغيير حالة تفعيل المدينة", "Toggle City Status", "Audit log action")
	addKey(e, "audit.action.reference.governorate.create", "audit", "إضافة محافظة", "Create Governorate", "Audit log action")
	addKey(e, "audit.action.reference.governorate.edit", "audit", "تعديل محافظة", "Edit Governorate", "Audit log action")
	addKey(e, "audit.action.reference.governorate.toggle", "audit", "تغيير حالة تفعيل المحافظة", "Toggle Governorate Status", "Audit log action")
	addKey(e, "audit.action.reference.institutional_work.create", "audit", "إضافة تصنيف عمل مؤسسي", "Create Institutional Work", "Audit log action")
	addKey(e, "audit.action.reference.institutional_work.edit", "audit", "تعديل تصنيف عمل مؤسسي", "Edit Institutional Work", "Audit log action")
	addKey(e, "audit.action.reference.institutional_work.delete", "audit", "حذف تصنيف عمل مؤسسي", "Delete Institutional Work", "Audit log action")
	addKey(e, "audit.action.reference.plan.create", "audit", "إنشاء باقة اشتراك", "Create Plan", "Audit log action")
	addKey(e, "audit.action.reference.plan.edit", "audit", "تعديل باقة اشتراك", "Edit Plan", "Audit log action")
	addKey(e, "audit.action.reference.plan.toggle", "audit", "تغيير حالة باقة الاشتراك", "Toggle Plan Status", "Audit log action")
}
