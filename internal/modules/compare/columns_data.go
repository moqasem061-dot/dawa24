package compare

// HeaderAliases maps target fields to their known Arabic and English/Technical column header variations.
// Extracted with 100% parity from Laravel's ColumnDetector.php (Plan V5 Phase 2 Task 2.3).
var HeaderAliases = map[TargetField][]string{
	FieldProductID: {
		// Arabic
		"رقم الصنف", "رقم المنتج", "معرف المنتج", "رقم", "كود", "المعرف", "تسلسل",
		"رقم تعريفي", "الرقم التسلسلي", "رقم السجل", "رقم العنصر", "معرف السجل",
		"رقم مرجعي", "الرقم المرجعي", "رمز تعريفي", "رقم الملف", "معرف الصنف",
		"رقم البند", "رقم الطلب", "الرقم المتسلسل", "رقم الصف", "كود تعريفي",
		"معرف السلعة", "رقم السلعة", "رقم الوحدة", "الترقيم", "م",
		// English / Technical
		"Product ID", "ID", "Product Code", "product_id", "id", "PID", "Product_ID",
		"Internal ID", "Ref ID", "Item ID", "Item Code", "Record ID", "Entry Number",
		"Row ID", "Index", "No.", "Number", "Product Number", "Item Number", "ProdID",
		"prod_id", "productId", "itemId", "Line ID", "Row Number", "Seq", "Seq No",
		"Serial", "Serial No", "External Product ID", "Master ID",
	},
	FieldProductName: {
		// Arabic
		"اسم الصنف", "إسم الصنف", "أسم الصنف", "الاسم", "الإسم", "اسم المنتج",
		"إسم المنتج", "صنف", "الاسم العربي", "اسم", "إسم", "المسمى", "العنوان",
		"اسم المادة", "بيان الصنف", "وصف الصنف", "اسم السلعة", "إسم السلعة",
		"مسمى الصنف", "مسمى المنتج", "الاسم التجاري", "اسم العرض", "عنوان المنتج",
		"عنوان الصنف", "الصنف", "اسم البند", "تسمية الصنف", "الاسم الكامل",
		"اسم القطعة", "اسم الموديل", "الاسم بالعربي", "الاسم بالانجليزي", "اسم الباقة",
		// English / Technical
		"Product Name", "Name", "product_name", "name", "Title", "Product Title",
		"Item Name", "Label", "Product Label", "Description Name", "p_name",
		"prod_name", "item_name", "Name EN", "productName", "itemName",
		"Item Title", "Display Name", "Full Name", "Brand Name", "Commercial Name",
		"Name AR", "Name (EN)", "Name (AR)", "English Name", "Arabic Name",
		"Item Label", "Catalog Name", "Listing Title", "Model Name",
	},
	FieldDescription: {
		// Arabic
		"الوصف", "وصف", "التفاصيل", "شرح", "وصف المنتج", "تفاصيل الصنف",
		"عن المنتج", "نبذة", "الشرح", "وصف تفصيلي", "ملاحظات الوصف",
		"بيان تفصيلي", "مواصفات المنتج", "المواصفات", "الوصف الكامل",
		"الوصف المختصر", "شرح المنتج", "تفاصيل إضافية", "ملاحظات المنتج", "بيانات المنتج",
		// English / Technical
		"Description", "description", "desc", "Summary", "Details",
		"Product Description", "Info", "Note", "Content", "Long Description",
		"Short Description", "Overview", "Specs", "Specifications", "About",
		"Product Details", "Item Description", "Body", "Text", "Full Description",
		"Extra Info", "Additional Details", "Product Info", "Remarks", "Comments",
	},
	FieldPrice: {
		// Arabic
		"السعر", "سعر", "السعر الأساسي", "سعر البيع", "القيمة", "سعر الوحدة",
		"التكلفة للبيع", "سعر الجملة", "سعر التجزئة", "السعر بالجنيه", "السعر بالدولار",
		"السعر الأصلي", "سعر قبل الخصم", "سعر الشراء", "التسعيرة", "المبلغ",
		"سعر بعد الخصم", "السعر النهائي", "سعر الكرتونة", "سعر القطعة",
		"سعر شامل الضريبة", "السعر شامل الضريبة", "سعر غير شامل الضريبة",
		// English / Technical
		"Price", "price", "Base Price", "Sale Price", "Selling Price",
		"Unit Price", "Amount", "Rate", "Value", "MSRP", "List Price",
		"Retail Price", "Wholesale Price", "Original Price", "Regular Price",
		"Price Before Discount", "Net Price", "Gross Price", "Price (USD)",
		"Price (EGP)", "Final Price", "Price incl. Tax", "Price excl. Tax",
		"Selling Rate", "Tag Price",
	},
	FieldCostPrice: {
		// Arabic
		"سعر التكلفة", "التكلفة", "سعر تكلفة",
		// English / Technical
		"Cost Price", "cost_price", "Cost", "cost", "Unit Cost", "unit_cost",
		"Purchase Price", "purchase_price",
	},
	FieldDiscount: {
		// Arabic
		"الخصم", "خصم", "نسبة الخصم", "التخفيض", "قيمة الخصم", "مقدار الخصم",
		"اوفر", "العرض", "التنزيل", "نسبة التخفيض", "الخصم %", "تخفيض السعر",
		"الحسم", "نسبة الحسم", "خصم كمي", "خصم نقدي", "عرض خاص", "تخفيضات",
		// English / Technical
		"Discount", "discount", "Discount %", "Reduction", "Offer", "Promo",
		"Discount Amount", "Percentage", "Discount Rate", "Sale %", "Off",
		"Markdown", "Discount Value", "Coupon", "Deal", "Promo Code",
		"Discount Percent", "Savings", "Special Offer", "Clearance",
	},
	FieldQuantity: {
		// Arabic
		"الكمية", "كمية", "حجم اساسي", "الحجم الأساسي", "المخزون", "العدد",
		"الرصيد", "كمية المخزن", "المتوفر", "الكمية المتاحة", "كمية متوفرة",
		"المخزون المتاح", "عدد القطع", "الوحدات", "الكمية بالمخزن", "كمية البضاعة",
		"الكمية الموجودة", "رصيد المخزن", "عدد الوحدات", "الكمية بالكرتونة",
		"كمية التعبئة", "المخزون الحالي",
		// English / Technical
		"Quantity", "quantity", "qty", "Stock", "Inventory", "Stock Level",
		"Available Qty", "Amount", "Count", "Balance", "Qty Available",
		"On Hand", "Units", "Stock Qty", "In Stock", "Available Stock",
		"Units Available", "Current Stock", "Warehouse Qty", "Pack Qty",
		"Box Qty", "Reserved Qty", "Free Stock",
	},
	FieldSKU: {
		// Arabic
		"SKU", "رمز المنتج", "الرمز", "Product Code", "sku", "code",
		"كود المنتج", "رمز", "الترميز", "رقم القطعة", "رمز الصنف", "كود الصنف",
		"الكود", "رمز التخزين", "كود المخزون", "رمز الموديل", "كود الموديل",
		"الرمز التخزيني", "رقم الموديل",
		// English / Technical
		"Product SKU", "Stock Keeping Unit", "SKU Number", "Part Number",
		"Model Number", "Ref", "Reference", "SKU Code", "Item Code",
		"Model No", "Product Model", "Style Number", "Variant Code",
		"SKU ID", "Vendor SKU", "Supplier Code", "Catalog Number",
		"Article Number", "Article No",
	},
	FieldUniqueID: {
		// Arabic
		"الرقم الفريد", "رقم فريد", "المعرف الوحيد", "كود مميز", "المعرف الفريد",
		"الرقم التسلسلي الفريد", "كود فريد", "الرقم الاستثنائي", "المعرف الخاص", "الكود الفريد",
		// English / Technical
		"Unique ID", "unique_id", "Unique Code", "UUID", "GUID", "Entry ID",
		"Record ID", "External ID", "System ID", "Global ID", "uid", "uuid",
		"Object ID", "Primary Key", "PK", "Hash ID", "Slug",
	},
	FieldBarcode: {
		// Arabic
		"الباركود", "Barcode", "باركود", "barcode", "Bar Code", "رمز الباركود",
		"الرمز الشريطي", "رقم الباركود", "كود الباركود", "الرمز العالمي",
		"باركود المنتج", "رمز التعريف الشريطي", "الترميز الشريطي",
		// English / Technical
		"UPC", "EAN", "GTIN", "Serial Number", "S/N", "Bar_Code", "ISBN",
		"EAN-13", "UPC-A", "QR Code", "Scan Code", "Barcode Number",
		"EAN Code", "UPC Code", "Product Barcode", "Scanning Code", "GS1 Code",
	},
	FieldAlertPrice: {
		// Arabic
		"سعر التنبيه", "سعر تنبيه", "تنبيه السعر", "أقل سعر", "الحد الأدنى للسعر",
		// English / Technical
		"Alert Price", "alert_price", "Price Alert", "Minimum Price",
		"Min Price", "Warning Price", "Threshold Price",
	},
	FieldAlertDiscount: {
		// Arabic
		"خصم التنبيه (%)", "Alert Discount", "alert_discount", "خصم التنبيه",
		"تنبيه الخصم", "أقصى خصم",
		// English / Technical
		"Discount Alert", "Max Discount", "Warning Discount", "Alert Disc",
	},
}
