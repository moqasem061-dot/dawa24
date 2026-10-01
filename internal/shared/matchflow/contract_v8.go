package matchflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	// ErrMalformedV8Response is returned when the response cannot be parsed into a v8 JSON object.
	ErrMalformedV8Response = errors.New("contract_v8: malformed or missing JSON in response")
)

// RequestView provides item options to validate v8 answers against (contracts/ai-match-v8.md).
type RequestView struct {
	// ItemOptions maps item ref to the set of allowed product IDs for that item.
	ItemOptions map[int][]int64
}

// AnswerV8 represents one answer in the compact v8 wire format (pm-answer-1).
type AnswerV8 struct {
	Ref        int               `json:"r"`
	Selected   *int64            `json:"s"`
	Abstain    string            `json:"x,omitempty"`
	Evidence   map[string]string `json:"e,omitempty"`
	Confidence string            `json:"c,omitempty"`
}

// ValidAnswer is an answer that has passed validation steps 1-3.
type ValidAnswer struct {
	Ref        int
	ProductID  *int64
	Abstain    string
	Evidence   map[string]string
	Confidence string
}

// Rejection records an answer that failed validation and why.
type Rejection struct {
	Ref    int
	Reason string
	Raw    AnswerV8
}

type v8ResponseWrapper struct {
	Answers []AnswerV8 `json:"a"`
}

// ValidateV8 implements validation steps 1–3 of contracts/ai-match-v8.md.
func ValidateV8(raw []byte, req RequestView) ([]ValidAnswer, []Rejection, error) {
	clean := extractBalancedJSON(string(raw))
	if clean == "" {
		return nil, nil, ErrMalformedV8Response
	}

	var wrapper v8ResponseWrapper
	if err := json.Unmarshal([]byte(clean), &wrapper); err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrMalformedV8Response, err)
	}

	// 1. Group answers by ref to detect duplicates (FR-074, Step 2)
	answersByRef := make(map[int][]AnswerV8)
	var refOrder []int
	seenRef := make(map[int]bool)
	for _, ans := range wrapper.Answers {
		answersByRef[ans.Ref] = append(answersByRef[ans.Ref], ans)
		if !seenRef[ans.Ref] {
			seenRef[ans.Ref] = true
			refOrder = append(refOrder, ans.Ref)
		}
	}
	sort.Ints(refOrder)

	var valid []ValidAnswer
	var rejected []Rejection

	for _, ref := range refOrder {
		ansList := answersByRef[ref]
		// Check ref known (Step 2)
		options, isKnown := req.ItemOptions[ref]
		if !isKnown {
			for _, a := range ansList {
				rejected = append(rejected, Rejection{
					Ref:    ref,
					Reason: "ai_rejected_unknown_ref",
					Raw:    a,
				})
			}
			continue
		}

		// Check duplicate refs with differing content (Step 2 & FR-074: discard both)
		if len(ansList) > 1 {
			if hasConflictingDuplicates(ansList) {
				for _, a := range ansList {
					rejected = append(rejected, Rejection{
						Ref:    ref,
						Reason: "ai_rejected_duplicate_conflict",
						Raw:    a,
					})
				}
				continue
			}
		}

		// Take the answer
		ans := ansList[0]

		// Validate enums (Step 2)
		if !isValidEnum(ans) {
			rejected = append(rejected, Rejection{
				Ref:    ref,
				Reason: "ai_rejected_invalid_enum",
				Raw:    ans,
			})
			continue
		}

		// Check selected product id
		if ans.Selected != nil {
			targetID := *ans.Selected
			offered := false
			for _, opt := range options {
				if opt == targetID {
					offered = true
					break
				}
			}
			if !offered {
				rejected = append(rejected, Rejection{
					Ref:    ref,
					Reason: "ai_rejected_not_offered",
					Raw:    ans,
				})
				continue
			}

			// Check self-reported conflict on hard attributes (Step 3)
			if hasHardConflict(ans.Evidence) {
				rejected = append(rejected, Rejection{
					Ref:    ref,
					Reason: "ai_rejected_conflict",
					Raw:    ans,
				})
				continue
			}
		}

		valid = append(valid, ValidAnswer{
			Ref:        ans.Ref,
			ProductID:  ans.Selected,
			Abstain:    ans.Abstain,
			Evidence:   ans.Evidence,
			Confidence: ans.Confidence,
		})
	}

	return valid, rejected, nil
}

func hasConflictingDuplicates(answers []AnswerV8) bool {
	first := answers[0]
	for _, a := range answers[1:] {
		if (first.Selected == nil && a.Selected != nil) ||
			(first.Selected != nil && a.Selected == nil) ||
			(first.Selected != nil && a.Selected != nil && *first.Selected != *a.Selected) ||
			first.Abstain != a.Abstain {
			return true
		}
	}
	return false
}

func isValidEnum(ans AnswerV8) bool {
	if ans.Abstain != "" {
		switch ans.Abstain {
		case "absent", "ambiguous", "conflict", "insufficient":
		default:
			return false
		}
	}
	if ans.Confidence != "" {
		switch ans.Confidence {
		case "hi", "mid", "lo":
		default:
			return false
		}
	}
	for k, v := range ans.Evidence {
		switch k {
		case "st", "fm", "rt", "rl", "md", "pk", "br":
		default:
			return false
		}
		switch v {
		case "a", "c", "u":
		default:
			return false
		}
	}
	return true
}

func hasHardConflict(ev map[string]string) bool {
	if ev == nil {
		return false
	}
	// Hard attributes per contracts/ai-match-v8.md §Validation: strength, form, route, release, modifier, brand
	hardKeys := []string{"st", "fm", "rt", "rl", "md", "br"}
	for _, k := range hardKeys {
		if ev[k] == "c" {
			return true
		}
	}
	return false
}

func extractBalancedJSON(s string) string {
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
