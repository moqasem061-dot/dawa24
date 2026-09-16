package catalog

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// DistinctValues collects the distinct non-empty values a field holds across
// the parsed products, in first-seen order.
//
// Folding is what keeps the request small: i18n.TDefault("w4_ui.s_13_13"), i18n.TDefault("w4_mod.s_206_206") and "Aqras " are
// one question, and the answer applies to every row that used any of them.
func DistinctValues(prods []*Product, read func(*Product) string) []string {
	seen := map[string]bool{}
	var out []string

	for _, p := range prods {
		if p == nil {
			continue
		}
		value := CleanCellString(read(p))
		if value == "" {
			continue
		}
		key := NormalizeKey(value)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
		if len(out) >= maxDistinctValues {
			break
		}
	}
	return out
}

// ValueMapping is a resolved translation table, keyed by folded source value so
// every spelling of a value finds it.
type ValueMapping struct {
	// resolved maps a folded source value to the exact existing value.
	resolved map[string]string
	// unmatched lists source values with no existing equivalent, in the file's
	// own spelling, for the caller to create when its toggle allows.
	unmatched []string
}

// Lookup returns the existing value a source value means.
func (m ValueMapping) Lookup(source string) (string, bool) {
	if m.resolved == nil {
		return "", false
	}
	target, ok := m.resolved[NormalizeKey(source)]
	return target, ok
}

// Unmatched lists the values nothing existing covers.
func (m ValueMapping) Unmatched() []string { return m.unmatched }

// Matched is how many distinct values were translated.
func (m ValueMapping) Matched() int { return len(m.resolved) }

// BuildValueMapping combines exact folding with the model's answer.
//
// Exact folding runs first and wins: if the file says i18n.TDefault("w4_mod.s_206_206") and the catalogue
// has i18n.TDefault("w4_ui.s_13_13"), those are the same string once folded and no model is needed or
// trusted to say so. The model only settles what folding cannot.
func BuildValueMapping(sources, targets []string, result ValueMapResult) ValueMapping {
	byFolded := make(map[string]string, len(targets))
	for _, target := range targets {
		if key := NormalizeKey(target); key != "" {
			if _, taken := byFolded[key]; !taken {
				byFolded[key] = target
			}
		}
	}

	mapping := ValueMapping{resolved: map[string]string{}}
	fromModel := map[string]ValueMatch{}
	for _, match := range result.Matches {
		if key := NormalizeKey(match.Source); key != "" {
			fromModel[key] = match
		}
	}

	for _, source := range sources {
		key := NormalizeKey(source)
		if key == "" {
			continue
		}
		if exact, ok := byFolded[key]; ok {
			mapping.resolved[key] = exact
			continue
		}

		match, asked := fromModel[key]
		target := strings.TrimSpace(match.Target)
		// The model must name an existing value exactly. Anything else — a
		// reworded label, an invented one — is discarded rather than written.
		if asked && target != "" &&
			(match.Confidence == 0 || match.Confidence >= minMapConfidence) {
			if exact, known := byFolded[NormalizeKey(target)]; known {
				mapping.resolved[key] = exact
				continue
			}
		}
		mapping.unmatched = append(mapping.unmatched, source)
	}

	sort.Strings(mapping.unmatched)
	return mapping
}

// DecodeValueMap reads the model's answer, tolerating markdown fences.
func DecodeValueMap(content string) (ValueMapResult, error) {
	var out ValueMapResult
	if err := decodeJSON(content, &out); err != nil {
		return ValueMapResult{}, fmt.Errorf("catalog: decode value map: %w", err)
	}
	return out, nil
}

// EncodeJSON renders a request as the model's user message.
func EncodeJSON(v any) (string, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("catalog: encode ai request: %w", err)
	}
	return string(body), nil
}

// decodeJSON parses a model answer, stripping the markdown fences models wrap
// JSON in often enough that tolerating them is part of parsing.
func decodeJSON(content string, into any) error {
	clean := strings.TrimSpace(content)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	return json.Unmarshal([]byte(strings.TrimSpace(clean)), into)
}

// NeedsColumnHelp reports whether header detection left enough doubt to be
// worth an AI request. Exported so the decision is testable on its own.
func NeedsColumnHelp(plan ColumnPlan) bool { return needsColumnHelp(plan) }
