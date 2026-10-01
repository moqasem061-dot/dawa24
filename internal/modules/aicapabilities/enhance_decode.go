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
		decisions := make([]EnhanceDecision, len(v8Wrapper.Answers))
		for i, ans := range v8Wrapper.Answers {
			conf := 0.85
			switch ans.Confidence {
			case "hi":
				conf = 0.95
			case "mid":
				conf = 0.85
			case "lo":
				conf = 0.70
			default:
				if ans.Selected == nil {
					conf = 0.0
				}
			}
			decisions[i] = EnhanceDecision{
				Ref:        ans.Ref,
				ProductID:  ans.Selected,
				Confidence: conf,
				Reason:     ans.Abstain,
			}
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
