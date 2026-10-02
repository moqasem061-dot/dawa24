package matchreview

import (
	"sort"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

var reasonAliases = map[string]string{
	"name_exact":          "exact_name",
	"candidate_ambiguous": "ambiguous_siblings",
	"reviewer_excluded":   "excluded_user",
	"stale_knowledge":     "knowledge_stale",
}

func reasonKey(code string) string {
	if canonical, ok := reasonAliases[code]; ok {
		code = canonical
	}
	return "match.reason." + code
}

func reasonText(lang i18n.Lang, code string) string {
	if label, ok := i18n.Lookup(lang, reasonKey(code)); ok {
		return label
	}
	return i18n.T(lang, "match.reason.unknown")
}

func reasonLabel(lang i18n.Lang, code string) string {
	if label, ok := i18n.Lookup(lang, reasonKey(code)+".short"); ok {
		return label
	}
	return reasonText(lang, code)
}

func reasonClass(code string) string {
	if strings.HasSuffix(code, "_conflict") || code == "reviewer_rejected" {
		return "match-reason-conflict"
	}
	switch code {
	case "name_exact", "exact_name", "brand_agree", "reviewer_selected", "identifier_gtin", "identifier_catalog_code", "identifier_supplier_code":
		return "match-reason-confirmed"
	case "word_mismatch", "word_unverified", "ambiguous_siblings", "candidate_ambiguous", "retrieval_incomplete", "catalog_changed":
		return "match-reason-warning"
	default:
		return "match-reason-neutral"
	}
}

func reasonParamKeys(params map[string]string) []string {
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func reasonParamLabel(lang i18n.Lang, key string) string {
	if label, ok := i18n.Lookup(lang, "match.param."+key); ok {
		return label
	}
	return i18n.T(lang, "match.param.details")
}

func reasonParamValue(lang i18n.Lang, value string) string {
	if label, ok := i18n.Lookup(lang, "match.attribute."+value); ok {
		return label
	}
	return value
}

func uniqueReasonFlags(props ReasonsProps) []string {
	seen := make(map[string]bool, len(props.Reasons)+len(props.Flags))
	for _, reason := range props.Reasons {
		seen[reasonKey(reason.Code)] = true
	}
	flags := make([]string, 0, len(props.Flags))
	for _, flag := range props.Flags {
		if !seen[reasonKey(flag)] {
			flags = append(flags, flag)
			seen[reasonKey(flag)] = true
		}
	}
	return flags
}
