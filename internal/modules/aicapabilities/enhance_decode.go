package aicapabilities

// Reading the answer.
//
// The response contract is narrow on purpose: a ref, a product id or null, a
// confidence, a short reason. Everything the pipeline does with it — the window
// check, the confidence floor, the strength re-check — depends on those four
// fields meaning exactly what they say.
//
// What this file is tolerant about is packaging, and only packaging. Models fence
// their JSON, introduce it with a sentence, or add a note after the closing
// brace; losing a batch of two hundred decisions to any of those would be absurd.
// Everything past the packaging is strict.

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// DecodeEnhanceResponse parses a response into decisions.
//
// It supports both prompt v8 format {"a": [...]} and legacy format {"results": [...]}.
// It is tolerant of packaging models add (markdown fences, sentences before/after).
func DecodeEnhanceResponse(content string) ([]EnhanceDecision, error) {
	clean := extractJSONObject(content)
	if clean == "" {
		return nil, errors.New("aicapabilities: no JSON object in enhancement response")
	}

	// Try v8 format first ({"a": [...]})
	var v8Wrapper struct {
		Answers []struct {
			Ref        int               `json:"r"`
			Selected   *int64            `json:"s"`
			Abstain    string            `json:"x"`
			Evidence   map[string]string `json:"e"`
			Confidence string            `json:"c"`
		} `json:"a"`
	}
	if err := json.Unmarshal([]byte(clean), &v8Wrapper); err == nil && len(v8Wrapper.Answers) > 0 {
		byRef := make(map[int]EnhanceDecision, len(v8Wrapper.Answers))
		contradicted := make(map[int]bool)
		for _, ans := range v8Wrapper.Answers {
			d := EnhanceDecision{
				Ref:        ans.Ref,
				ProductID:  ans.Selected,
				Confidence: v8Confidence(ans.Selected, ans.Confidence),
				Reason:     ans.Abstain,
			}
			// The model's own evidence is binding: a selection it says
			// contradicts the row on an identity attribute is not an answer,
			// however confidently it was given.
			if d.ProductID != nil {
				if attr, bad := selfContradicted(ans.Evidence); bad {
					d.Confidence = 0
					d.Reason = "ai_self_conflict:" + attr
				}
			}
			if prev, dup := byRef[ans.Ref]; dup {
				// The same ref answered twice. Identical answers are one
				// answer; different ones cancel each other (pm-answer-1).
				if !sameSelection(prev.ProductID, d.ProductID) {
					contradicted[ans.Ref] = true
				}
				continue
			}
			byRef[ans.Ref] = d
		}
		decisions := make([]EnhanceDecision, 0, len(byRef))
		for ref, d := range byRef {
			if contradicted[ref] {
				continue
			}
			decisions = append(decisions, d)
		}
		sort.SliceStable(decisions, func(i, j int) bool {
			return decisions[i].Ref < decisions[j].Ref
		})
		return decisions, nil
	}

	// Fallback to legacy v7 format ({"results": [...]})
	var wrapper struct {
		Results []EnhanceDecision `json:"results"`
	}
	if err := json.Unmarshal([]byte(clean), &wrapper); err != nil {
		return nil, fmt.Errorf("aicapabilities: decode enhancement: %w", err)
	}
	// Sorted by ref so application order is stable regardless of the order the
	// model answered in.
	sort.SliceStable(wrapper.Results, func(i, j int) bool {
		return wrapper.Results[i].Ref < wrapper.Results[j].Ref
	})
	return wrapper.Results, nil
}

// extractJSONObject returns the outermost balanced JSON object in a string.
//
// Trimming a "```json" prefix covers the common case and fails on the one that
// actually costs a batch: a model that prefixes a sentence, or appends a note
// after the closing brace. Scanning for the balanced object handles both, and
// string-awareness stops a brace inside an Arabic reason from ending it early.
func extractJSONObject(s string) string {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return ""
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

// v8Confidence maps the answer's optional confidence bucket. A selection with
// no bucket is read as mid: the bucket is not what decides acceptance — the
// identity screen and the qualification of the configuration are — and a
// model that omits an optional field has not said it is unsure.
func v8Confidence(selected *int64, bucket string) float64 {
	if selected == nil {
		return 0
	}
	switch bucket {
	case "hi":
		return 0.95
	case "lo":
		return 0.70
	default:
		return 0.85
	}
}

// identityEvidence are the attributes whose contradiction makes a different
// product.
var identityEvidence = map[string]bool{"st": true, "fm": true, "rl": true, "md": true, "br": true, "rt": true}

// selfContradicted reports an identity attribute the model itself marked as
// conflicting with the row.
func selfContradicted(evidence map[string]string) (string, bool) {
	for attr, v := range evidence {
		if identityEvidence[attr] && (v == "c" || v == "conflict") {
			return attr, true
		}
	}
	return "", false
}

func sameSelection(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
