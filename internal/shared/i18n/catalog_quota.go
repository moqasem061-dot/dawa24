package i18n

// The per-branch purchase quota: حصص الفروع.
//
// A supplier caps how much any one buying branch may take of a variant, and
// resets that consumption when it wants to. Every string the feature shows —
// the refusals at the cart, the supplier's management screen, the form field on
// the item editor — is declared here so the whole thing can be read in English
// as well as Arabic, and so a wording change happens in one file.

func loadQuotaKeys(e *engine) {
	const ns = "quota"

	// The gate: what a pharmacy is told when the cap stops it.
	addKey(e, "quota.exhausted", ns,
		"استهلك هذا الفرع كامل الحصة المسموح بها من هذا الصنف (%d). لا يمكن طلب المزيد لنفس الفرع.",
		"This branch has already used its full quota of %d for this item.",
		"Cart refusal: branch quota fully consumed")
	addKey(e, "quota.exceeded", ns,
		"الحصة المتبقية لهذا الفرع من هذا الصنف %d فقط (الحد %d لكل فرع).",
		"Only %d left of this branch's quota for this item (limit %d per branch).",
		"Cart refusal: requested more than the branch has left")
	addKey(e, "quota.exhausted_named", ns,
		"استهلك هذا الفرع كامل الحصة المسموح بها من «%s» (%d لكل فرع).",
		"This branch has used its full quota of %[2]d for %[1]s.",
		"Checkout refusal naming the item")
	addKey(e, "quota.exceeded_named", ns,
		"الحصة المتبقية لهذا الفرع من «%s» هي %d فقط (الحد %d لكل فرع).",
		"Only %[2]d left of this branch's quota for %[1]s (limit %[3]d per branch).",
		"Checkout refusal naming the item")
	addKey(e, "quota.unnamed_item", ns, "هذا الصنف", "this item",
		"Fallback when a variant name cannot be read for a refusal message")
	addKey(e, "quota.branch_required", ns,
		"يجب تحديد فرع الاستلام لطلب صنف عليه حصة لكل فرع.",
		"Select a receiving branch before ordering an item that carries a per-branch quota.",
		"Checkout refusal: a quota item needs a branch to be charged to")

	// The item editor's field.
	addKey(e, "vendor.catalog.quota_label", ns, "حصة كل فرع", "Per-branch quota",
		"Label of the quota field on the item editor")
	addKey(e, "vendor.catalog.quota_hint", ns,
		"أقصى كمية يمكن لأي فرع مشترٍ شراؤها من هذا الصنف إجمالاً. اتركه فارغاً لإلغاء الحصة.",
		"The most any one buying branch may ever take of this item. Leave blank for no quota.",
		"Help text under the quota field")
	addKey(e, "vendor.catalog.invalid_quota", ns,
		"حصة الفرع يجب أن تكون رقماً صحيحاً.",
		"The per-branch quota must be a whole number.",
		"Rejected quota entry")
	addKey(e, "vendor.catalog.quota_too_large", ns,
		"حصة الفرع المدخلة كبيرة بشكل غير منطقي.",
		"That per-branch quota is unreasonably large.",
		"Rejected quota entry above the ceiling")

	// The supplier's management screen.
	addKey(e, "vendor.quota.title", ns, "حصص الفروع", "Branch quotas",
		"Page title")
	addKey(e, "vendor.quota.subtitle", ns,
		"حدّد أقصى كمية يشتريها كل فرع من كل صنف، وتابع استهلاك الفروع وحرّره عند الحاجة.",
		"Cap how much each branch may buy of each item, watch what they have taken, and reset it when needed.",
		"Page subtitle")
	addKey(e, "vendor.quota.tab_branches", ns, "استهلاك الفروع", "Branch usage",
		"Tab showing one row per buying branch")
	addKey(e, "vendor.quota.tab_variants", ns, "الأصناف المقيّدة", "Restricted items",
		"Tab showing one row per item that carries a quota")
	addKey(e, "vendor.quota.stat_variants", ns, "أصناف عليها حصة", "Items with a quota",
		"Stat card")
	addKey(e, "vendor.quota.stat_branches", ns, "فروع استهلكت", "Branches consuming",
		"Stat card")
	addKey(e, "vendor.quota.stat_exhausted", ns, "فروع استنفدت حصتها", "Branches at their limit",
		"Stat card")
	addKey(e, "vendor.quota.stat_units", ns, "إجمالي الكميات المستهلكة", "Units consumed",
		"Stat card")
	addKey(e, "vendor.quota.stat_releases", ns, "مرات التحرير", "Resets made",
		"Stat card")
	addKey(e, "vendor.quota.col_item", ns, "الصنف", "Item", "Table column")
	addKey(e, "vendor.quota.col_customer", ns, "المنشأة", "Company", "Table column")
	addKey(e, "vendor.quota.col_branch", ns, "الفرع", "Branch", "Table column")
	addKey(e, "vendor.quota.col_limit", ns, "الحصة", "Quota", "Table column")
	addKey(e, "vendor.quota.col_used", ns, "المستهلك", "Used", "Table column")
	addKey(e, "vendor.quota.col_remaining", ns, "المتبقي", "Remaining", "Table column")
	addKey(e, "vendor.quota.col_orders", ns, "الطلبات", "Orders", "Table column")
	addKey(e, "vendor.quota.col_last_order", ns, "آخر طلب", "Last order", "Table column")
	addKey(e, "vendor.quota.col_actions", ns, "إجراءات", "Actions", "Table column")
	addKey(e, "vendor.quota.col_branches", ns, "عدد الفروع", "Branches", "Table column")
	addKey(e, "vendor.quota.col_exhausted", ns, "استنفدت", "At limit", "Table column")
	addKey(e, "vendor.quota.release", ns, "تحرير الحصة", "Reset quota",
		"Button that frees a branch's consumption")
	addKey(e, "vendor.quota.release_confirm", ns,
		"سيتم تصفير استهلاك هذا الفرع من هذا الصنف، ليتمكن من الشراء مرة أخرى حتى الحد المسموح. هل تريد المتابعة؟",
		"This branch's consumption of this item will be reset to zero so it can buy up to the limit again. Continue?",
		"Confirmation before releasing")
	addKey(e, "vendor.quota.release_done", ns, "تم تحرير حصة الفرع.", "The branch's quota was reset.",
		"Toast after releasing")
	addKey(e, "vendor.quota.undo_release", ns, "تراجع عن التحرير", "Undo reset",
		"Button that restores a released consumption")
	addKey(e, "vendor.quota.undo_done", ns, "تم التراجع عن تحرير الحصة.", "The reset was undone.",
		"Toast after undoing a release")
	addKey(e, "vendor.quota.released_badge", ns, "محرَّرة", "Reset",
		"Badge on a row whose consumption the supplier has reset")
	addKey(e, "vendor.quota.released_units", ns, "أُعفي %d وحدة", "%d units forgiven",
		"How much the reset took off this branch's tally")
	addKey(e, "vendor.quota.exhausted_badge", ns, "استنفدت", "At limit",
		"Badge on a branch that has taken its whole allowance")
	addKey(e, "vendor.quota.set_limit", ns, "تعديل الحصة", "Change quota",
		"Button on the items tab")
	addKey(e, "vendor.quota.remove_limit", ns, "إلغاء الحصة", "Remove quota",
		"Button that lifts the cap from an item entirely")
	addKey(e, "vendor.quota.remove_confirm", ns,
		"سيتم إلغاء الحصة نهائياً عن هذا الصنف ويصبح متاحاً لكل الفروع بلا حد. هل تريد المتابعة؟",
		"The quota will be lifted from this item entirely and every branch may buy without a cap. Continue?",
		"Confirmation before removing a quota")
	addKey(e, "vendor.quota.limit_saved", ns, "تم حفظ الحصة.", "The quota was saved.",
		"Toast after setting a quota")
	addKey(e, "vendor.quota.limit_removed", ns, "تم إلغاء الحصة عن الصنف.", "The quota was removed from the item.",
		"Toast after removing a quota")
	addKey(e, "vendor.quota.empty", ns,
		"لا توجد حصص بعد. حدّد حصة لأي صنف من صفحة الأصناف ليظهر استهلاك الفروع هنا.",
		"No quotas yet. Set one on any item and branch usage will appear here.",
		"Empty state")
	addKey(e, "vendor.quota.empty_branches", ns,
		"لم يشترِ أي فرع من الأصناف المقيّدة حتى الآن.",
		"No branch has bought a restricted item yet.",
		"Empty state on the branch tab")
	addKey(e, "vendor.quota.filter_all_items", ns, "كل الأصناف", "All items", "Filter option")
	addKey(e, "vendor.quota.filter_all_customers", ns, "كل المنشآت", "All companies", "Filter option")
	addKey(e, "vendor.quota.filter_all_branches", ns, "كل الفروع", "All branches", "Filter option")
	addKey(e, "vendor.quota.filter_all_states", ns, "كل الحالات", "Any state", "Filter option")
	addKey(e, "vendor.quota.filter_exhausted", ns, "استنفدت الحصة", "At the limit", "Filter option")
	addKey(e, "vendor.quota.filter_active", ns, "لديها رصيد", "Still has room", "Filter option")
	addKey(e, "vendor.quota.filter_released", ns, "تم تحريرها", "Previously reset", "Filter option")
	addKey(e, "vendor.quota.search_placeholder", ns, "ابحث بالصنف أو الفرع أو المنشأة...",
		"Search by item, branch or company...", "Filter box placeholder")
	addKey(e, "vendor.quota.unlimited", ns, "بلا حد", "Unlimited",
		"Shown where an item has no quota")
	addKey(e, "vendor.quota.new_limit", ns, "الحصة الجديدة لكل فرع", "New per-branch quota",
		"Label in the change-quota dialog")
}
