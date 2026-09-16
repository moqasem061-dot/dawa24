package i18n

// تقرير التوفير الاستراتيجي — the generated commentary on the market
// intelligence screen.
//
// Every one of these is a sentence a supplier reads about their own market, so
// each carries real English as well as Arabic rather than the Arabic twice.
// They are format strings: the arguments are the figures the report computed,
// and the order of the verbs differs between the two languages, so the
// placeholders are not interchangeable between them — check both when editing
// one.
func loadMarketIntelKeys(e *engine) {
	// The separator between items in a generated list. Arabic uses ، and
	// English a comma, and joining with a hardcoded one leaves the wrong mark
	// in whichever language did not write it.
	addKey(e, "common.list_separator", "common", "، ", ", ", "List separator")

	// --- empty market -----------------------------------------------------
	addKey(e, "market.intel.empty.no_offers", "compare",
		"لا توجد عروض منشورة في السوق حالياً. تظهر بيانات هذا التقرير بعد رفع قوائم موردين ومشاركتها كمستودعات مؤقتة أو عروض عامة.",
		"No offers are published in the market yet. This report fills in once supplier lists are uploaded and shared as temporary warehouses or public offers.",
		"Market intelligence: nothing published at all")
	addKey(e, "market.intel.empty.single_supplier", "compare",
		"السوق يحتوي على %d عرضاً من مورد واحد فقط، ولا تتوفر مقارنة قبل وجود موردين اثنين على الأقل لنفس الصنف.",
		"The market holds %d offers from a single supplier. A comparison needs at least two suppliers quoting the same item.",
		"Market intelligence: one supplier only")
	addKey(e, "market.intel.empty.nothing_shared", "compare",
		"تم رصد %d عرضاً عبر %d مورد، لكن لا يوجد صنف واحد يعرضه أكثر من مورد. غالباً لم تُشغَّل مطابقة الأصناف على الملفات المرفوعة: بدونها يُعامَل كل اختلاف في كتابة الاسم كصنف مستقل.",
		"%d offers across %d suppliers, but no single item is quoted by more than one of them. Item matching has most likely not been run on the uploaded files: without it every spelling of a name is treated as a separate product.",
		"Market intelligence: no product is quoted twice")

	// --- analysis ---------------------------------------------------------
	addKey(e, "market.intel.analysis.savings", "compare",
		"شراء كل صنف من أرخص مورد له يخفض تكلفة سلة من %s إلى %s، أي وفر قدره %s (%.1f%%) على %d صنفاً قابلاً للمقارنة.",
		"Buying each item from its cheapest supplier takes a basket from %s down to %s — a saving of %s (%.1f%%) across %d comparable items.",
		"Market intelligence: headline saving")
	addKey(e, "market.intel.analysis.no_spread", "compare",
		"الأسعار متقاربة عبر الموردين على %d صنفاً قابلاً للمقارنة، ولا يوجد فارق تكلفة يُذكر بين الشراء المجزأ والشراء الموحد.",
		"Prices are close across suppliers on %d comparable items; there is no meaningful cost difference between split and consolidated purchasing.",
		"Market intelligence: no meaningful spread")
	addKey(e, "market.intel.analysis.coverage", "compare",
		"من %d صنف مرصود، %d صنفاً يعرضه أكثر من مورد و%d صنفاً متاح لدى مورد واحد فقط — هذه الأخيرة لا تقبل المقارنة وتمثل نقاط ارتكاز تفاوضية أو نواقص سوق.",
		"Of %d tracked items, %d are quoted by more than one supplier and %d by only one. The latter cannot be compared and represent either negotiating leverage or a gap in the market.",
		"Market intelligence: comparable versus single-sourced")
	addKey(e, "market.intel.analysis.matched_high", "compare",
		"%.0f%% من عروض السوق مرتبطة بالكتالوج المركزي (%d من %d)، فالمقارنة تتم على مستوى الصنف نفسه وليس على تشابه كتابة الاسم.",
		"%.0f%% of market offers are linked to the shared catalogue (%d of %d), so the comparison is item-for-item rather than name-for-name.",
		"Market intelligence: good catalogue coverage")
	addKey(e, "market.intel.analysis.matched_low", "compare",
		"%.0f%% فقط من عروض السوق مرتبطة بالكتالوج المركزي (%d من %d). تشغيل المطابقة على بقية الملفات يرفع دقة هذا التقرير مباشرة.",
		"Only %.0f%% of market offers are linked to the shared catalogue (%d of %d). Running matching on the remaining files raises this report's accuracy directly.",
		"Market intelligence: partial catalogue coverage")
	addKey(e, "market.intel.analysis.matched_none", "compare",
		"لم تُربط أي من عروض السوق (%d عرضاً) بالكتالوج المركزي بعد، والمقارنة الحالية مبنية على تطابق كتابة الأسماء وحدها. شغّل المطابقة على الملفات لرفع الدقة.",
		"None of the %d market offers are linked to the shared catalogue yet, so this comparison rests on spelling alone. Run matching on the files to improve it.",
		"Market intelligence: no catalogue coverage")
	addKey(e, "market.intel.analysis.rejected", "compare",
		"استُبعد %d صفاً من المقارنة لأن سعره النهائي صفر — غالباً عمود «الخصم» في الملف لا يحمل نسبة خصم.",
		"%d rows were excluded because their net price is zero — usually a file whose discount column does not hold a discount.",
		"Market intelligence: rows excluded as unusable")
	addKey(e, "market.intel.analysis.rejected_worst", "compare",
		" الأكثر تأثراً: ",
		" Most affected: ",
		"Market intelligence: lead-in to the worst-affected supplier list")
	addKey(e, "market.intel.analysis.rejected_entry", "compare",
		"%s (%d صف)",
		"%s (%d rows)",
		"Market intelligence: one worst-affected supplier")
	addKey(e, "market.intel.analysis.rejected_fix", "compare",
		". أعد ربط أعمدة هذه الملفات من مركز الملفات.",
		". Re-map those files' columns from the file centre.",
		"Market intelligence: how to fix the excluded rows")
	addKey(e, "market.intel.analysis.ai_off", "compare",
		"مطابقة الذكاء الاصطناعي غير مفعّلة على المنصة حالياً، والمطابقة تعمل بالقواعد الحتمية وحدها. تفعيلها يزيد عدد الأصناف المرتبطة بالكتالوج وبالتالي دقة هذا التقرير.",
		"AI matching is not enabled on this platform, so matching runs on the deterministic rules alone. Enabling it links more items to the catalogue and raises this report's accuracy.",
		"Market intelligence: AI tier unavailable")

	// --- advice -----------------------------------------------------------
	addKey(e, "market.intel.advice.negotiate", "compare",
		"أكبر فرصة تفاوض: «%s» — %s لدى %s مقابل %s لدى %s، بفارق %s للوحدة (%.1f%%). اعرض سعر السوق الأفضل على الأغلى للحصول على مطابقة سعرية.",
		"Biggest negotiating opportunity: \"%s\" — %s from %s against %s from %s, a gap of %s per unit (%.1f%%). Put the better market price to the dearer supplier and ask them to match it.",
		"Market intelligence: largest negotiable gap")
	addKey(e, "market.intel.advice.dearest", "compare",
		"«%s» أغلى من أفضل عرض بالسوق بمتوسط %.1f%% على %d صنفاً يعرضها. سلة مشترياتك منه تكلف %s بينما تكلف %s عند الشراء الأمثل — فارق %s.",
		"\"%s\" is on average %.1f%% above the market's best price across the %d items they quote. That basket costs %s from them against %s bought optimally — a difference of %s.",
		"Market intelligence: systematically dearer supplier")
	addKey(e, "market.intel.advice.discount_gap", "compare",
		"أكبر فارق في شروط الخصم: «%s» بخصم %.1f%% لدى %s مقابل %.1f%% لدى %s — فارق %.1f نقطة يصلح كأساس لمراجعة شروط التعاقد.",
		"Widest gap in discount terms: \"%s\" at %.1f%% from %s against %.1f%% from %s — %.1f points, and a basis for reopening the contract terms.",
		"Market intelligence: widest discount gap")
	addKey(e, "market.intel.advice.exclusives", "compare",
		"%d صنفاً متاح لدى مورد واحد فقط (يُعرض منها أعلى %d قيمةً أدناه). راجعها كفرص لتأمين مصدر بديل، أو كأصناف يمكن التفاوض عليها من موقع الحاجة لا من موقع المقارنة.",
		"%d items are single-sourced (the %d most valuable are listed below). Treat them as candidates for a second supplier, or as items negotiated from need rather than from comparison.",
		"Market intelligence: single-sourced items")

	// --- guidance ---------------------------------------------------------
	addKey(e, "market.intel.guidance.best_supplier", "compare",
		"ابدأ من «%s»: يعرض %d صنفاً من الأصناف المقارَنة، وهو الأرخص في %d منها (%.0f%%)، بمتوسط فارق %.1f%% عن أفضل سعر بالسوق ومتوسط خصم %.1f%%.",
		"Start with \"%s\": they quote %d of the comparable items and are cheapest on %d of them (%.0f%%), averaging %.1f%% above the market's best price and a %.1f%% discount.",
		"Market intelligence: where to start buying")
	addKey(e, "market.intel.guidance.concentration", "compare",
		"أعلى %d صنفاً في الجدول أدناه تمثل %.0f%% من إجمالي الوفر الممكن. معالجتها وحدها تحقق معظم المكسب دون إعادة توزيع كل المشتريات.",
		"The top %d items in the table below account for %.0f%% of the total available saving. Addressing those alone captures most of it without redistributing the whole purchase.",
		"Market intelligence: where the saving is concentrated")
	addKey(e, "market.intel.guidance.split_versus_single", "compare",
		"الشراء المجزأ يوزّع الطلب على %d مورداً على الأقل مقابل وفر %s. إذا كانت تكلفة التعامل مع موردين متعددين أعلى من ذلك، فالشراء الموحد من «%s» هو الخيار الأفضل عملياً.",
		"Split purchasing spreads the order across at least %d suppliers for a saving of %s. If dealing with that many suppliers costs more than that, consolidating on \"%s\" is the better practical choice.",
		"Market intelligence: split versus consolidated purchasing")
}
