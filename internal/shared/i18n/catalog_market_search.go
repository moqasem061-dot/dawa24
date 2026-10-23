package i18n

func loadMarketSearchKeys(e *engine) {
	addKey(e, "vendor.pricing.input_type", "catalog", "طريقة إدخال الخصم", "Discount input", "Vendor pricing")
	addKey(e, "vendor.pricing.sale_price", "catalog", "سعر البيع للصيدلية", "Sale price to pharmacy", "Vendor pricing")
	addKey(e, "vendor.pricing.sale_help", "catalog", "عند اختيار سعر البيع، نحسب نسبة الخصم من سعر الجمهور تلقائيًا ونحفظها بدلًا من نسبة الخصم المدخلة. تُحفظ النسبة بمنزلتين عشريتين ويُحسب السعر النهائي منها. مثال: جمهور ١٠٠ وبيع ٨٠ = خصم ٢٠٪.", "When sale price is selected, we calculate and save the discount from the public price. The percentage is rounded to two decimals and determines the final price. Example: public 100, sale 80 = 20% off.", "Vendor pricing")
	addKey(e, "vendor.pricing.invalid", "catalog", "أدخل خصمًا من ٠ إلى ١٠٠٪، أو سعر بيع من صفر حتى سعر الجمهور، مع سعر جمهور أكبر من صفر.", "Enter a discount from 0 to 100%, or a sale price between zero and a positive public price.", "Vendor pricing validation")
	addKey(e, "compare.discount.lower", "compare", "خصمك أقل من السوق", "Your discount is below the market", "Discount comparison")
	addKey(e, "compare.discount.higher", "compare", "خصمك أفضل من السوق", "Your discount is above the market", "Discount comparison")
	addKey(e, "compare.discount.competitors", "compare", "عروض السوق بخصم أعلى", "Market offers with a higher discount", "Discount comparison")

	addKey(e, "market.search.placeholder", "compare", "ابحث باسم الصنف في الكتالوج أو الملف…", "Search catalog or file product names…", "Market product search")
	addKey(e, "market.search.relevance", "compare", "الأكثر صلة بالبحث", "Most relevant", "Market search sort")
	addKey(e, "market.search.file_name", "compare", "في الملف:", "In the file:", "Original supplier product name")
	addKey(e, "market.search.unavailable", "compare", "تعذر تحميل النتائج. يرجى المحاولة مرة أخرى.", "Results could not be loaded. Please try again.", "Search failure")
}
