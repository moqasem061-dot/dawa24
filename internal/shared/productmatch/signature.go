package productmatch

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// NormalizerVersion defines the current revision of the row normalizer and fact extractor.
const NormalizerVersion = "n1"

// QuestionSignature produces a stable SHA-256 hex digest representing a row's identity question.
// Identical rows with identical decisive facts hash to the same signature, enabling safe in-run deduplication.
func QuestionSignature(rowFacts IdentityFacts, normName string, normalizerVersion string) string {
	if normalizerVersion == "" {
		normalizerVersion = NormalizerVersion
	}

	canonical := strings.Join([]string{
		strings.TrimSpace(normName),
		canonicalFacts(rowFacts),
		normalizerVersion,
	}, "\x1f")

	h := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(h[:])
}

func canonicalFacts(facts IdentityFacts) string {
	var b strings.Builder

	// Brand tokens
	var brandTokens []string
	for _, t := range facts.Brand {
		txt := strings.ToLower(strings.TrimSpace(t.Text))
		if txt != "" {
			brandTokens = append(brandTokens, txt)
		}
	}
	sort.Strings(brandTokens)
	b.WriteString(strings.Join(brandTokens, " "))
	b.WriteByte('\x1f')

	// Form
	b.WriteString(string(facts.FormFamily))
	b.WriteByte('\x1f')

	// Strengths
	var strParts []string
	for _, s := range facts.Strengths {
		strParts = append(strParts, s.Value+":"+s.Unit)
	}
	sort.Strings(strParts)
	b.WriteString(strings.Join(strParts, ","))
	b.WriteByte('\x1f')

	// Concentration
	if facts.Concentration != nil {
		b.WriteString(fmt.Sprintf("%s:%s/%v:%s",
			facts.Concentration.Num.Value, facts.Concentration.Num.Unit,
			facts.Concentration.Den.Value, facts.Concentration.Den.Unit))
	}
	b.WriteByte('\x1f')

	// Modifiers
	mods := append([]string(nil), facts.Modifiers...)
	sort.Strings(mods)
	b.WriteString(strings.Join(mods, ","))
	b.WriteByte('\x1f')

	// Release & Letter
	b.WriteString(facts.Release)
	b.WriteByte('\x1f')
	b.WriteString(facts.Letter)
	b.WriteByte('\x1f')

	// Pack
	if facts.Pack != nil {
		b.WriteString(fmt.Sprintf("%d:%s", facts.Pack.Count, facts.Pack.SubUnit))
	}
	b.WriteByte('\x1f')

	// Identifiers
	var idParts []string
	for _, id := range facts.Identifiers {
		idParts = append(idParts, id.Namespace+":"+id.Value)
	}
	sort.Strings(idParts)
	b.WriteString(strings.Join(idParts, ","))

	return b.String()
}
