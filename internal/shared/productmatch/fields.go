// Package engine resolves a supplier's spreadsheet into vendor catalogue rows.
//
// It is deterministic end to end. The same file produces the same mapping, the
// same matches and the same warnings on every run, on every machine, with no
// model in the loop — because a supplier who re-uploads a corrected file has to
// see the correction, not a different guess.
//
// The engine knows about products, prices, stock and variants. It does not know
// about HTTP, Postgres, or the catalog module's types; the ingest service maps
// its output onto those. That boundary is what makes every decision here
// testable against a fixture rather than against a database.
package productmatch

// Field is one thing a vendor's file can be saying about a product.
//
// The set is deliberately wider than catalog.product_variants: a supplier file
// carries the public price and the discount, from which the vendor's own price
// is derived, and it carries the identity attributes that decide which master
// product a row is about. Both are needed and neither is a column in the table
// being written.
type Field string

// Identity: what product this row is about. These drive matching, not writing.
const (
	FieldName          Field = "name"
	FieldNameEN        Field = "name_en"
	FieldScientific    Field = "scientific_name"
	FieldBarcode       Field = "barcode"
	FieldSKU           Field = "sku"
	FieldManufacturer  Field = "manufacturer"
	FieldDosageForm    Field = "dosage_form"
	FieldConcentration Field = "concentration"
	FieldUnit          Field = "unit"
	FieldPackSize      Field = "pack_size"
	FieldCategory      Field = "category"
	// FieldProductID is dawa24's own primary key for a catalogue product.
	//
	// It is not a property of the medicine — it is this platform's identifier,
	// and only a file previously exported from dawa24 carries it. When it is
	// present it settles the match outright, which is why it is worth
	// recognising rather than leaving to be mistaken for an item code.
	FieldProductID Field = "product_id"
	// FieldActiveIngredient is the molecule as the master catalogue records it,
	// which catalog.products keeps in its own column beside the scientific
	// name. A supplier file rarely carries both; the master catalogue does.
	FieldActiveIngredient Field = "active_ingredient"
)

// Commercial: what the vendor is charging. A file usually carries two of the
// four price fields and implies the rest.
const (
	FieldPublicPrice     Field = "public_price"
	FieldPrice           Field = "price"
	FieldCostPrice       Field = "cost_price"
	FieldCostDiscountPct Field = "cost_discount_percentage"
	FieldNetPrice        Field = "net_price"
	FieldDiscountPct     Field = "discount_percent"
	FieldDiscountAmt     Field = "discount_amount"
	FieldBonus           Field = "bonus"
)

// Stock: what the vendor is holding, and under what terms.
const (
	FieldQuantity     Field = "quantity"
	FieldMinOrderQty  Field = "min_order_qty"
	FieldMinThreshold Field = "min_threshold"
	FieldBatchNumber  Field = "batch_number"
	FieldExpiryDate   Field = "expiry_date"
	FieldBranch       Field = "branch"
	FieldWarehouse    Field = "warehouse"
	FieldNegotiable   Field = "negotiable"
	FieldStatus       Field = "status"
	FieldImage        Field = "image"
	FieldNotes        Field = "notes"
	// The two description columns belong to the master catalogue rather than to
	// any supplier file: a distributor states a price, an administrator states
	// what the product is.
	FieldDescription   Field = "description"
	FieldDescriptionEN Field = "description_en"
)

// Group buckets fields for the review screen, so a vendor confirming a mapping
// reads four short lists instead of one list of twenty-nine.
type Group string

const (
	GroupIdentity  Group = "identity"
	GroupPricing   Group = "pricing"
	GroupStock     Group = "stock"
	GroupAttribute Group = "attribute"
)

// Label renders a group in Arabic.
func (g Group) Label() string {
	switch g {
	case GroupIdentity:
		return "تعريف الصنف والمطابقة"
	case GroupPricing:
		return "الأسعار والخصومات"
	case GroupStock:
		return "المخزون والتشغيلات"
	default:
		return "خصائص إضافية"
	}
}

// Kind is the shape a field's values must take. It is what lets the resolver
// veto a binding on evidence alone: a column with no numbers in it cannot be a
// price however convincing its header.
type Kind string

const (
	KindText    Kind = "text"
	KindMoney   Kind = "money"
	KindPercent Kind = "percent"
	KindCount   Kind = "count"
	KindDate    Kind = "date"
	KindBool    Kind = "bool"
	KindCode    Kind = "code"
	KindURL     Kind = "url"
)

// Numeric reports whether the kind requires parseable numbers.
func (k Kind) Numeric() bool {
	return k == KindMoney || k == KindPercent || k == KindCount
}

// Need says how much the import depends on a field being present.
type Need int

const (
	// NeedCore fields decide what a row is. Without at least one of them the
	// row cannot be matched to anything and the file cannot be imported.
	NeedCore Need = iota
	// NeedImportant fields change what is written. Their absence is reported
	// prominently, because a price list imported without a price is a silent
	// catastrophe.
	NeedImportant
	// NeedOptional fields enrich. Their absence is noted once and forgotten.
	NeedOptional
)

// Spec describes one field: how it is shown, what shape its values take, and
// how much the import needs it.
type Spec struct {
	Field Field
	Label string
	Hint  string
	Group Group
	Kind  Kind
	Need  Need
	// Example is a value a vendor would recognise, shown beside the field in
	// the mapping screen so they can tell two similar fields apart at a glance.
	Example string
}

// Specs is the full field catalogue, in the order the mapping screen shows it.
//
// Ordering is by group and then by how often a real supplier file carries the
// field, so the four columns a typical Egyptian price list actually has —
// name, code, public price, discount — are the first four rows a vendor sees.
var Specs = []Spec{
	{FieldName, "اسم الصنف", "الاسم التجاري كما يكتبه المورد؛ أهم عمود في المطابقة", GroupIdentity, KindText, NeedCore, "بانادول إكسترا 24 قرص"},
	{FieldProductID, "معرف المنتج في دوا 24", "رقم الصنف في الكتالوج المركزي — يوجد فقط في ملف سبق تصديره من المنصة", GroupIdentity, KindCode, NeedOptional, "311352"},
	{FieldSKU, "كود الصنف لدى المورد", "الكود الداخلي للمورد — يُستخدم للتعرف على الصنف عند إعادة الرفع", GroupIdentity, KindCode, NeedImportant, "12690"},
	{FieldBarcode, "الباركود الدولي", "باركود GTIN/EAN المطبوع على العبوة", GroupIdentity, KindCode, NeedOptional, "6221234567890"},
	{FieldNameEN, "الاسم بالإنجليزية", "اسم الصنف اللاتيني إن وُجد في عمود منفصل", GroupIdentity, KindText, NeedOptional, "Panadol Extra"},
	{FieldScientific, "الاسم العلمي / المادة الفعالة", "يُستخدم لترجيح المطابقة عند تشابه الأسماء التجارية", GroupIdentity, KindText, NeedOptional, "Paracetamol"},
	{FieldManufacturer, "الشركة المصنعة", "اسم الشركة أو العلامة التجارية", GroupIdentity, KindText, NeedOptional, "جلاكسو"},
	{FieldDosageForm, "الشكل الصيدلي", "أقراص، شراب، كريم، أمبول…", GroupIdentity, KindText, NeedOptional, "أقراص"},
	{FieldConcentration, "التركيز", "قوة الجرعة كما هي مكتوبة على العبوة", GroupIdentity, KindText, NeedOptional, "500mg"},
	{FieldUnit, "الوحدة / العبوة", "وحدة البيع: علبة، شريط، زجاجة…", GroupIdentity, KindText, NeedOptional, "علبة"},
	{FieldPackSize, "عدد الوحدات بالعبوة", "كم قرصاً أو أمبولاً داخل العبوة الواحدة", GroupIdentity, KindCount, NeedOptional, "24"},
	{FieldCategory, "التصنيف", "قسم أو مجموعة الصنف في ملف المورد", GroupIdentity, KindText, NeedOptional, "مسكنات"},
	{FieldActiveIngredient, "المادة الفعالة", "المادة الدوائية الفعالة كما يسجلها الكتالوج المركزي", GroupIdentity, KindText, NeedOptional, "باراسيتامول"},

	{FieldPublicPrice, "سعر الجمهور", "السعر الرسمي المطبوع — الأساس الذي يُحسب عليه الخصم", GroupPricing, KindMoney, NeedImportant, "45.00"},
	{FieldDiscountPct, "نسبة الخصم %", "الخصم الممنوح للصيدلية كنسبة مئوية", GroupPricing, KindPercent, NeedImportant, "32"},
	{FieldPrice, "سعر البيع للصيدلية", "السعر الذي يبيع به المورد فعلياً، إن كان معطى صراحةً", GroupPricing, KindMoney, NeedOptional, "30.60"},
	{FieldNetPrice, "الصافي بعد الخصم", "السعر النهائي بعد تطبيق الخصم", GroupPricing, KindMoney, NeedOptional, "30.60"},
	{FieldCostPrice, "سعر التكلفة", "تكلفة الشراء لدى المورد — لا تظهر للصيدليات", GroupPricing, KindMoney, NeedOptional, "27.00"},
	{FieldCostDiscountPct, "نسبة خصم التكلفة %", "خصم التكلفة أو الشراء على سعر الجمهور", GroupPricing, KindPercent, NeedOptional, "25"},
	{FieldDiscountAmt, "قيمة الخصم", "الخصم كمبلغ نقدي بدلاً من نسبة", GroupPricing, KindMoney, NeedOptional, "14.40"},
	{FieldBonus, "البونص / العرض", "عروض الكمية مثل 1+1 أو 10+2", GroupPricing, KindText, NeedOptional, "1+1"},

	{FieldQuantity, "الكمية المتاحة", "الرصيد الحالي في المخزن", GroupStock, KindCount, NeedImportant, "150"},
	{FieldExpiryDate, "تاريخ الصلاحية", "تاريخ انتهاء صلاحية التشغيلة", GroupStock, KindDate, NeedOptional, "2027-11-30"},
	{FieldBatchNumber, "رقم التشغيلة", "رقم الباتش المطبوع على العبوة", GroupStock, KindCode, NeedOptional, "BN2026-A1"},
	{FieldMinOrderQty, "أقل كمية للطلب", "الحد الأدنى الذي تقبله في الطلب الواحد", GroupStock, KindCount, NeedOptional, "1"},
	{FieldMinThreshold, "حد إعادة الطلب", "الرصيد الذي يُنبَّه عنده المخزون", GroupStock, KindCount, NeedOptional, "5"},
	{FieldWarehouse, "المخزن", "اسم أو كود المخزن إن كان الملف يغطي أكثر من مخزن", GroupStock, KindText, NeedOptional, "المخزن الرئيسي"},
	{FieldBranch, "الفرع", "اسم الفرع المالك للرصيد", GroupStock, KindText, NeedOptional, "فرع المنصورة"},

	{FieldStatus, "الحالة", "مفعل / موقوف", GroupAttribute, KindBool, NeedOptional, "مفعل"},
	{FieldNegotiable, "قابل للتفاوض", "هل يقبل المورد التفاوض على سعر هذا الصنف", GroupAttribute, KindBool, NeedOptional, "نعم"},
	{FieldImage, "رابط الصورة", "رابط مباشر لصورة المنتج", GroupAttribute, KindURL, NeedOptional, "https://…/panadol.jpg"},
	{FieldNotes, "ملاحظات", "أي نص إضافي يُحفظ مع الصنف", GroupAttribute, KindText, NeedOptional, "توريد أسبوعي"},
	{FieldDescription, "الوصف بالعربية", "وصف الصنف كما يظهر للصيدليات", GroupAttribute, KindText, NeedOptional, "مسكن للألم وخافض للحرارة"},
	{FieldDescriptionEN, "الوصف بالإنجليزية", "الوصف اللاتيني إن وُجد", GroupAttribute, KindText, NeedOptional, "Pain reliever"},
}

// specIndex is the catalogue keyed by field, built once.
var specIndex = func() map[Field]Spec {
	m := make(map[Field]Spec, len(Specs))
	for _, s := range Specs {
		m[s.Field] = s
	}
	return m
}()

// SpecOf returns the description of a field, and whether it is a known one.
func SpecOf(f Field) (Spec, bool) {
	s, ok := specIndex[f]
	return s, ok
}

// Label renders a field in Arabic, falling back to its raw name so an unknown
// field never renders as an empty cell.
func (f Field) Label() string {
	if s, ok := specIndex[f]; ok {
		return s.Label
	}
	return string(f)
}

// Kind returns a field's value shape.
func (f Field) Kind() Kind {
	if s, ok := specIndex[f]; ok {
		return s.Kind
	}
	return KindText
}

// Group returns a field's review-screen bucket.
func (f Field) Group() Group {
	if s, ok := specIndex[f]; ok {
		return s.Group
	}
	return GroupAttribute
}

// Need returns how much the import depends on the field.
func (f Field) Need() Need {
	if s, ok := specIndex[f]; ok {
		return s.Need
	}
	return NeedOptional
}

// Groups is the display order of the review screen's sections.
var Groups = []Group{GroupIdentity, GroupPricing, GroupStock, GroupAttribute}
