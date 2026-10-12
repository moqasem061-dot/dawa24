package productmatch

import "regexp"

var comparedForms = regexp.MustCompile(`(?i)\b(?:vs\.?|versus|against)\b`)

// Explicit comparisons name alternatives; they do not identify one dosage form.
func ambiguousFormComparison(name string) bool {
	parts := comparedForms.Split(name, 2)
	if len(parts) != 2 {
		return false
	}
	left, right := formKeyOf(parts[0]), formKeyOf(parts[1])
	if left == "" || right == "" {
		return false
	}
	if left != right {
		return true
	}
	return topicalSubForm(parts[0]) != topicalSubForm(parts[1])
}

func formClass(key string) string {
	switch key {
	case "tablet", "capsule":
		return "solid_oral"
	case "topical", "wash":
		return "external"
	}
	return key
}
