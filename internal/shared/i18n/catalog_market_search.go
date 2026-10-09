package i18n

func loadMarketSearchKeys(e *engine) {
	addKey(e, "compare.discount.lower", "compare", "خصمك أقل من السوق", "Your discount is below the market", "Discount comparison")
	addKey(e, "compare.discount.higher", "compare", "خصمك أفضل من السوق", "Your discount is above the market", "Discount comparison")
	addKey(e, "compare.discount.competitors", "compare", "عروض السوق بخصم أعلى", "Market offers with a higher discount", "Discount comparison")

	addKey(e, "market.search.placeholder", "compare", "ابحث باسم الصنف في الكتالوج أو الملف…", "Search catalog or file product names…", "Market product search")
	addKey(e, "market.search.relevance", "compare", "الأكثر صلة بالبحث", "Most relevant", "Market search sort")
	addKey(e, "market.search.file_name", "compare", "في الملف:", "In the file:", "Original supplier product name")
	addKey(e, "market.search.unavailable", "compare", "تعذر تحميل النتائج. يرجى المحاولة مرة أخرى.", "Results could not be loaded. Please try again.", "Search failure")
}
