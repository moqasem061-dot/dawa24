package i18n

// Keys for the admin "System pages" screen (internal/ui/admin_pagecontrol_*).
// The screen's static labels live in the template; these are only the strings a
// Go handler emits as a redirect notice, which cannot be hardcoded because a
// handler string has no language.
func loadAdminPageControlKeys(e *engine) {
	const ns = "admin"
	addKey(e, "admin.pagecontrol.service_unavailable", ns,
		"خدمة التحكم في الصفحات غير متاحة حالياً.",
		"The page control service is unavailable right now.",
		"System pages: store not wired")
	addKey(e, "admin.pagecontrol.not_found", ns,
		"الصفحة المطلوبة غير موجودة في السجل.",
		"That managed page was not found.",
		"System pages: unknown id")
	addKey(e, "admin.pagecontrol.invalid_input", ns,
		"البيانات المُدخلة غير صالحة. تحقّق من المسار ونمط المطابقة.",
		"The input is not valid. Check the path and match mode.",
		"System pages: validation failed")
	addKey(e, "admin.pagecontrol.toggled_enabled", ns,
		"تم تفعيل الصفحة. ستعود للعمل خلال ثوانٍ على كل الخوادم.",
		"Page enabled. It returns to service within seconds across all instances.",
		"System pages: enable success")
	addKey(e, "admin.pagecontrol.toggled_disabled", ns,
		"تم تعطيل الصفحة. أي محاولة وصول إليها سترجع 404.",
		"Page disabled. Any request to it now returns 404.",
		"System pages: disable success")
	addKey(e, "admin.pagecontrol.toggle_failed", ns,
		"تعذّر تغيير حالة الصفحة.",
		"Could not change the page state.",
		"System pages: toggle failed")
	addKey(e, "admin.pagecontrol.created", ns,
		"تمت إضافة الصفحة إلى السجل.",
		"The page was added.",
		"System pages: create success")
	addKey(e, "admin.pagecontrol.create_failed", ns,
		"تعذّرت إضافة الصفحة.",
		"Could not add the page.",
		"System pages: create failed")
	addKey(e, "admin.pagecontrol.updated", ns,
		"تم تحديث بيانات الصفحة بنجاح.",
		"The page was updated successfully.",
		"System pages: update success")
	addKey(e, "admin.pagecontrol.update_failed", ns,
		"تعذّر تحديث بيانات الصفحة.",
		"Could not update the page.",
		"System pages: update failed")
	addKey(e, "admin.pagecontrol.deleted", ns,
		"تم حذف الصفحة المخصّصة من السجل.",
		"The custom page was removed.",
		"System pages: delete success")
	addKey(e, "admin.pagecontrol.delete_failed", ns,
		"تعذّر حذف الصفحة.",
		"Could not delete the page.",
		"System pages: delete failed")
	addKey(e, "admin.pagecontrol.rescanned", ns,
		"تم فحص المسارات: أُضيفت %d صفحة جديدة.",
		"Routes rescanned: %d new pages added.",
		"System pages: rescan result")
}
