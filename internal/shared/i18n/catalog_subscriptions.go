package i18n

// The subscriber log's vocabulary.
//
// Kept in its own file rather than appended to catalog_billing_hr.go because
// that file is already near the size ceiling, and because these keys describe
// one screen: /admin/plans?tab=subscriptions.

func loadSubscriptionLogKeys(e *engine) {
	addKey(e, "admin.subs.empty_title", "admin",
		"لا توجد اشتراكات مطابقة", "No matching subscriptions",
		"Subscriber log empty state title")
	addKey(e, "admin.subs.empty_body", "admin",
		"جرّب توسيع الفلاتر أو إزالتها لعرض سجلات الاشتراكات.",
		"Try widening or clearing the filters to see subscription records.",
		"Subscriber log empty state body")

	addKey(e, "admin.subs.col_ref", "admin", "رقم الاشتراك", "Reference", "Subscriber log column")
	addKey(e, "admin.subs.col_buyer", "admin", "المنشأة / المستفيد", "Organization / subscriber", "Subscriber log column")
	addKey(e, "admin.subs.col_plan", "admin", "الباقة", "Plan", "Subscriber log column")
	addKey(e, "admin.subs.col_amount", "admin", "المبلغ المدفوع", "Amount paid", "Subscriber log column")
	addKey(e, "admin.subs.col_status", "admin", "الحالة", "Status", "Subscriber log column")
	addKey(e, "admin.subs.col_cycle", "admin", "دورة الفوترة", "Billing cycle", "Subscriber log column")
	addKey(e, "admin.subs.col_renew", "admin", "التجديد التلقائي", "Auto-renew", "Subscriber log column")
	addKey(e, "admin.subs.col_starts", "admin", "تاريخ البدء", "Starts", "Subscriber log column")
	addKey(e, "admin.subs.col_expires", "admin", "تاريخ الانتهاء", "Expires", "Subscriber log column")
	addKey(e, "admin.subs.col_remaining", "admin", "الأيام المتبقية", "Days left", "Subscriber log column")
	addKey(e, "admin.subs.col_date", "admin", "التاريخ", "Date", "Subscription history column")
	addKey(e, "admin.subs.col_action", "admin", "العملية", "Action", "Subscription history column")
	addKey(e, "admin.subs.col_details", "admin", "التفاصيل", "Details", "Subscription history column")

	addKey(e, "admin.subs.personal", "admin", "اشتراك شخصي", "Personal", "Badge for a subscription with no organization")
	addKey(e, "admin.subs.free", "admin", "مجانية", "Free", "Badge for a free plan subscription")

	addKey(e, "admin.subs.status_active", "admin", "ساري وفعال", "Active", "Subscription status")
	addKey(e, "admin.subs.status_trialing", "admin", "فترة تجريبية", "Trialing", "Subscription status")
	addKey(e, "admin.subs.status_past_due", "admin", "متأخر السداد", "Past due", "Subscription status")
	addKey(e, "admin.subs.status_cancelled", "admin", "ملغي", "Cancelled", "Subscription status")
	// A term that has run out is reported from expires_at rather than from the
	// status column: nothing sweeps status when a term lapses, so a row can read
	// active months after it ended.
	addKey(e, "admin.subs.status_expired", "admin", "منتهي", "Expired", "Subscription status")

	addKey(e, "admin.subs.cycle_monthly", "admin", "شهري", "Monthly", "Billing cycle")
	addKey(e, "admin.subs.cycle_annual", "admin", "سنوي", "Annual", "Billing cycle")

	addKey(e, "admin.subs.filter_org", "admin", "المنشأة أو المستفيد", "Organization or subscriber", "Subscriber log filter")
	addKey(e, "admin.subs.filter_org_ph", "admin", "ابحث بالاسم التجاري أو البريد...", "Search by trade name or email…", "Subscriber log filter placeholder")
	addKey(e, "admin.subs.filter_expiring", "admin", "ينتهي خلال", "Expiring within", "Subscriber log filter")
	addKey(e, "admin.subs.filter_expiring_days", "admin", "%d يوم", "%d days", "Subscriber log expiring window option")
	addKey(e, "admin.subs.filter_starts_from", "admin", "بدأ من", "Started from", "Subscriber log filter")
	addKey(e, "admin.subs.filter_starts_to", "admin", "بدأ حتى", "Started to", "Subscriber log filter")
	addKey(e, "admin.subs.filter_expires_from", "admin", "ينتهي من", "Expires from", "Subscriber log filter")
	addKey(e, "admin.subs.filter_expires_to", "admin", "ينتهي حتى", "Expires to", "Subscriber log filter")

	addKey(e, "admin.subs.history_action", "admin", "السجل", "History", "Subscription history button")
	addKey(e, "admin.subs.history_title", "admin", "سجل تغييرات الاشتراك", "Subscription history", "Subscription history modal title")
	addKey(e, "admin.subs.history_empty", "admin", "لا توجد حركات مسجلة لهذا الاشتراك.", "No recorded changes for this subscription.", "Subscription history empty state")

	addKey(e, "admin.subs.action_subscription_checkout", "admin", "اشتراك جديد", "New subscription", "Subscription history action")
	addKey(e, "admin.subs.action_renewed", "admin", "تجديد", "Renewal", "Subscription history action (legacy value)")
	addKey(e, "admin.subs.action_subscription_renewal", "admin", "تجديد", "Renewal", "Subscription history action")
	addKey(e, "admin.subs.action_subscription_change", "admin", "تغيير باقة", "Plan change", "Subscription history action")
	addKey(e, "admin.subs.action_subscription_upgrade", "admin", "ترقية", "Upgrade", "Subscription history action")
	addKey(e, "admin.subs.action_cancel", "admin", "إلغاء", "Cancellation", "Subscription history action")
	addKey(e, "admin.subs.action_", "admin", "غير محدد", "Unspecified", "Subscription history action fallback")

	addKey(e, "common.apply_filters", "common", "تطبيق الفلاتر", "Apply filters", "Filter bar submit")
	addKey(e, "common.reset", "common", "إعادة تعيين", "Reset", "Filter bar reset")
}

// loadSharedYesNoAndOrgTypeKeys fills gaps the subscriber log surfaced.
//
// common.yes/common.no and the org.type.* labels were being rendered from
// hardcoded strings at each call site, so a screen showing an organisation's
// type in Arabic had no English at all.
func loadSharedYesNoAndOrgTypeKeys(e *engine) {
	addKey(e, "common.yes", "common", "نعم", "Yes", "Boolean yes")
	addKey(e, "common.no", "common", "لا", "No", "Boolean no")

	addKey(e, "org.type.vendor", "org", "مورد", "Supplier", "Organization type")
	addKey(e, "org.type.supplier", "org", "مورد", "Supplier", "Organization type")
	addKey(e, "org.type.pharmacy", "org", "صيدلية", "Pharmacy", "Organization type")
	addKey(e, "org.type.customer", "org", "صيدلية", "Pharmacy", "Organization type (legacy value)")
	addKey(e, "org.type.company", "org", "شركة", "Company", "Organization type")
	addKey(e, "org.type.agency", "org", "وكالة", "Agency", "Organization type")
	addKey(e, "org.type.", "org", "غير محدد", "Unspecified", "Organization type fallback")
}

// loadAdminVariantListingKeys is /admin/product-child: every supplier's stock,
// with the branch and warehouse detail the screen was missing.
func loadAdminVariantListingKeys(e *engine) {
	addKey(e, "admin.child.title", "admin", "أصناف الموردين وعروض التوريد", "Supplier items and offers", "Page title")
	addKey(e, "admin.child.heading", "admin", "أصناف الموردين وعروض الفروع (%d)", "Supplier items and branch offers (%d)", "Page heading with total")
	addKey(e, "admin.child.subtitle", "admin",
		"استعراض أصناف الموردين مع الفرع والمخازن والكميات المتاحة في كل مخزن.",
		"Supplier items with their branch, warehouses and the quantity held in each.",
		"Page subtitle")
	addKey(e, "admin.child.back", "admin", "العودة لكتالوج المنتجات", "Back to the product catalogue", "Back link")

	addKey(e, "admin.child.col_image", "admin", "الصورة", "Image", "Column")
	addKey(e, "admin.child.col_item", "admin", "الصنف / SKU", "Item / SKU", "Column")
	addKey(e, "admin.child.col_supplier", "admin", "المورد", "Supplier", "Column")
	addKey(e, "admin.child.col_branch", "admin", "الفرع", "Branch", "Column")
	addKey(e, "admin.child.col_price", "admin", "السعر", "Price", "Column")
	addKey(e, "admin.child.col_stock", "admin", "الرصيد", "Stock", "Column")
	addKey(e, "admin.child.col_warehouses", "admin", "المخازن والكميات", "Warehouses and quantities", "Column")
	addKey(e, "admin.child.col_status", "admin", "الحالة", "Status", "Column")

	addKey(e, "admin.child.status_active", "admin", "نشط ومعروض", "Active", "Variant status")
	addKey(e, "admin.child.status_inactive", "admin", "موقوف", "Inactive", "Variant status")
	addKey(e, "admin.child.out_of_stock", "admin", "لا يوجد رصيد", "Out of stock", "Stock badge")
	addKey(e, "admin.child.parent_image", "admin", "صورة المنتج الأساسي", "Master product image", "Image provenance badge")
	addKey(e, "admin.child.any_branch", "admin", "كل الفروع", "Any branch", "Variant with no branch of its own")

	addKey(e, "admin.child.filter_search", "admin", "بحث", "Search", "Filter label")
	addKey(e, "admin.child.filter_search_ph", "admin",
		"اسم الصنف أو المورد أو SKU أو الباركود أو رقم التشغيلة...",
		"Item, supplier, SKU, barcode or batch…", "Filter placeholder")
	addKey(e, "admin.child.filter_warehouse", "admin", "المخزن", "Warehouse", "Filter label")
	addKey(e, "admin.child.filter_expiring", "admin", "قارب على الانتهاء", "Expiring soon", "Filter label")
	addKey(e, "admin.child.stock_in", "admin", "متوفر", "In stock", "Stock filter")
	addKey(e, "admin.child.stock_low", "admin", "رصيد منخفض", "Low stock", "Stock filter")
	addKey(e, "admin.child.stock_out", "admin", "نافد", "Out of stock", "Stock filter")

	addKey(e, "admin.child.empty_title", "admin", "لا توجد أصناف مطابقة", "No matching items", "Empty state")
	addKey(e, "admin.child.empty_body", "admin",
		"جرّب توسيع الفلاتر أو إزالتها لعرض أصناف الموردين.",
		"Try widening or clearing the filters to see supplier items.", "Empty state")
}

// loadAdminOrdersFilterKeys renames the order filters from "the pharmacy" to
// "the buyer".
//
// A supplier restocking from another supplier is an ordinary order on this
// marketplace. Calling the filter الصيدلية and populating it only with
// organisations of type customer made those orders unfindable by their buyer.
func loadAdminOrdersFilterKeys(e *engine) {
	addKey(e, "admin.orders.filter_buyer", "admin", "المشتري", "Buyer", "Order filter label")
	addKey(e, "admin.orders.all_buyers", "admin", "كافة المشترين", "All buyers", "Order filter option")
	addKey(e, "admin.orders.filter_buyer_type", "admin", "نوع المشتري", "Buyer type", "Order filter label")
	addKey(e, "admin.orders.filter_seller", "admin", "البائع المنفّذ", "Fulfilling seller", "Order filter label")
	addKey(e, "admin.orders.all_sellers", "admin", "كافة البائعين", "All sellers", "Order filter option")
	addKey(e, "admin.orders.search_ph", "admin",
		"رقم الطلب، اسم المشتري، أو اسم البائع...",
		"Order number, buyer name, or seller name…", "Order search placeholder")
}

// loadAdminWarehouseKeys is /admin/warehouses: every organisation's warehouses,
// with the filters, pager and controls the screen was missing.
func loadAdminWarehouseKeys(e *engine) {
	addKey(e, "admin.wh.title", "admin", "إدارة المخازن ومراكز التوزيع", "Warehouses and distribution centres", "Page title")
	addKey(e, "admin.wh.subtitle", "admin",
		"متابعة مخازن كل المنشآت وأرصدتها الحية، مع إمكانية التعديل والتفعيل والإيقاف.",
		"Every organisation's warehouses and live balances, with edit and enable/disable.",
		"Page subtitle")
	addKey(e, "admin.wh.total_badge", "admin", "إجمالي %d مخزن", "%d warehouses", "Header count badge")
	addKey(e, "admin.wh.create", "admin", "إضافة مخزن", "Add warehouse", "Create action")

	addKey(e, "admin.wh.col_name", "admin", "اسم المخزن", "Warehouse", "Column")
	addKey(e, "admin.wh.col_owner", "admin", "المنشأة المالكة", "Owning organization", "Column")
	addKey(e, "admin.wh.col_branch", "admin", "الفرع", "Branch", "Column")
	addKey(e, "admin.wh.col_items", "admin", "عدد الأصناف", "Items", "Column")
	addKey(e, "admin.wh.col_qty", "admin", "إجمالي الكمية", "Total quantity", "Column")
	addKey(e, "admin.wh.col_last_movement", "admin", "آخر حركة", "Last movement", "Column")
	addKey(e, "admin.wh.col_status", "admin", "الحالة", "Status", "Column")

	addKey(e, "admin.wh.active", "admin", "مُفعّل", "Active", "Warehouse status")
	addKey(e, "admin.wh.inactive", "admin", "موقوف", "Inactive", "Warehouse status")
	addKey(e, "admin.wh.enable", "admin", "تفعيل", "Enable", "Warehouse action")
	addKey(e, "admin.wh.disable", "admin", "إيقاف", "Disable", "Warehouse action")
	// Disabling a warehouse that still holds goods is allowed but worth
	// warning about: the stock stays counted while the warehouse stops being
	// offered, which is inventory that exists in arithmetic and nowhere else.
	addKey(e, "admin.wh.disable_holds_stock", "admin",
		"هذا المخزن يحتوي على رصيد؛ إيقافه لا يحذف الكميات.",
		"This warehouse still holds stock; disabling it does not remove the quantities.",
		"Disable warning tooltip")

	addKey(e, "admin.wh.field_code", "admin", "الكود", "Code", "Form field")
	addKey(e, "admin.wh.field_phone", "admin", "الهاتف", "Phone", "Form field")
	addKey(e, "admin.wh.field_address", "admin", "العنوان", "Address", "Form field")

	addKey(e, "admin.wh.filter_search", "admin", "بحث", "Search", "Filter label")
	addKey(e, "admin.wh.filter_search_ph", "admin",
		"اسم المخزن أو الكود أو المنشأة...", "Warehouse, code or organization…", "Filter placeholder")

	addKey(e, "admin.wh.empty_title", "admin", "لا توجد مخازن مطابقة", "No matching warehouses", "Empty state")
	addKey(e, "admin.wh.empty_body", "admin",
		"جرّب توسيع الفلاتر أو إزالتها لعرض مخازن المنشآت.",
		"Try widening or clearing the filters to see the organisations' warehouses.", "Empty state")

	addKey(e, "admin.wh.saved", "admin", "تم حفظ بيانات المخزن بنجاح.", "Warehouse saved.", "Success notice")
	addKey(e, "admin.wh.status_changed", "admin", "تم تحديث حالة المخزن بنجاح.", "Warehouse status updated.", "Success notice")
	addKey(e, "admin.wh.invalid", "admin", "بيانات المخزن غير مكتملة.", "The warehouse details are incomplete.", "Error notice")
}

// loadAdminErrorDetailKeys is the diagnostics modal on
// /admin/developers?tab=errors.
func loadAdminErrorDetailKeys(e *engine) {
	addKey(e, "admin.err.modal_title", "admin", "سجل تشخيص الخطأ", "Error diagnostics", "Modal title")
	addKey(e, "admin.err.not_found", "admin", "لم يتم العثور على هذا السجل.", "This record was not found.", "Missing error")
	addKey(e, "admin.err.request", "admin", "الطلب", "Request", "Field label")
	addKey(e, "admin.err.source", "admin", "الملف والسطر", "File and line", "Field label")
	addKey(e, "admin.err.user", "admin", "المستخدم", "User", "Field label")
	addKey(e, "admin.err.organization", "admin", "المنشأة", "Organization", "Field label")
	addKey(e, "admin.err.ip", "admin", "عنوان IP", "IP address", "Field label")
	addKey(e, "admin.err.user_agent", "admin", "المتصفح", "User agent", "Field label")
	addKey(e, "admin.err.payload", "admin", "محتوى الطلب", "Request payload", "Section heading")
	addKey(e, "admin.err.stack", "admin", "تتبّع الاستدعاءات", "Stack trace", "Section heading")
	addKey(e, "admin.err.no_stack", "admin",
		"لم يُسجَّل تتبّع استدعاءات لهذا الخطأ.", "No stack trace was recorded for this error.",
		"Empty stack trace")

	addKey(e, "admin.err.status_NEW", "admin", "جديد", "New", "Error status")
	addKey(e, "admin.err.status_INVESTIGATING", "admin", "قيد الفحص", "Investigating", "Error status")
	addKey(e, "admin.err.status_RESOLVED", "admin", "تم الحل", "Resolved", "Error status")
	addKey(e, "admin.err.status_IGNORED", "admin", "تم التجاهل", "Ignored", "Error status")

	addKey(e, "admin.err.mark_investigating", "admin", "بدء الفحص", "Start investigating", "Error status action")
	addKey(e, "admin.err.mark_resolved", "admin", "تعليم كمحلول", "Mark resolved", "Error status action")
	addKey(e, "admin.err.mark_ignored", "admin", "تجاهل", "Ignore", "Error status action")
}

// loadAdminAuditDetailKeys is the audit diff modal, which had the same
// escaping defect as the error one.
func loadAdminAuditDetailKeys(e *engine) {
	addKey(e, "admin.audit.modal_title", "admin", "تفاصيل عملية التدقيق", "Audit entry", "Modal title")
	addKey(e, "admin.audit.entity", "admin", "الكيان", "Entity", "Field label")
	addKey(e, "admin.audit.before", "admin", "قبل التغيير", "Before", "Section heading")
	addKey(e, "admin.audit.after", "admin", "بعد التغيير", "After", "Section heading")
	addKey(e, "admin.audit.no_before", "admin", "لا توجد حالة سابقة (عملية إنشاء).", "No prior state (a creation).", "Empty diff side")
	addKey(e, "admin.audit.no_after", "admin", "لا توجد حالة لاحقة (عملية حذف).", "No resulting state (a deletion).", "Empty diff side")
}

// loadAdminRegistrationKeys is the registration review modal on
// /admin/approvals?tab=organizations.
func loadAdminRegistrationKeys(e *engine) {
	addKey(e, "admin.reg.not_found", "admin", "لم يتم العثور على هذه المنشأة.", "This organization was not found.", "Missing record")
	addKey(e, "admin.reg.section_identity", "admin", "بيانات المنشأة", "Organization identity", "Section")
	addKey(e, "admin.reg.section_contact", "admin", "بيانات التواصل", "Contact details", "Section")
	addKey(e, "admin.reg.section_owner", "admin", "مقدّم الطلب", "Requesting user", "Section")
	addKey(e, "admin.reg.section_branches", "admin", "الفروع (%d)", "Branches (%d)", "Section")
	addKey(e, "admin.reg.section_documents", "admin", "المستندات (%d)", "Documents (%d)", "Section")
	addKey(e, "admin.reg.section_trail", "admin", "ملاحظات المراجعة", "Review notes", "Section")

	addKey(e, "admin.reg.legal_name", "admin", "الاسم القانوني", "Legal name", "Field")
	addKey(e, "admin.reg.trade_name_ar", "admin", "الاسم التجاري (عربي)", "Trade name (Arabic)", "Field")
	addKey(e, "admin.reg.trade_name_en", "admin", "الاسم التجاري (إنجليزي)", "Trade name (English)", "Field")
	addKey(e, "admin.reg.commercial_register", "admin", "رقم السجل التجاري", "Commercial register", "Field")
	addKey(e, "admin.reg.tax_number", "admin", "الرقم الضريبي", "Tax number", "Field")
	addKey(e, "admin.reg.pharmacist_license", "admin", "ترخيص الصيدلي", "Pharmacist licence", "Field")
	addKey(e, "admin.reg.email", "admin", "البريد الإلكتروني", "Email", "Field")
	addKey(e, "admin.reg.phone", "admin", "الهاتف", "Phone", "Field")
	addKey(e, "admin.reg.address", "admin", "العنوان", "Address", "Field")
	addKey(e, "admin.reg.plan", "admin", "الباقة المختارة", "Selected plan", "Field")
	addKey(e, "admin.reg.owner_name", "admin", "الاسم", "Name", "Field")
	addKey(e, "admin.reg.owner_email", "admin", "البريد الإلكتروني", "Email", "Field")
	addKey(e, "admin.reg.owner_phone", "admin", "الهاتف", "Phone", "Field")
	addKey(e, "admin.reg.owner_profile", "admin", "الملف الشخصي", "Profile", "Field")
	addKey(e, "admin.reg.open_profile", "admin", "فتح ملف المستخدم", "Open user profile", "Link")
	addKey(e, "admin.reg.notes", "admin", "ملاحظات التدقيق", "Verification notes", "Field")
	addKey(e, "admin.reg.rejection", "admin", "سبب الرفض", "Rejection reason", "Field")

	addKey(e, "admin.reg.branch", "admin", "الفرع", "Branch", "Column")
	addKey(e, "admin.reg.branch_address", "admin", "العنوان", "Address", "Column")
	addKey(e, "admin.reg.branch_works", "admin", "الأعمال المؤسسية", "Institutional works", "Column")
	addKey(e, "admin.reg.main_branch", "admin", "رئيسي", "Main", "Badge")
	// A branch with no institutional work cannot buy or be bought from, so it
	// is flagged in red at review time rather than discovered later.
	addKey(e, "admin.reg.no_works", "admin", "بلا أعمال مؤسسية", "No institutional works", "Warning badge")
	addKey(e, "admin.reg.no_branches", "admin",
		"لم يتم تسجيل أي فرع لهذه المنشأة؛ لن تتمكن من الشراء أو البيع قبل إضافة فرع.",
		"No branch is registered; this company cannot buy or sell until one is added.",
		"Warning")
	addKey(e, "admin.reg.no_documents", "admin",
		"لم يتم رفع أي مستندات مع هذا الطلب.", "No documents were uploaded with this request.", "Warning")
	addKey(e, "admin.reg.view_document", "admin", "عرض", "View", "Document action")

	addKey(e, "admin.reg.status_pending", "admin", "قيد التدقيق", "Pending review", "Status")
	addKey(e, "admin.reg.status_approved", "admin", "معتمد ونشط", "Approved", "Status")
	addKey(e, "admin.reg.status_rejected", "admin", "مرفوض", "Rejected", "Status")
	addKey(e, "admin.reg.status_suspended", "admin", "موقوف", "Suspended", "Status")

	addKey(e, "admin.reg.doctype_commercial_register", "admin", "السجل التجاري", "Commercial register", "Document type")
	addKey(e, "admin.reg.doctype_tax_card", "admin", "البطاقة الضريبية", "Tax card", "Document type")
	addKey(e, "admin.reg.doctype_pharmacist_license", "admin", "ترخيص الصيدلي", "Pharmacist licence", "Document type")
	addKey(e, "admin.reg.doctype_pharmacy_license", "admin", "ترخيص الصيدلية", "Pharmacy licence", "Document type")
}
