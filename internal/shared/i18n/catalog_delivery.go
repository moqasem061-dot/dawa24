package i18n

// إدارة الشحنات — the delivery representative's portal and the supplier's
// dispatch controls.
//
// These are the strings a handler emits: notices after an action, refusals,
// and the two notifications a handover produces. Page copy lives in the templ
// templates, which are exempt because they are compiled per-language by templ
// itself; anything a Go file can put in front of a person belongs here, so it
// can be shown in English and edited by an operator.
func loadDeliveryKeys(e *engine) {
	// --- Dispatch: assigning a parcel to a representative ---
	addKey(e, "vendor.delivery.choose_courier", "commerce",
		"يرجى اختيار مندوب التوصيل الذي سيتم إسناد الطرد إليه.",
		"Choose the delivery representative this parcel goes to.",
		"Validation notice on the assignment form")
	addKey(e, "vendor.delivery.not_a_courier", "commerce",
		"الموظف المحدد ليس من مندوبي التوصيل في منشأتك.",
		"The selected employee is not one of your delivery representatives.",
		"Refusal when the posted courier is not a member holding the delivery grant")
	addKey(e, "vendor.delivery.assigned_success", "commerce",
		"تم إسناد الطرد %s إلى المندوب وإشعاره به.",
		"Parcel %s was assigned to the representative and they were notified.",
		"Success notice after assigning a parcel")
	addKey(e, "vendor.delivery.unassigned_success", "commerce",
		"تمت إعادة الطرد إلى قائمة الطرود غير المسندة.",
		"The parcel was returned to the unassigned pool.",
		"Success notice after unassigning a parcel")
	addKey(e, "vendor.delivery.assignment_note", "commerce",
		"تم إسناد الطرد إلى مندوب التوصيل رقم %d",
		"Parcel assigned to delivery representative #%d",
		"Audit trail entry recorded on assignment")
	addKey(e, "vendor.delivery.unassignment_note", "commerce",
		"تم سحب الطرد من المندوب وإعادته إلى قائمة الطرود غير المسندة",
		"Parcel taken back from the representative and returned to the unassigned pool",
		"Audit trail entry recorded on unassignment")

	// --- The representative working a parcel ---
	addKey(e, "vendor.delivery.shipment_not_found", "commerce",
		"لم يتم العثور على الطرد المطلوب ضمن شحنات منشأتك.",
		"That parcel was not found among your company's shipments.",
		"Error notice when a shipment id does not resolve")
	addKey(e, "vendor.delivery.status_updated_success", "commerce",
		"تم تحديث حالة الطرد بنجاح وإشعار الصيدلية.",
		"The parcel status was updated and the pharmacy was notified.",
		"Success notice after a courier advances a parcel")
	addKey(e, "vendor.delivery.handover_success", "commerce",
		"تم تأكيد تسليم الطرد بالكود وتوثيق العملية وإشعار الصيدلية والمورد.",
		"Handover confirmed with the delivery code; the pharmacy and the supplier were notified.",
		"Success notice after a verified handover")

	// --- Notifying the representative that a parcel is waiting ---
	addKey(e, "vendor.delivery.courier_notif_title", "commerce",
		"طرد جديد مسند إليك (%s)",
		"A new parcel is assigned to you (%s)",
		"Courier notification title")
	addKey(e, "vendor.delivery.courier_notif_body", "commerce",
		"تم إسناد الشحنة %s المتجهة إلى %s إليك. افتح «إدارة الشحنات» لبدء التوصيل.",
		"Shipment %s bound for %s has been assigned to you. Open Shipment Dispatch to start the delivery.",
		"Courier notification body")
}
