package i18n

// بيانات المنشأة — the sectioned organization profile and its review queue.
//
// The section titles and the field labels live here rather than in the templ
// files because both the company's own page and the administrator's review
// queue name the same fields, and a diff that labelled a field differently from
// the form that produced it would be its own small lie.
func loadOrgProfileKeys(e *engine) {
	addKey(e, "org.profile.title", "org", "بيانات المنشأة", "Organization Profile", "Profile page title")

	addKey(e, "org.profile.section.identity", "org", "الهوية التجارية", "Commercial identity", "Section title")
	addKey(e, "org.profile.section.identity_sub", "org",
		"الاسم القانوني والتجاري والسجل التجاري والرقم الضريبي — هذه هي البيانات التي اعتُمدت بها المنشأة، لذلك يمر تعديلها على إدارة المنصة.",
		"Legal and trade name, commercial register and tax number. These are the details the platform verified on approval, so changes are reviewed.",
		"Section subtitle")
	addKey(e, "org.profile.section.limits", "org", "حدود قيمة الطلب", "Order value limits", "Section title")
	addKey(e, "org.profile.section.limits_sub", "org",
		"أقل وأعلى قيمة يقبلها هذا المورد للطلب الواحد.",
		"The smallest and largest order value this supplier accepts.", "Section subtitle")
	addKey(e, "org.profile.section.contact", "org", "بيانات التواصل", "Contact details", "Section title")
	addKey(e, "org.profile.section.contact_sub", "org",
		"أرقام وعناوين التواصل الرسمية للمنشأة.",
		"The organization's official contact numbers and address.", "Section subtitle")
	addKey(e, "org.profile.section.description", "org", "الوصف التعريفي", "Public description", "Section title")
	addKey(e, "org.profile.section.description_sub", "org",
		"النبذة التي تظهر للعملاء في الصفحة العامة للمنشأة.",
		"The blurb customers see on the organization's public page.", "Section subtitle")
	addKey(e, "org.profile.section.media", "org", "الشعار وصورة الغلاف", "Logo and cover image", "Section title")
	addKey(e, "org.profile.section.media_sub", "org",
		"ترك الحقل فارغاً يُبقي الصورة الحالية كما هي.",
		"Leaving a field empty keeps the current image.", "Section subtitle")

	addKey(e, "org.profile.field.legal_name", "org", "الاسم القانوني", "Legal name", "Field label")
	addKey(e, "org.profile.field.trade_name_ar", "org", "الاسم التجاري (عربي)", "Trade name (Arabic)", "Field label")
	addKey(e, "org.profile.field.trade_name_en", "org", "الاسم التجاري (إنجليزي)", "Trade name (English)", "Field label")
	addKey(e, "org.profile.field.commercial_register", "org", "رقم السجل التجاري", "Commercial register", "Field label")
	addKey(e, "org.profile.field.tax_number", "org", "الرقم الضريبي", "Tax number", "Field label")
	addKey(e, "org.profile.field.min_order_price", "org", "الحد الأدنى لقيمة الطلب", "Minimum order value", "Field label")
	addKey(e, "org.profile.field.max_order_price", "org", "الحد الأقصى لقيمة الطلب", "Maximum order value", "Field label")
	addKey(e, "org.profile.field.email", "org", "البريد الإلكتروني", "Email", "Field label")
	addKey(e, "org.profile.field.phone", "org", "رقم الهاتف", "Phone", "Field label")
	addKey(e, "org.profile.field.address", "org", "العنوان", "Address", "Field label")
	addKey(e, "org.profile.field.organization_number", "org", "رقم المنشأة الداخلي", "Internal organization number", "Field label")
	addKey(e, "org.profile.field.description_ar", "org", "الوصف (عربي)", "Description (Arabic)", "Field label")
	addKey(e, "org.profile.field.description_en", "org", "الوصف (إنجليزي)", "Description (English)", "Field label")
	addKey(e, "org.profile.field.image", "org", "الشعار", "Logo", "Field label")
	addKey(e, "org.profile.field.coverage_image", "org", "صورة الغلاف", "Cover image", "Field label")

	addKey(e, "org.profile.saved", "org", "تم حفظ هذا القسم.", "This section has been saved.", "Success notice")
	addKey(e, "org.profile.withdrawn", "org", "تم سحب طلب التعديل.", "The change request has been withdrawn.", "Success notice")
	addKey(e, "org.profile.change_pending", "org",
		"يوجد بالفعل طلب تعديل قيد المراجعة لهذا القسم.",
		"A change to this section is already awaiting review.", "Conflict notice")
	addKey(e, "org.profile.no_change", "org", "لم يتغير شيء في هذا القسم.", "Nothing in this section changed.", "Notice")
	addKey(e, "org.profile.unknown_section", "org", "قسم غير معروف.", "Unknown profile section.", "Validation error")
	addKey(e, "org.profile.invalid_amount", "org", "قيمة غير صالحة.", "Invalid amount.", "Validation error")
	addKey(e, "org.profile.price_range", "org",
		"لا يمكن أن يقل الحد الأقصى لقيمة الطلب عن الحد الأدنى.",
		"The maximum order value must not be below the minimum.", "Validation error")
	addKey(e, "org.profile.invalid_request", "org", "معرف الطلب غير صالح.", "A valid request id is required.", "Validation error")
	addKey(e, "org.profile.rejection_needs_reason", "org",
		"يجب كتابة سبب الرفض.", "A rejection must say why.", "Validation error")
	addKey(e, "org.profile.change_decided", "org",
		"تمت مراجعة هذا الطلب بالفعل.", "This request has already been decided.", "Conflict notice")
}
