package i18n

// استهلاك الباقة — the sponsorship credit ledger.
//
// The reason labels are keyed by the same identifiers the database CHECK
// constraint accepts, so a statement row renders its reason by lookup rather
// than by a switch that has to be kept in step with the constraint.
func loadPromoCreditKeys(e *engine) {
	addKey(e, "promo.credits.statement_title", "promo", "كشف حساب الباقة", "Package statement", "Statement page title")
	addKey(e, "promo.credits.consumption", "promo", "استهلاك الباقة", "Package usage", "Button and section title")
	addKey(e, "promo.credits.empty_title", "promo", "لا توجد حركات على هذه الباقة", "No movements on this package", "Empty title")
	addKey(e, "promo.credits.empty_message", "promo", "ستظهر هنا كل عملية استهلاك أو إرجاع لرصيد الباقة فور حدوثها.", "Every credit consumed or refunded on this package will appear here.", "Empty message")
	addKey(e, "promo.credits.total", "promo", "إجمالي الرصيد", "Total credits", "Statement metric")
	addKey(e, "promo.credits.consumed", "promo", "المستهلك", "Consumed", "Statement metric")
	addKey(e, "promo.credits.refunded", "promo", "المُرجَع", "Refunded", "Statement metric")
	addKey(e, "promo.credits.remaining", "promo", "المتبقي", "Remaining", "Statement metric")
	addKey(e, "promo.credits.col_date", "promo", "التاريخ", "Date", "Statement column")
	addKey(e, "promo.credits.col_reason", "promo", "السبب", "Reason", "Statement column")
	addKey(e, "promo.credits.col_delta", "promo", "الحركة", "Change", "Statement column")
	addKey(e, "promo.credits.col_balance", "promo", "الرصيد بعد الحركة", "Balance after", "Statement column")
	addKey(e, "promo.credits.col_reference", "promo", "المرجع", "Reference", "Statement column")

	addKey(e, "promo.credits.reason.ad_created", "promo", "إنشاء إعلان", "Ad created", "Ledger reason")
	addKey(e, "promo.credits.reason.ad_refunded", "promo", "إرجاع رصيد إعلان", "Ad credit refunded", "Ledger reason")
	addKey(e, "promo.credits.reason.sponsorship_requested", "promo", "طلب رعاية", "Sponsorship requested", "Ledger reason")
	addKey(e, "promo.credits.reason.sponsorship_batch", "promo", "طلب رعاية متعدد", "Batch sponsorship request", "Ledger reason")
	addKey(e, "promo.credits.reason.sponsorship_rejected", "promo", "إرجاع رصيد طلب مرفوض", "Rejected request refunded", "Ledger reason")
	addKey(e, "promo.credits.reason.adjustment", "promo", "تسوية رصيد", "Balance adjustment", "Ledger reason")

	addKey(e, "promo.credits.note.ad_failed", "promo", "تعذر إنشاء الإعلان", "The ad could not be created", "Refund note")
	addKey(e, "promo.credits.note.request_cancelled", "promo", "سحب طلب الرعاية", "Sponsorship request withdrawn", "Refund note")
	addKey(e, "promo.credits.note.request_rejected", "promo", "رفض طلب الرعاية", "Sponsorship request rejected", "Refund note")

	addKey(e, "promo.credit_invalid_purchase", "promo", "الباقة المحددة غير صالحة.", "A valid purchase is required.", "Validation error")
	addKey(e, "promo.credit_invalid_amount", "promo", "عدد الأرصدة يجب أن يكون أكبر من صفر.", "The number of credits must be positive.", "Validation error")
	addKey(e, "promo.credit_invalid_reason", "promo", "سبب حركة الرصيد غير معروف.", "Unknown credit movement reason.", "Validation error")

	addKey(e, "promo.credits.entity.ad", "promo", "إعلان", "Ad", "Ledger entity")
	addKey(e, "promo.credits.entity.product", "promo", "منتج", "Product", "Ledger entity")
	addKey(e, "promo.credits.entity.offer", "promo", "عرض", "Offer", "Ledger entity")

	addKey(e, "promo.sponsorship.invalid_id", "promo", "معرف طلب الرعاية غير صالح.", "Invalid sponsorship request ID.", "Error notice")
	addKey(e, "promo.sponsorship.approved_success", "promo", "تمت الموافقة على رعاية المنتج وتفعيله في صدارة نتائج البحث بنجاح.", "Product sponsorship approved successfully.", "Success notice")
	addKey(e, "promo.sponsorship.rejected_success", "promo", "تم رفض طلب رعاية المنتج وإعادة الرصيد للمورد بنجاح.", "Product sponsorship rejected and credits refunded successfully.", "Success notice")
	addKey(e, "promo.sponsorship.created_success", "promo", "تم تفعيل رعاية وتثبيت المنتج في الصدارة بنجاح.", "Direct product sponsorship created successfully.", "Success notice")
	addKey(e, "promo.sponsorship.required_fields", "promo", "يرجى تحديد المنشأة، المنتج، وباقة الرعاية بدقة.", "Please select the organization, product, and package.", "Error notice")
	addKey(e, "promo.sponsorship.default_reject_notes", "promo", "تم رفض طلب الرعاية من قبل إدارة المنصة.", "Sponsorship request rejected by platform administration.", "Default rejection note")
	addKey(e, "promo.sponsorship.not_found", "promo", "طلب الرعاية غير موجود.", "Sponsorship request not found.", "Error notice")
}
