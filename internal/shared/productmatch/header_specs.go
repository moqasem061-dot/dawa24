package productmatch

import ()

// headerSpecs is ordered only for reading; resolution is order independent.
//
// Entries are written the natural way — "الشكل الدوائي", "Item No." — and
// folded through sheet.NormalizeKey at init, exactly as the header cells they
// are compared against are. Hand-folding them in source was tried in the admin
// importer and got the hamza forms wrong, which cost "الشكل الدوائي" its exact
// match and dropped it to a weak partial.
var headerSpecs = []headerSpec{
	{
		field: FieldName,
		exact: []string{
			"اسم الصنف", "إسم الصنف", "اسم المنتج", "اسم الدواء", "الصنف", "صنف",
			"المنتج", "الاسم", "اسم المستحضر", "المستحضر", "الاسم التجاري",
			"اسم الصنف التجاري", "بيان الصنف", "وصف الصنف", "الوصف",
			"item description", "item desc", "itemname", "item name", "description",
			"product name", "product", "name", "trade name", "medicine",
		},
		strong: []string{
			"اسم الصنف", "اسم المنتج", "اسم الدواء", "اسم المستحضر",
			"الاسم التجاري", "اسم الصنف التجاري", "وصف الصنف", "بيان الصنف",
			"item description", "product name", "item name", "product description",
		},
		weak: []string{"اسم", "الصنف", "المنتج", "description", "name", "item"},
		blocked: []string{
			"كود", "code", "باركود", "barcode", "سعر", "price", "خصم", "discount",
			"كميه", "qty", "quantity", "الشركه", "المصنع", "المورد", "manufacturer",
			"vendor", "supplier", "التصنيف", "category", "الوحده", "unit",
			"الحاله", "status", "علمي", "generic", "scientific", "english",
			"انجليز", "تاريخ", "date", "الفرع", "branch", "المخزن",
		},
	},
	{
		field: FieldNameEN,
		exact: []string{
			"name en", "english name", "english", "اسم بالانجليزي",
			"اسم الصنف بالانجليزي", "الاسم بالانجليزية", "الاسم الانجليزي",
			"الاسم اللاتيني",
		},
		strong:  []string{"english name", "english description", "اسم بالانجليز", "الاسم بالانجليز", "اسم لاتيني"},
		weak:    []string{"english", "انجليز", "latin", "لاتيني"},
		blocked: []string{"generic", "scientific", "علمي", "سعر", "price"},
	},
	{
		field: FieldSKU,
		exact: []string{
			"الكود", "كود", "كود الصنف", "كود المنتج", "رقم الصنف", "رقم المنتج",
			"كود المورد", "كود الشركه", "رقم الكود", "code", "sku", "item no",
			"item code", "item id", "product code", "material code", "article code",
			"ref", "reference", "id",
		},
		strong: []string{
			"كود الصنف", "كود المنتج", "رقم الصنف", "رقم المنتج", "كود المورد",
			"item code", "item no", "product code", "material code",
		},
		weak: []string{"كود", "code", "sku", "رقم", "ref"},
		blocked: []string{
			"باركود", "barcode", "ean", "upc", "gtin", "التشغيله", "batch", "lot",
			"الفرع", "المخزن", "تصنيف", "category",
			// A column the file calls a price, a discount, a quantity or a date
			// is not the item code, however code-shaped its values look. Without
			// these, a three-column file of name / price / discount offers the
			// price column as an item-code candidate and the review screen warns
			// about a competition that does not exist.
			"سعر", "price", "ثمن", "خصم", "discount", "الكميه", "quantity",
			"الرصيد", "stock", "تاريخ", "date", "صلاحيه", "expiry",
		},
	},
	{
		field:   FieldBarcode,
		exact:   []string{"الباركود", "باركود", "الباركود الدولي", "باركود دولي", "barcode", "bar code", "ean", "ean13", "upc", "gtin"},
		strong:  []string{"الباركود الدولي", "باركود دولي", "رقم الباركود", "barcode", "ean 13", "gtin"},
		weak:    []string{"باركود", "barcode", "ean", "upc", "gtin"},
		blocked: []string{},
	},
	{
		// Matched on the header alone. Its values are integers indistinguishable
		// from a quantity or an item code, so there is nothing for value
		// evidence to add — and a wrong guess links a row to an arbitrary
		// product, which is the most expensive mistake this column can make.
		field:   FieldProductID,
		exact:   []string{"معرف المنتج", "معرف الصنف", "معرف صنف", "product id", "productid", "product_id", "dawa id", "dawa24 id", "pid"},
		strong:  []string{"معرف المنتج", "معرف الصنف", "product id", "dawa24 id", "رقم المنتج في المنصة", "رقم الصنف في المنصة"},
		weak:    []string{"معرف", "product id", "pid"},
		blocked: []string{"باركود", "barcode", "المورد", "supplier"},
	},
	{
		field:   FieldScientific,
		exact:   []string{"الاسم العلمي", "الاسم العلمى", "علمي", "generic name", "generic", "scientific name", "scientific", "inn"},
		strong:  []string{"الاسم العلمي", "generic name", "scientific name"},
		weak:    []string{"علمي", "generic", "scientific"},
		blocked: []string{"سعر", "price"},
	},
	{
		// Split from the scientific name rather than sharing its synonyms.
		// catalog.products keeps both columns, and a master-catalogue file that
		// carries both had them competing for one field: whichever column came
		// first won and the other was left unmapped.
		field:   FieldActiveIngredient,
		exact:   []string{"المادة الفعالة", "المادة النشطة", "الماده الفعاله", "المكون الفعال", "active ingredient", "active substance", "active"},
		strong:  []string{"المادة الفعالة", "المكون الفعال", "active ingredient", "active substance"},
		weak:    []string{"فعاله", "ingredient"},
		blocked: []string{"سعر", "price", "الحاله", "status"},
	},
	{
		field: FieldManufacturer,
		exact: []string{
			"الشركة", "الشركة المصنعة", "المصنع", "الشركه المصنعه", "جهة التصنيع",
			"الماركة", "البراند", "العلامة التجارية", "المورد", "اسم المورد",
			"manufacturer", "company", "brand", "vendor", "supplier", "maker",
			"preferred vendor", "producer", "agent",
		},
		strong:  []string{"الشركة المصنعة", "جهة التصنيع", "اسم الشركة", "اسم المورد", "شركة التصنيع", "preferred vendor", "manufacturer name"},
		weak:    []string{"الشركه", "المصنع", "المورد", "الماركه", "company", "brand", "vendor", "supplier"},
		blocked: []string{"كود", "code", "id", "رقم", "سعر", "price"},
	},
	{
		field:   FieldDosageForm,
		exact:   []string{"الشكل الصيدلي", "الشكل الدوائي", "شكل الدواء", "الشكل", "هيئة الدواء", "dosage form", "pharmaceutical form", "form", "dosage"},
		strong:  []string{"الشكل الصيدلي", "الشكل الدوائي", "هيئة الدواء", "dosage form", "pharmaceutical form"},
		weak:    []string{"الشكل", "form", "dosage"},
		blocked: []string{"format", "platform", "performance", "سعر", "price"},
	},
	{
		field:   FieldConcentration,
		exact:   []string{"التركيز", "تركيز", "القوة", "العيار", "الجرعة", "concentration", "strength", "dose", "potency"},
		strong:  []string{"تركيز الدواء", "قوة التركيز", "الجرعة الدوائية", "concentration"},
		weak:    []string{"تركيز", "strength", "dose", "الجرعه", "عيار"},
		blocked: []string{"سعر", "price"},
	},
	{
		field:   FieldUnit,
		exact:   []string{"الوحدة", "وحدة القياس", "العبوة", "نوع العبوة", "التعبئة", "unit", "uom", "pack", "packaging", "package"},
		strong:  []string{"وحدة القياس", "نوع العبوة", "unit of measure", "packaging type"},
		weak:    []string{"الوحده", "العبوه", "unit", "uom", "pack"},
		blocked: []string{"سعر الوحده", "unit price", "unit cost", "عدد", "count", "size", "حجم"},
	},
	{
		field:   FieldPackSize,
		exact:   []string{"عدد الوحدات", "حجم العبوة", "عدد الاقراص", "عدد بالعبوة", "pack size", "units per pack", "qty per pack", "size"},
		strong:  []string{"عدد الوحدات", "حجم العبوة", "عدد الاقراص", "pack size", "units per pack"},
		weak:    []string{"عدد", "size"},
		blocked: []string{"سعر", "price", "الكميه", "quantity", "الرصيد"},
	},
	{
		field:   FieldCategory,
		exact:   []string{"التصنيف", "الفئة", "تصنيف", "فئة", "القسم", "المجموعة", "فئة المنتج", "تصنيف المنتج", "نوع المنتج", "category", "group", "department", "class"},
		strong:  []string{"فئة المنتج", "تصنيف المنتج", "القسم الرئيسي", "المجموعة الرئيسية", "product category"},
		weak:    []string{"التصنيف", "الفئه", "القسم", "المجموعه", "category", "group"},
		blocked: []string{"كود", "code", "id", "sub"},
	},

	{
		field: FieldPublicPrice,
		exact: []string{
			"سعر الجمهور", "سعر ج", "سعر جمهور", "السعر", "سعر", "سعر البيع للجمهور",
			"السعر العام", "سعر قبل الخصم", "سعر العبوة", "سعر الوحدة", "سعر الصنف",
			"public price", "list price", "mrp", "retail price", "price", "unit price",
		},
		strong: []string{
			"سعر الجمهور", "سعر البيع للجمهور", "سعر قبل الخصم", "السعر العام",
			"public price", "retail price", "list price",
		},
		weak:    []string{"سعر", "price", "ثمن", "mrp"},
		blocked: []string{"تكلفه", "شراء", "cost", "purchase", "صافي", "net", "بعد الخصم", "خصم", "discount"},
	},
	{
		field:   FieldPrice,
		exact:   []string{"سعر البيع", "سعر الصيدلية", "سعر التوريد", "سعر المورد", "selling price", "sales price", "supply price", "pharmacy price", "wholesale price"},
		strong:  []string{"سعر البيع", "سعر الصيدلية", "سعر التوريد", "selling price", "pharmacy price", "supply price"},
		weak:    []string{"البيع", "selling", "توريد"},
		blocked: []string{"جمهور", "public", "تكلفه", "cost", "شراء", "purchase", "خصم", "discount"},
	},
	{
		field:   FieldNetPrice,
		exact:   []string{"الصافي", "صافي", "السعر الصافي", "سعر الصافي", "بعد الخصم", "السعر بعد الخصم", "net", "net price", "final price", "price after discount"},
		strong:  []string{"السعر الصافي", "السعر بعد الخصم", "net price", "price after discount", "final price"},
		weak:    []string{"صافي", "net", "بعدالخصم"},
		blocked: []string{"تكلفه", "cost", "جمهور", "public"},
	},
	{
		field: FieldCostPrice,
		exact: []string{
			"سعر التكلفة", "التكلفة", "سعر الشراء", "تكلفة الوحدة",
			"cost", "cost price", "purchase price", "buying price", "buy price",
		},
		strong:  []string{"سعر التكلفة", "سعر الشراء", "تكلفة الوحدة", "cost price", "purchase price", "buying price"},
		weak:    []string{"تكلفه", "شراء", "cost", "purchase"},
		blocked: []string{"جمهور", "public", "بيع", "selling", "نسبه", "percent", "خصم", "discount", "%"},
	},
	{
		field: FieldCostDiscountPct,
		exact: []string{
			"خصم التكلفة", "خصم تكلفة", "نسبة خصم التكلفة", "نسبه خصم التكلفه",
			"خصم الشراء", "خصم شراء", "نسبة خصم الشراء", "نسبه خصم الشراء",
			"خصم المورد", "نسبة خصم المورد",
			"cost discount", "cost discount percent", "cost discount percentage",
			"purchase discount", "cost disc", "buy discount",
		},
		strong: []string{
			"خصم التكلفة", "نسبة خصم التكلفة", "خصم الشراء", "نسبة خصم الشراء",
			"cost discount", "purchase discount",
		},
		weak:    []string{"خصم تكلفه", "خصم شراء", "cost disc"},
		blocked: []string{"بعد الخصم", "سعر", "price", "net", "صافي"},
	},
	{
		field: FieldDiscountPct,
		exact: []string{
			"الخصم", "خصم", "نسبة الخصم", "نسبه الخصم", "الخصم التجاري",
			"خصم اساسي", "خصم أساسى", "خصم خاص", "الخصم الممنوح", "نسبة",
			"discount", "disc", "discount percent", "discount percentage",
			"discount rate", "disc %", "rebate",
		},
		strong: []string{
			"نسبة الخصم", "الخصم التجاري", "خصم اساسي", "خصم خاص",
			"الخصم الممنوح", "discount percent", "discount rate", "discount %",
		},
		// The circumstantial vocabulary. Every one of these labels a discount
		// column in a real distributor file and says nothing on its own; the
		// value evidence is what promotes or drops them.
		weak: []string{
			"خصم", "discount", "disc", "نسبه", "تخفيض",
			"القائمه", "المرجح", "جمله", "ج الجمله", "مندوب", "العموله",
		},
		blocked: []string{"بعد الخصم", "قبل الخصم", "after discount", "قيمة الخصم", "مبلغ الخصم", "تكلفه", "شراء", "cost", "purchase"},
	},
	{
		field:   FieldDiscountAmt,
		exact:   []string{"قيمة الخصم", "مبلغ الخصم", "discount amount", "discount value"},
		strong:  []string{"قيمة الخصم", "مبلغ الخصم", "discount amount", "discount value"},
		weak:    []string{},
		blocked: []string{"نسبه", "percent", "rate"},
	},
	{
		field:   FieldBonus,
		exact:   []string{"البونص", "بونص", "العرض", "عرض", "الهدية", "هدية", "bonus", "offer", "free", "promo", "deal"},
		strong:  []string{"عرض الكمية", "بونص الكمية", "free goods", "bonus qty"},
		weak:    []string{"بونص", "عرض", "bonus", "offer", "هديه"},
		blocked: []string{"سعر", "price", "خصم", "discount"},
	},

	{
		field: FieldQuantity,
		exact: []string{
			"الكمية", "الكميه", "كمية", "الرصيد", "المخزون", "العدد", "المتاح",
			"الكمية المتوفرة", "الرصيد المتاح", "ارصدة", "أرصدة", "رصيد",
			"quantity", "qty", "stock", "balance", "available", "on hand", "count",
		},
		strong:  []string{"الكمية المتوفرة", "الرصيد المتاح", "كمية المخزون", "available quantity", "stock quantity", "on hand"},
		weak:    []string{"الكميه", "الرصيد", "المخزون", "quantity", "qty", "stock", "balance"},
		blocked: []string{"ادني", "اقل", "min", "حد", "طلب", "order", "moq", "اقصي", "max", "سعر", "price", "عبوه", "pack"},
	},
	{
		field:   FieldMinOrderQty,
		exact:   []string{"اقل كمية", "أقل كمية للطلب", "الحد الادني للطلب", "الحد الأدنى للطلب", "min order", "min order qty", "minimum order", "moq"},
		strong:  []string{"اقل كمية للطلب", "الحد الادني للطلب", "min order qty", "minimum order quantity"},
		weak:    []string{"moq", "min order", "اقل كميه"},
		blocked: []string{"مخزون", "stock", "reorder", "اعاده"},
	},
	{
		field:   FieldMinThreshold, // .
		exact:   []string{"حد الطلب", "حد اعادة الطلب", "الحد الادني للمخزون", "حد التنبيه", "reorder level", "reorder point", "min stock", "safety stock", "minimum stock"},
		strong:  []string{"حد اعادة الطلب", "الحد الادني للمخزون", "reorder level", "min stock level", "safety stock"},
		weak:    []string{"reorder", "حد التنبيه"},
		blocked: []string{"طلب الشراء", "purchase order"},
	},
	{
		field:   FieldBatchNumber,
		exact:   []string{"رقم التشغيلة", "التشغيلة", "تشغيلة", "رقم الباتش", "الباتش", "الطبخة", "batch", "batch no", "batch number", "lot", "lot no", "lot number"},
		strong:  []string{"رقم التشغيلة", "رقم الباتش", "batch number", "lot number"},
		weak:    []string{"تشغيله", "باتش", "batch", "lot", "الطبخه"},
		blocked: []string{"تاريخ", "date", "صلاحيه"},
	},
	{
		field:  FieldExpiryDate,
		exact:  []string{"تاريخ الصلاحية", "الصلاحية", "تاريخ الانتهاء", "انتهاء الصلاحية", "الانتهاء", "expiry", "expiry date", "exp", "exp date", "expiration", "expiration date", "best before"},
		strong: []string{"تاريخ الصلاحية", "تاريخ الانتهاء", "انتهاء الصلاحية", "expiry date", "expiration date"},
		weak:   []string{"صلاحيه", "انتهاء", "expiry", "exp"},
		// A column stating when the *record* changed is not a column stating
		// when the *medicine* expires, and binding one to the other is not a
		// cosmetic error: RejectExpired then refused 17,871 of a real file's
		// 19,996 rows and reported the import as successful. Every word an ERP
		// uses for a bookkeeping timestamp is blocked here.
		blocked: []string{
			"الانتاج", "production", "تصنيع", "manufactur",
			"updated", "update", "modified", "created", "entry", "added",
			"تحديث", "التحديث", "اضافه", "الاضافة", "انشاء", "الإنشاء", "تسجيل",
			"الطلب", "order", "invoice", "الفاتوره", "الفاتورة",
		},
	},
	{
		field:   FieldWarehouse,
		exact:   []string{"المخزن", "مخزن", "المستودع", "مستودع", "warehouse", "store", "location", "depot"},
		strong:  []string{"اسم المخزن", "كود المخزن", "warehouse name", "warehouse code"},
		weak:    []string{"مخزن", "مستودع", "warehouse", "store"},
		blocked: []string{"الرصيد", "الكميه", "quantity", "ارصده"},
	},
	{
		field:   FieldBranch,
		exact:   []string{"الفرع", "فرع", "branch", "outlet"},
		strong:  []string{"اسم الفرع", "كود الفرع", "branch name", "branch code"},
		weak:    []string{"فرع", "branch"},
		blocked: []string{},
	},
	{
		field:   FieldStatus,
		exact:   []string{"الحالة", "حالة الصنف", "الحاله", "نشط", "status", "state", "active"},
		strong:  []string{"حالة الصنف", "حالة المنتج", "product status"},
		weak:    []string{"الحاله", "status", "state"},
		blocked: []string{"حالة الطلب", "order status", "payment"},
	},
	{
		field:   FieldNegotiable,
		exact:   []string{"قابل للتفاوض", "تفاوض", "التفاوض", "negotiable", "negotiation"},
		strong:  []string{"قابل للتفاوض", "negotiable"},
		weak:    []string{"تفاوض", "negotiab"},
		blocked: []string{},
	},
	{
		field:   FieldImage,
		exact:   []string{"الصورة", "الصوره", "صورة", "رابط الصورة", "image", "img", "picture", "photo", "image url", "image link"},
		strong:  []string{"رابط الصورة", "image url", "image link", "product image"},
		weak:    []string{"صوره", "image", "img", "photo"},
		blocked: []string{},
	},
	{
		field:   FieldNotes,
		exact:   []string{"ملاحظات", "ملاحظة", "بيان", "تعليق", "notes", "note", "remarks", "comment", "comments"},
		strong:  []string{"ملاحظات", "notes", "remarks"},
		weak:    []string{"ملاحظه", "notes", "remark", "comment"},
		blocked: []string{"بيان الصنف", "item description"},
	},
	{
		field:   FieldDescription,
		exact:   []string{"الوصف", "وصف", "وصف الصنف", "وصف المنتج", "التفاصيل", "description", "desc", "details", "description ar"},
		strong:  []string{"وصف الصنف", "وصف المنتج", "الوصف بالعربية", "description"},
		weak:    []string{"وصف", "desc", "details"},
		blocked: []string{"english", "بالانجليزية", "بالإنجليزية", "en"},
	},
	{
		field:   FieldDescriptionEN,
		exact:   []string{"الوصف بالإنجليزية", "الوصف بالانجليزية", "description en", "english description", "desc en", "description_en"},
		strong:  []string{"الوصف بالإنجليزية", "english description", "description en"},
		weak:    []string{"desc en"},
		blocked: []string{},
	},
}
