package productmatch

// Prov indicates the provenance of an identity fact.
type Prov string

const (
	ProvOriginal  Prov = "original"
	ProvCorrected Prov = "corrected"
	ProvInferred  Prov = "inferred"
)

// Cert indicates the certainty of an extracted or provided fact.
type Cert string

const (
	CertExplicit  Cert = "explicit"
	CertParsed    Cert = "parsed"
	CertUncertain Cert = "uncertain"
)

// FormFamily categorizes pharmaceutical dosage forms into broad families.
type FormFamily string

// FormDetail specifies the exact formulation detail (e.g. odt, film-coated).
type FormDetail string

// Token represents a distinctive brand word or token with script tagging.
type Token struct {
	Text   string `json:"text"`
	Script string `json:"script,omitempty"` // ar | en | mixed
}

// Strength represents an active ingredient concentration or amount.
// Invariant: strengths are kept in written order, never sorted.
type Strength struct {
	Value     string `json:"value"` // decimal string (e.g. "500", "0.5")
	Unit      string `json:"unit"`  // mg, g, mcg, iu, %, etc.
	Basis     string `json:"basis,omitempty"` // per_unit, per_volume
	Component int    `json:"component"`       // -1 when association is unknown
	Prov      Prov   `json:"prov,omitempty"`
	Cert      Cert   `json:"cert,omitempty"`
}

// Quantity represents a physical volume or mass (e.g. 100 mL, 30 g).
type Quantity struct {
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
	Prov  Prov    `json:"prov,omitempty"`
	Cert  Cert    `json:"cert,omitempty"`
}

// Ratio represents a concentration fraction (e.g. 250 mg / 5 mL).
type Ratio struct {
	Num  Strength `json:"num"`
	Den  Quantity `json:"den"`
	Prov Prov     `json:"prov,omitempty"`
	Cert Cert     `json:"cert,omitempty"`
}

// PackFacts defines package count, subunit, and container without collapsing multi-packs.
type PackFacts struct {
	Count     int    `json:"count"`
	SubUnit   string `json:"sub_unit,omitempty"`   // strip, blister, ampoule, sachet, etc.
	Container string `json:"container,omitempty"` // box, bottle, vial, etc.
	Prov      Prov   `json:"prov,omitempty"`
	Cert      Cert   `json:"cert,omitempty"`
}

// Identifier records an explicit barcode, supplier code, or catalogue code.
type Identifier struct {
	Namespace string `json:"namespace"` // gtin, supplier_code, catalog_code
	Value     string `json:"value"`
	Valid     bool   `json:"valid"`
	Prov      Prov   `json:"prov,omitempty"`
	Cert      Cert   `json:"cert,omitempty"`
}

// Correction represents a human or curated override for a specific attribute.
type Correction struct {
	Attribute string `json:"attribute"`
	Value     string `json:"value"`
	Reason    string `json:"reason,omitempty"`
}

// IdentityFacts holds all extracted identity evidence for a row or product.
type IdentityFacts struct {
	Brand         []Token      `json:"brand,omitempty"`
	Ingredients   []string     `json:"ingredients,omitempty"`
	Strengths     []Strength   `json:"strengths,omitempty"` // ordered, never sorted
	Concentration *Ratio       `json:"concentration,omitempty"`
	FormFamily    FormFamily   `json:"form_family,omitempty"`
	FormDetail    FormDetail   `json:"form_detail,omitempty"`
	Route         string       `json:"route,omitempty"`
	Release       string       `json:"release,omitempty"`
	Modifiers     []string     `json:"modifiers,omitempty"`
	Letter        string       `json:"letter,omitempty"`
	TotalContent  *Quantity    `json:"total_content,omitempty"`
	Pack          *PackFacts   `json:"pack,omitempty"`
	DeviceSize    string       `json:"device_size,omitempty"`
	Manufacturer  string       `json:"manufacturer,omitempty"`
	Identifiers   []Identifier `json:"identifiers,omitempty"`
	MoneyEvidence []string     `json:"money_evidence,omitempty"`

	Prov Prov `json:"prov,omitempty"`
	Cert Cert `json:"cert,omitempty"`
}
