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
	"externalProductCode": {"externalProductCode", "itemId", "itemCode", "productCode", "code"},
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

// codeOrder is the priority of the item-code spellings when one row carries
// more than one. externalProductCode always wins over an alias.
var codeOrder = []string{"externalProductCode", "itemId", "itemCode", "productCode", "code"}

// pickCode returns the first non-empty item code in codeOrder, so the result
// never depends on the order a map is iterated in.
func pickCode(codes map[string]string) string {
	for _, key := range codeOrder {
		if v := codes[key]; v != "" {
			return v
		}
	}
	return ""
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
