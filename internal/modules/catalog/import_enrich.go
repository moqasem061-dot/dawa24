package catalog

import (
	"context"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"strings"
)

// Filling in what the file did not say.
//
// Two kinds of gap, handled differently. A category or a pharmaceutical form
// written in the supplier's own words is a translation problem: the file's
// distinct words are folded against the catalogue's vocabulary, and only what
// exact folding cannot settle is worth asking a model about. Which column holds
// which field is a reading problem, and it is settled before parsing because
// its whole purpose is to change how the sheet is read.
//
// Distinct is what makes the first cheap. A fifty-thousand-row file has perhaps
// twenty category words in it, so this is two requests regardless of the file's
// size, and the answers are applied to every row by the ordinary importer.
//
// None of it is ever fatal. Exact folding has already matched everything that
// matches on spelling; without a model the rest are simply left for the admin
// to see as unmatched, and the import proceeds.

// resolveTaxonomies is the whole of AI's involvement after the column mapping:
// two small requests that translate the file's distinct category and
// pharmaceutical-form words onto the ones the catalogue already uses.
//
// Distinct is what makes it cheap. A fifty-thousand-row file has perhaps twenty
// category words in it, so this is two requests regardless of the file's size,
// and the answers are applied to every row by the ordinary importer.
//
// Failure is never fatal. Exact folding has already matched everything that
// matches on spelling; without a model the rest are simply left for the admin
// to see as unmatched, and the import proceeds.
func (s *Service) resolveTaxonomies(
	ctx context.Context, session *ImportSession, parsed *ParseResult, progress ProgressFunc,
) {
	progress.report(ImportPhaseMapping, 0, 0)

	vocab, err := s.imports.ImportVocabulary(ctx, session.OrganizationID)
	if err != nil {
		s.log.WarnContext(ctx, "taxonomy vocabulary unavailable", "error", err)
		return
	}

	var notes []string
	if session.Options.AssignCategory {
		// Two passes, in this order. The file's own category column is the
		// administrator's stated intent and outranks anything inferred; only
		// the products it left without a category are guessed at from the
		// molecule. See import_categorize.go.
		notes = append(notes, s.resolveCategories(ctx, session, parsed, vocab)...)
		notes = append(notes, s.inferCategories(ctx, session, parsed, vocab, progress)...)
	}
	if session.Options.AssignDosageForm {
		notes = append(notes, s.resolveDosageForms(ctx, session, parsed, vocab)...)
	}

	applyDefaultCategory(parsed.Products, session.Options)
	if len(notes) > 0 {
		session.AINote = strings.Join(notes, " ")
	}
	if progress != nil {
		progress.report(ImportPhaseMapping, 1, 1)
	}
}

// resolveCategories translates the file's category words and stamps the
// resulting ids onto every product that used them.
func (s *Service) resolveCategories(
	ctx context.Context, session *ImportSession, parsed *ParseResult, vocab EnrichVocabulary,
) []string {
	sources := DistinctValues(parsed.Products, func(p *Product) string { return p.SourceCategory })
	if len(sources) == 0 {
		return nil
	}

	targets := make([]string, 0, len(vocab.Categories))
	idByName := make(map[string]int64, len(vocab.Categories))
	for _, option := range vocab.Categories {
		targets = append(targets, option.Name)
		idByName[option.Name] = option.ID
	}

	mapping := s.mapValues(ctx, session, ValueMapCategory, sources, targets)
	for _, p := range parsed.Products {
		if p == nil || p.SourceCategory == "" {
			continue
		}
		if p.CategoryID != nil && *p.CategoryID > 0 {
			continue
		}
		if name, ok := mapping.Lookup(p.SourceCategory); ok {
			if id := idByName[name]; id > 0 {
				resolved := id
				p.CategoryID = &resolved
			}
		}
	}

	// Categories nothing existing covers. They become real rows at commit, and
	// only when the admin left auto-creation on.
	session.NewCategories = mapping.Unmatched()
	return []string{fmt.Sprintf(
		i18n.TDefault("w4_mod.d_d_s_80"),
		mapping.Matched(), len(sources), unmatchedSuffix(len(mapping.Unmatched())))}
}

// resolveDosageForms translates the file's form words in place, so the products
// carry the catalogue's own spelling rather than the supplier's.
func (s *Service) resolveDosageForms(
	ctx context.Context, session *ImportSession, parsed *ParseResult, vocab EnrichVocabulary,
) []string {
	sources := DistinctValues(parsed.Products, func(p *Product) string { return p.DosageForm })
	if len(sources) == 0 {
		return nil
	}

	mapping := s.mapValues(ctx, session, ValueMapDosageForm, sources, vocab.DosageForms)
	for _, p := range parsed.Products {
		if p == nil || p.DosageForm == "" {
			continue
		}
		if canonical, ok := mapping.Lookup(p.DosageForm); ok {
			p.DosageForm = canonical
		}
	}
	return []string{fmt.Sprintf(
		i18n.TDefault("w4_mod.d_d_s_81"),
		mapping.Matched(), len(sources), unmatchedSuffix(len(mapping.Unmatched())))}
}

// mapValues runs one value-mapping request, falling back to exact folding alone
// when the model is unavailable.
func (s *Service) mapValues(
	ctx context.Context, session *ImportSession, kind ValueMapKind, sources, targets []string,
) ValueMapping {
	if !session.Options.UseAI || s.mapper == nil || len(targets) == 0 {
		return BuildValueMapping(sources, targets, ValueMapResult{})
	}

	req := ValueMapRequest{
		Kind: kind, Sources: sources, Targets: targets,
		OrganizationID: session.OrganizationID,
	}
	if session.CreatedBy != nil {
		req.UserID = *session.CreatedBy
	}

	session.AICalls++
	result, err := s.mapper.MapValues(ctx, req)
	if err != nil {
		session.AIFallback = true
		s.log.WarnContext(ctx, "value mapping unavailable, using exact matching only",
			"session", session.PublicID, "kind", kind, "error", err)
		return BuildValueMapping(sources, targets, ValueMapResult{})
	}

	mapping := BuildValueMapping(sources, targets, result)
	session.AIApplied += mapping.Matched()
	return mapping
}

func unmatchedSuffix(unmatched int) string {
	if unmatched == 0 {
		return ""
	}
	return fmt.Sprintf(i18n.TDefault("w4_mod.d_264"), unmatched)
}

// resolveColumnMapping is the first AI request: it reads the header and a few
// sample rows and says which column is which field.
//
// It runs only where the deterministic mapper is unsure. A file whose headers
// are plain — and most are — never reaches a model at all, which is the point:
// AI is here for the badly labelled file, not the ordinary one.
func (s *Service) resolveColumnMapping(
	ctx context.Context, session *ImportSession, data *SheetData, overrides LayoutOverrides,
) LayoutOverrides {
	if !session.Options.UseAI || s.mapper == nil {
		return overrides
	}

	layout := AnalyzeLayout(data).Apply(data, overrides)
	if !needsColumnHelp(layout.Primary) {
		return overrides
	}

	req := BuildColumnMapRequest(data, layout)
	req.OrganizationID = session.OrganizationID
	if session.CreatedBy != nil {
		req.UserID = *session.CreatedBy
	}

	session.AICalls++
	result, err := s.mapper.MapColumns(ctx, req)
	if err != nil {
		session.AIFallback = true
		s.log.WarnContext(ctx, "column mapping unavailable, using header detection only",
			"session", session.PublicID, "error", err)
		return overrides
	}

	suggested := ApplyColumnMap(result, layout.Primary, data.Width)
	if len(suggested.Columns) == 0 {
		return overrides
	}

	// The admin's own corrections outrank the model's, always.
	merged := overrides
	if merged.Columns == nil {
		merged.Columns = map[string]int{}
	}
	for field, column := range suggested.Columns {
		if _, chosen := overrides.Columns[field]; !chosen {
			merged.Columns[field] = column
		}
	}
	s.log.InfoContext(ctx, "column mapping assisted by ai",
		"session", session.PublicID, "assigned", len(suggested.Columns))
	return merged
}

// needsColumnHelp reports whether the header detection left enough doubt to be
// worth a request.
//
// The test is whether the fields that decide an import — what a product is
// called, what it costs, who makes it — were found confidently. A file that
// names all of them plainly is read correctly without help.
func needsColumnHelp(plan ColumnPlan) bool {
	if plan.Positional {
		return true
	}
	for _, field := range []string{FieldNameAR, FieldPrice, FieldManufacturer} {
		column, bound := plan.Columns[field]
		if !bound {
			return true
		}
		if !boundWithCertainty(plan, field, column) {
			return true
		}
	}
	return false
}

func boundWithCertainty(plan ColumnPlan, field string, column int) bool {
	for _, binding := range plan.Bindings {
		if binding.Field == field && binding.Index == column {
			return binding.Score >= scoreCertain
		}
	}
	return false
}
