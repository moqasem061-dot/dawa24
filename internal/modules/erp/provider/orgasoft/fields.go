package orgasoft

// productAliases maps the canonical wire name to every accepted spelling. The
// first letter's case is folded by foldKey, so Orgasoft's camelCase sample and
// the PascalCase sample both resolve without extra entries; a spelling that
// differs beyond the first letter would go here. This file is the only place a
// wire spelling is added (trap 28).
//
// `unit`, `manufacturer` and `batchNumber` are optional fields that appear in
// no Orgasoft sample; they are unverified with the owner and ignored when
// absent (verify-notes "V5 unverified").
var productAliases = map[string][]string{
	"externalProductCode": {"externalProductCode"},
	"arName":              {"arName"},
	"publicPrice":         {"publicPrice"},
	"stock":               {"stock"},
	"discount":            {"discount"},
	"tax":                 {"tax"},
	"isQuota":             {"isQuota"},
	"quotaQuantity":       {"quotaQuantity"},
	"barcode":             {"barcode"},
	"expiryDate":          {"expiryDate"},
	"unit":                {"unit"},
	"manufacturer":        {"manufacturer"},
	"batchNumber":         {"batchNumber"},
}

// foldKey lower-cases the first letter, so Orgasoft's PascalCase-dated sample
// ("ExternalProductCode", "PublicPrice", …) folds onto the camelCase wire
// names above and one alias entry covers both spellings.
func foldKey(k string) string {
	if k == "" {
		return k
	}
	if k[0] >= 'A' && k[0] <= 'Z' {
		return string(k[0]+('a'-'A')) + k[1:]
	}
	return k
}
