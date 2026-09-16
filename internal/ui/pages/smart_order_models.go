package pages

import (
	"fmt"
	"net/url"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
)

type SmartOrderResultsData struct {
	Run       *smartorder.Run
	Counts    smartorder.FilterCounts
	Blocked   smartorder.BlockedCounts
	Lines     []*smartorder.Line
	Total     int
	Page      int
	PerPage   int
	Match     string
	SortBy    string
	SortOrder string
	Search    string
}

// smartOrderResultsQuery carries the match tab, sort and search through a page change.
func smartOrderResultsQuery(data SmartOrderResultsData) url.Values {
	vals := url.Values{}
	if data.Match != "" {
		vals.Set("match", data.Match)
	}
	if data.SortBy != "" {
		vals.Set("sort", data.SortBy)
	}
	if data.SortOrder != "" {
		vals.Set("order", data.SortOrder)
	}
	if data.Search != "" {
		vals.Set("q", data.Search)
	}
	return vals
}

func buildSmartOrderResultsURL(publicID, match, sort, order string, page, limit int, search string) string {
	vals := url.Values{}
	if match != "" {
		vals.Set("match", match)
	}
	if sort != "" {
		vals.Set("sort", sort)
	}
	if order != "" {
		vals.Set("order", order)
	}
	if page > 1 {
		vals.Set("page", fmt.Sprintf("%d", page))
	}
	if limit != 25 && limit != 0 {
		vals.Set("limit", fmt.Sprintf("%d", limit))
	}
	if search != "" {
		vals.Set("q", search)
	}
	base := fmt.Sprintf("/customer/smart-order/%s/results", publicID)
	if len(vals) > 0 {
		return base + "?" + vals.Encode()
	}
	return base
}

// SmartOrderOutcomeLabel returns a human-readable outcome label in the requested language.
func SmartOrderOutcomeLabel(o smartorder.Outcome, langOpt ...string) string {
	isEn := len(langOpt) > 0 && langOpt[0] == "en"
	switch o {
	case smartorder.OutcomeOrdered:
		if isEn {
			return "Ready to Order"
		}
		return "جاهز للطلب"
	case smartorder.OutcomeNoSupplier:
		if isEn {
			return "No Supplier"
		}
		return "لا يوجد مورد"
	case smartorder.OutcomeCoverageBlocked:
		if isEn {
			return "Out of Coverage"
		}
		return "خارج التغطية"
	case smartorder.OutcomeInstitutionalBlocked:
		if isEn {
			return "Institutionally Restricted"
		}
		return "غير متاح مؤسسياً"
	case smartorder.OutcomeQuotaBlocked:
		if isEn {
			return "Quota Exceeded"
		}
		return "تجاوز الكوتة"
	case smartorder.OutcomeOutOfStock:
		if isEn {
			return "Out of Stock"
		}
		return "غير متوفر بالمخزون"
	case smartorder.OutcomeBelowMinQty:
		if isEn {
			return "Below Minimum Quantity"
		}
		return "أقل من الحد الأدنى"
	case smartorder.OutcomeUnmatched:
		if isEn {
			return "Unmatched"
		}
		return "غير مطابق"
	case smartorder.OutcomeZeroQty:
		if isEn {
			return "Zero Quantity"
		}
		return "الكمية صفر"
	case smartorder.OutcomeRemoved:
		if isEn {
			return "Removed"
		}
		return "تم حذفه"
	}
	return string(o)
}

func MatchMethodLabel(m smartorder.MatchMethod, langOpt ...string) string {
	isEn := len(langOpt) > 0 && langOpt[0] == "en"
	switch m {
	case smartorder.MethodSavingProduct:
		if isEn {
			return "Saving Products"
		}
		return "منتجات التوفير"
	case smartorder.MethodLearnedMapping:
		if isEn {
			return "Prior Decision"
		}
		return "ربط سابق"
	case smartorder.MethodBarcode:
		if isEn {
			return "Barcode"
		}
		return "باركود"
	case smartorder.MethodSKU:
		if isEn {
			return "SKU Code"
		}
		return "كود الصنف"
	case smartorder.MethodExactName:
		if isEn {
			return "Exact Name"
		}
		return "اسم مطابق"
	case smartorder.MethodIdentityKey:
		if isEn {
			return "Formula & Strength"
		}
		return "تركيب وتركيز"
	case smartorder.MethodFuzzy:
		if isEn {
			return "Fuzzy Match"
		}
		return "تشابه"
	case smartorder.MethodAlias:
		if isEn {
			return "Trade Synonym"
		}
		return "اسم بديل"
	case smartorder.MethodAI:
		if isEn {
			return "AI Match"
		}
		return "ذكاء اصطناعي"
	case smartorder.MethodManual:
		if isEn {
			return "Manual Override"
		}
		return "تصحيح يدوي"
	}
	return "—"
}

func outcomeChip(o smartorder.Outcome) string {
	switch o {
	case smartorder.OutcomeOrdered:
		return "outcome-chip outcome-chip-success"
	case smartorder.OutcomeUnmatched:
		return "outcome-chip outcome-chip-danger"
	case smartorder.OutcomeNoSupplier:
		return "outcome-chip outcome-chip-warning"
	default:
		return "outcome-chip outcome-chip-warning"
	}
}

func aiSummary(ai smartorder.AIUsage, langOpt ...string) string {
	isEn := len(langOpt) > 0 && langOpt[0] == "en"
	reviewed := ai.LinesReviewed + ai.CacheHits
	if isEn {
		if ai.LinesImproved == 0 {
			return fmt.Sprintf("Reviewed %d items unresolved by deterministic matching; AI found no confident matches.", reviewed)
		}
		out := fmt.Sprintf("Reviewed %d items unresolved by deterministic matching; improved matching for %d items.", reviewed, ai.LinesImproved)
		if ai.CacheHits > 0 {
			out += fmt.Sprintf(" (%d answered from decision memory at no cost)", ai.CacheHits)
		}
		return out
	}
	if ai.LinesImproved == 0 {
		return fmt.Sprintf("تمت مراجعة %d صنف لم تتمكن المطابقة الحتمية من حسمها، ولم يُضِف الذكاء الاصطناعي مطابقة جديدة موثوقة.", reviewed)
	}
	out := fmt.Sprintf("تمت مراجعة %d صنف لم تتمكن المطابقة الحتمية من حسمها، وتم تحسين مطابقة %d منها.", reviewed, ai.LinesImproved)
	if ai.CacheHits > 0 {
		out += fmt.Sprintf(" (%d منها أُجيبت من ذاكرة القرارات دون تكلفة إضافية)", ai.CacheHits)
	}
	return out
}