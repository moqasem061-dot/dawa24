package i18n

func loadWarehouseReviewKeys(e *engine) {
	addKey(e, "warehouse.review.button", "compare", "مراجعة وحفظ أعمدة جميع المستودعات", "Review and save columns for all warehouses", "Bulk review action")
	addKey(e, "warehouse.review.scope", "compare", "يشمل جميع المستودعات النشطة والمؤرشفة في المنصة، بغض النظر عن فلاتر الصفحة. يحفظ الأعمدة ويعيد مطابقة جميع الأصناف مع الحفاظ على الأرشفة.", "Includes all active and archived warehouses across the platform regardless of page filters. Saves columns and rematches every product while preserving archive status.", "Bulk review scope")
	addKey(e, "warehouse.review.busy", "compare", "مراجعة جميع المستودعات قيد التشغيل بالفعل. انتظر اكتمالها.", "A platform-wide warehouse review is already running. Wait for it to finish.", "Duplicate run")
	addKey(e, "warehouse.review.start_failed", "compare", "تعذر بدء المراجعة. تحقق من وجود مستودعات ثم حاول مجدداً.", "Could not start the review. Check that warehouses exist and try again.", "Start failure")
	addKey(e, "warehouse.review.partial", "compare", "تحتاج بعض الملفات إلى مراجعة يدوية أو إعادة المحاولة. تم الاحتفاظ بالبيانات السابقة للملفات التي تعذر حفظها.", "Some files need manual review or a retry. Previous data was retained for files that could not be saved.", "Partial failure")
	addKey(e, "warehouse.review.finished", "compare", "اكتملت مراجعة الأعمدة وحفظها وإعادة مطابقة الأصناف لجميع المستودعات.", "Column review, saving and product rematching completed for every warehouse.", "Completed")
	addKey(e, "warehouse.review.running", "compare", "المراجعة تعمل في الخلفية. يمكنك مغادرة الصفحة والعودة إليها. إذا توقف الخادم، اضغط زر المراجعة لاستكمال الملفات المتبقية.", "Review is running in the background. You can leave and return. If the server stops, press the review button to resume remaining files.", "Running")
	addKey(e, "warehouse.review.counts", "compare", "تمت مراجعة %d من %d مستودع. تم الحفظ: %d. تحتاج مراجعة: %d.", "Reviewed %d of %d warehouses. Saved: %d. Need review: %d.", "Progress counters")
	addKey(e, "warehouse.review.file", "compare", "مراجعة المستودع رقم %d", "Review warehouse #%d", "Manual review link")
	addKey(e, "warehouse.review.back", "compare", "العودة إلى المستودعات", "Back to warehouses", "Return link")
}
