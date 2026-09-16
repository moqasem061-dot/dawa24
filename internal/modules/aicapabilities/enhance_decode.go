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
// It is tolerant of the packaging models add — a markdown fence, a sentence
// before the JSON — and intolerant of everything else. Exported because the
// parsing contract is the part of this file most worth testing directly.
func DecodeEnhanceResponse(content string) ([]EnhanceDecision, error) {
	clean := extractJSONObject(content)
	if clean == "" {
		return nil, errors.New("aicapabilities: no JSON object in enhancement response")
	}
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
