// Package ingest manages vendor catalog bulk file uploads, column detection,
// staging rows, and intelligent multi-stage product matching.
package ingest

import (
	"sort"
	"strings"
	"time"
)

// SessionStatus tracks lifecycle states of an import task.
type SessionStatus string

const (
	StatusPending    SessionStatus = "pending"
	StatusProcessing SessionStatus = "processing"
	StatusCompleted  SessionStatus = "completed"
	StatusFailed     SessionStatus = "failed"
)

// ImportMode specifies the reconciliation strategy during commit.
type ImportMode string

const (
	ModeAddNewOnly         ImportMode = "add_new_only"
	ModeUpdateExistingOnly ImportMode = "update_existing_only"
	ModeClearAndAdd        ImportMode = "clear_and_add"
	ModeUpdateAndAdd       ImportMode = "update_and_add"
)

// ConfidenceLevel classifies matching reliability.
type ConfidenceLevel string

const (
	ConfidenceHigh      ConfidenceLevel = "high"      // >= 85% or deterministic ID/Exact
	ConfidenceReview    ConfidenceLevel = "review"    // 60% - 84%
	ConfidenceLow       ConfidenceLevel = "low"       // < 60%
	ConfidenceUnmatched ConfidenceLevel = "unmatched" // No match
)

// CandidateMatch stores potential alternative master products with scores.
type CandidateMatch struct {
	ProductID      int64   `json:"product_id"`
	ProductName    string  `json:"product_name"`
	ScientificName string  `json:"scientific_name,omitempty"`
	DosageForm     string  `json:"dosage_form,omitempty"`
	Concentration  string  `json:"concentration,omitempty"`
	Manufacturer   string  `json:"manufacturer,omitempty"`
	PublicPrice    string  `json:"public_price,omitempty"`
	Similarity     float64 `json:"similarity"`
	Reason         string  `json:"reason"`
}

// FileUpload records an uploaded catalog file in S3/MinIO.
type FileUpload struct {
	ID             int64     `json:"id"`
	PublicID       string    `json:"public_id"`
	OrganizationID int64     `json:"organization_id"`
	UserID         int64     `json:"user_id"`
	Filename       string    `json:"filename"`
	StorageKey     string    `json:"storage_key"`
	FileSizeBytes  int64     `json:"file_size_bytes"`
	MimeType       string    `json:"mime_type"`
	CreatedAt      time.Time `json:"created_at"`
}

// ImportSession represents an ongoing or completed bulk file import.
type ImportSession struct {
	ID                    int64             `json:"id"`
	PublicID              string            `json:"public_id"`
	OrganizationID        int64             `json:"organization_id"`
	WarehouseID           *int64            `json:"warehouse_id,omitempty"`
	ImportMode            ImportMode        `json:"import_mode"`
	EnableAIMatching      bool              `json:"enable_ai_matching"`
	EnableSavingsMatching bool              `json:"enable_savings_matching"`
	FileUploadID          int64             `json:"file_upload_id"`
	Status                SessionStatus     `json:"status"`
	ColumnMapping         map[string]string `json:"column_mapping"`
	MinSimilarityScore    float64           `json:"min_similarity_score"`
	TotalRows             int               `json:"total_rows"`
	ProcessedRows         int               `json:"processed_rows"`
	MatchedRows           int               `json:"matched_rows"`
	ReviewRows            int               `json:"review_rows"`
	UnmatchedRows         int               `json:"unmatched_rows"`
	ErrorMessage          string            `json:"error_message,omitempty"`
	StartedAt             *time.Time        `json:"started_at,omitempty"`
	CompletedAt           *time.Time        `json:"completed_at,omitempty"`
	CreatedAt             time.Time         `json:"created_at"`
	UpdatedAt             time.Time         `json:"updated_at"`
}

// ImportRow represents a single staged row extracted from an import file.
type ImportRow struct {
	ID               int64            `json:"id"`
	SessionID        int64            `json:"session_id"`
	OrganizationID   int64            `json:"organization_id"`
	RowNumber        int              `json:"row_number"`
	RawData          map[string]any   `json:"raw_data"`
	NormalizedName   string           `json:"normalized_name,omitempty"`
	MatchedProductID *int64           `json:"matched_product_id,omitempty"`
	MatchedProdName  string           `json:"matched_product_name,omitempty"`
	MatchedProdSKU   string           `json:"matched_product_sku,omitempty"`
	SimilarityScore  *float64         `json:"similarity_score,omitempty"`
	ConfidenceLevel  ConfidenceLevel  `json:"confidence_level"`
	MatchReason      string           `json:"match_reason,omitempty"`
	CandidateMatches []CandidateMatch `json:"candidate_matches,omitempty"`
	IsApproved       bool             `json:"is_approved"`
	ImportAction     string           `json:"import_action"`
	ErrorDetails     string           `json:"error_details,omitempty"`
	Status           string           `json:"status"`
	CreatedAt        time.Time        `json:"created_at"`
}

// RowMatchUpdate encapsulates batched match update parameters for a staged row.
type RowMatchUpdate struct {
	RowID            int64
	MatchedProductID *int64
	Score            float64
	ConfidenceLevel  ConfidenceLevel
	MatchReason      string
	Candidates       []CandidateMatch
	IsApproved       bool
	Status           string
}

// RowActionUpdate records the committed execution action of a staged row.
type RowActionUpdate struct {
	RowID        int64
	ImportAction string
	ErrorDetails string
}

// Standard target fields expected by the catalog ingest pipeline.
const (
	FieldProductName   = "product_name"
	FieldPrice         = "price"
	FieldCostPrice     = "cost_price"
	FieldCostDiscount  = "cost_discount_percentage"
	FieldQuantity      = "quantity"
	FieldDiscount      = "discount"
	FieldBarcode       = "barcode"
	FieldSKU           = "sku"
	FieldBatchNumber   = "batch_number"
	FieldExpiryDate    = "expiry_date"
	FieldUnit          = "unit"
	FieldMinThreshold  = "min_threshold"
	FieldMinOrderQty   = "min_order_qty"
	FieldManufacturer  = "manufacturer"
	FieldDosageForm    = "dosage_form"
	FieldConcentration = "concentration"
)

type columnRule struct {
	field   string
	exact   []string
	strong  []string
	weak    []string
	blocked []string
}

const (
	scoreExactCol  = 100
	scoreStrongCol = 60
	scoreWeakCol   = 25
)

var columnRules = []columnRule{
	{
		field:  FieldBarcode,
		exact:  []string{"الباركود", "باركود", "كود دولي", "الرقم الدولي", "الباركود الدولي", "barcode", "gtin", "ean", "ean13", "upc"},
		strong: []string{"كود دولي", "باركود دولي", "رقم باركود", "international code"},
		weak:   []string{"باركود", "barcode"},
	},
	{
		field:   FieldSKU,
		exact:   []string{"كود الصنف", "رقم الصنف", "كود المورد", "كود الشركة", "كود المنتج", "sku", "item code", "code", "item no", "product code"},
		strong:  []string{"كود الصنف", "رقم الصنف", "كود المورد", "كود الشركة", "item code", "product code"},
		weak:    []string{"كود", "code", "sku"},
		blocked: []string{"باركود", "barcode", "دولي", "gtin", "ean"},
	},
	{
		field:   FieldCostPrice,
		exact:   []string{"سعر التكلفة", "سعر الشراء", "التكلفة", "cost price", "cost", "buy price", "purchase price"},
		strong:  []string{"سعر التكلفة", "سعر الشراء", "cost price", "purchase price"},
		weak:    []string{"تكلفة", "cost"},
		blocked: []string{"نسبة", "خصم", "discount", "%"},
	},
	{
		field:   FieldCostDiscount,
		exact:   []string{"خصم التكلفة", "نسبة خصم التكلفة", "خصم الشراء", "نسبة خصم الشراء", "خصم المورد", "cost discount", "cost discount percentage", "purchase discount", "cost disc"},
		strong:  []string{"خصم التكلفة", "خصم الشراء", "نسبة خصم الشراء", "cost discount", "purchase discount"},
		weak:    []string{"خصم تكلفة", "خصم شراء", "cost disc"},
		blocked: []string{"سعر", "price", "net", "جمهور"},
	},
	{
		field:   FieldPrice,
		exact:   []string{"سعر البيع", "سعر الصيدلية", "السعر", "سعر التوريد", "سعر الجمهور", "سعر البيع للجمهور", "سعر المستهلك", "سعر الوحدة", "price", "selling price", "pharmacy price", "net price", "public price", "retail price", "unit price"},
		strong:  []string{"سعر البيع", "سعر الصيدلية", "سعر التوريد", "سعر الجمهور", "سعر البيع للجمهور", "selling price", "pharmacy price", "public price", "retail price"},
		weak:    []string{"سعر", "price"},
		blocked: []string{"تكلفة", "شراء", "cost", "buy"},
	},
	{
		field:   FieldDiscount,
		exact:   []string{"الخصم", "نسبة الخصم", "الخصم التجاري", "خصم", "قيمة الخصم", "discount", "disc", "discount percentage", "discount rate"},
		strong:  []string{"نسبة الخصم", "الخصم التجاري", "discount percent", "discount rate"},
		weak:    []string{"خصم", "disc", "discount"},
		blocked: []string{"تكلفة", "شراء", "cost", "purchase"},
	},
	{
		field:   FieldQuantity,
		exact:   []string{"الكمية", "الرصيد", "المخزون", "العدد", "كمية الصنف", "الرصيد المتاح", "quantity", "qty", "stock", "balance", "count"},
		strong:  []string{"الرصيد المتاح", "كمية المخزون", "available qty", "stock balance"},
		weak:    []string{"كمية", "رصيد", "qty", "stock"},
		blocked: []string{"ادنى", "اقل", "min", "طلب", "order"},
	},
	{
		field:   FieldProductName,
		exact:   []string{"اسم الصنف", "اسم المنتج", "الصنف", "اسم الدواء", "المنتج", "اسم المستحضر", "product name", "item name", "description", "item description", "trade name"},
		strong:  []string{"اسم الصنف", "اسم المنتج", "اسم الدواء", "اسم المستحضر", "product name", "item name", "trade name"},
		weak:    []string{"صنف", "دواء", "product", "item"},
		blocked: []string{"كود", "code", "سعر", "price", "كمية", "qty", "علمي", "scientific"},
	},
	{
		field:  FieldBatchNumber,
		exact:  []string{"رقم التشغيلة", "التشغيلة", "رقم الباتش", "الباتش", "الطبخة", "batch", "batch no", "batch number", "lot", "lot no", "lot number"},
		strong: []string{"رقم التشغيلة", "رقم الباتش", "batch number", "lot number"},
		weak:   []string{"تشغيلة", "باتش", "batch", "lot"},
	},
	{
		field:  FieldExpiryDate,
		exact:  []string{"تاريخ الصلاحية", "الصلاحية", "تاريخ الانتهاء", "انتهاء الصلاحية", "expiry", "exp date", "expiry date", "expiration date"},
		strong: []string{"تاريخ الصلاحية", "تاريخ الانتهاء", "expiry date", "expiration date"},
		weak:   []string{"صلاحية", "انتهاء", "expiry", "exp"},
	},
	{
		field:  FieldMinOrderQty,
		exact:  []string{"الحد الأدنى للطلب", "أقل كمية للطلب", "أقل كمية", "الحد الأدنى", "min order qty", "min order", "moq", "minimum order"},
		strong: []string{"الحد الأدنى للطلب", "أقل كمية للطلب", "min order qty", "minimum order"},
		weak:   []string{"moq", "min order"},
	},
	{
		field:  FieldUnit,
		exact:  []string{"الوحدة", "وحدة القياس", "العبوة", "نوع العبوة", "unit", "pack", "packaging", "uom"},
		strong: []string{"وحدة القياس", "نوع العبوة", "unit of measure"},
		weak:   []string{"وحدة", "عبوة", "unit", "pack"},
	},
	{
		field:   FieldManufacturer,
		exact:   []string{"الشركة المصنعة", "المصنع", "الشركة", "البراند", "المورد", "manufacturer", "brand", "company", "producer"},
		strong:  []string{"الشركة المصنعة", "اسم المصنع", "manufacturer name"},
		weak:    []string{"شركة", "مصنع", "brand", "company"},
		blocked: []string{"كود", "code"},
	},
	{
		field:  FieldDosageForm,
		exact:  []string{"الشكل الصيدلي", "الشكل الدوائي", "شكل الدواء", "النوع", "dosage form", "form"},
		strong: []string{"الشكل الصيدلي", "الشكل الدوائي", "dosage form"},
		weak:   []string{"شكل", "form"},
	},
	{
		field:  FieldConcentration,
		exact:  []string{"التركيز", "القوة", "عيار", "تركيز الدواء", "concentration", "strength", "dose"},
		strong: []string{"تركيز الدواء", "قوة الدواء"},
		weak:   []string{"تركيز", "concentration", "strength"},
	},
}

// DetectColumns uses bipartite weighted global scoring to map standard target fields to raw spreadsheet headers.
// Each column is assigned at most once to the highest-scoring target field, eliminating order-dependent collisions.
func DetectColumns(headers []string) map[string]string {
	type candidatePair struct {
		field  string
		header string
		score  int
	}

	var pairs []candidatePair
	for _, h := range headers {
		clean := strings.ToLower(strings.TrimSpace(h))
		clean = strings.ReplaceAll(clean, "_", " ")
		clean = strings.ReplaceAll(clean, "-", " ")
		clean = strings.ReplaceAll(clean, ".", " ")

		for _, rule := range columnRules {
			// Check blocked words first
			blocked := false
			for _, b := range rule.blocked {
				if strings.Contains(clean, strings.ToLower(b)) {
					blocked = true
					break
				}
			}
			if blocked {
				continue
			}

			// 1. Exact matches
			matched := false
			for _, ex := range rule.exact {
				exClean := strings.ToLower(strings.TrimSpace(ex))
				if clean == exClean {
					pairs = append(pairs, candidatePair{field: rule.field, header: h, score: scoreExactCol})
					matched = true
					break
				}
			}
			if matched {
				continue
			}

			// 2. Strong substring matches
			for _, st := range rule.strong {
				stClean := strings.ToLower(strings.TrimSpace(st))
				if strings.Contains(clean, stClean) {
					pairs = append(pairs, candidatePair{field: rule.field, header: h, score: scoreStrongCol})
					matched = true
					break
				}
			}
			if matched {
				continue
			}

			// 3. Weak matches
			for _, wk := range rule.weak {
				wkClean := strings.ToLower(strings.TrimSpace(wk))
				if strings.Contains(clean, wkClean) {
					pairs = append(pairs, candidatePair{field: rule.field, header: h, score: scoreWeakCol})
					break
				}
			}
		}
	}

	// Sort highest score first
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].score > pairs[j].score
	})

	mapping := make(map[string]string)
	usedFields := make(map[string]bool)
	usedHeaders := make(map[string]bool)

	for _, p := range pairs {
		if usedFields[p.field] || usedHeaders[p.header] {
			continue
		}
		mapping[p.field] = p.header
		usedFields[p.field] = true
		usedHeaders[p.header] = true
	}

	return mapping
}
