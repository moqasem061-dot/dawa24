package catalog

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	categoryInferBatchSize    = 60
	maxCategoryInferBatches   = 3
	categoryInferBatchTimeout = 25 * time.Second
)

// Categorising the products a file gave no category word for.
//
// resolveCategories translates the file's own category column onto the
// platform's categories. That is the easy half, and it is all there used to be:
// a file with no category column produced nine thousand products with a null
// category, because there was nothing to translate.
//
// Most real administrative extracts — the drug registry, a distributor's master
// list — have no category column at all. So the harder half is here: work out
// the category from what the file DOES say about the product.
//
// The signal used is the scientific name, not the product name, and that choice
// is the whole design. A brand name says nothing generalisable — بانادول and
// أدول are the same category and share no letters — while a molecule says it
// outright, and the same molecule appears on hundreds of rows. Grouping by
// molecule turns "categorise fifty thousand products" into "categorise nine
// hundred molecules", which is both affordable and far more accurate: the model
// is asked a pharmacological question it can answer rather than a branding one
// it cannot.
//
// Rows with no scientific name fall back to the product's leading word, which
// is the brand, and the brand is at least stable across a manufacturer's line.
//
// This runs in the admin main-catalogue import and nowhere else. A vendor
// import does not write catalog.products.category_id — it does not write
// catalog.products at all — so offering it there would be a switch that does
// nothing.

// categorySignal is the text a product is categorised by, and how sure we are
// that it means anything.
func categorySignal(p *Product) (signal string, strong bool) {
	if p == nil {
		return "", false
	}
	if s := CleanCellString(p.ScientificName); s != "" {
		return s, true
	}
	if s := CleanCellString(p.Active); s != "" {
		return s, true
	}
	// The brand, taken as the first word of the name. Everything after it is
	// dose, pack size and form, which vary within one product line and would
	// scatter a single brand across dozens of one-row groups.
	name := CleanCellString(p.Name.Get("ar"))
	if name == "" {
		name = CleanCellString(p.Name.Get("en"))
	}
	if name == "" {
		return "", false
	}
	if head, _, cut := strings.Cut(name, " "); cut && len([]rune(head)) >= 3 {
		return head, false
	}
	return name, false
}

// inferCategories assigns a platform category to every product still without
// one, from the molecule or the brand.
//
// It never creates a category: session.NewCategories is deliberately left
// alone. A word the file stated and the catalogue lacks is a defensible new
// category; a molecule the model could not place is not, and inventing a
// category named "Paracetamol" is how a category tree becomes a drug index.
func (s *Service) inferCategories(
	ctx context.Context, session *ImportSession, parsed *ParseResult, vocab EnrichVocabulary, progress ProgressFunc,
) []string {
	if parsed == nil || len(vocab.Categories) == 0 {
		return nil
	}

	pending := make([]*Product, 0, len(parsed.Products))
	for _, p := range parsed.Products {
		if p == nil {
			continue
		}
		if p.CategoryID != nil && *p.CategoryID > 0 {
			continue
		}
		pending = append(pending, p)
	}
	if len(pending) == 0 {
		return nil
	}

	signals := make(map[*Product]string, len(pending))
	counts := make(map[string]int)
	origByNorm := make(map[string]string)
	for _, p := range pending {
		signal, _ := categorySignal(p)
		if signal == "" {
			continue
		}
		signals[p] = signal
		key := NormalizeKey(signal)
		if key == "" {
			continue
		}
		counts[key]++
		if _, exists := origByNorm[key]; !exists {
			origByNorm[key] = signal
		}
	}
	if len(counts) == 0 {
		return nil
	}

	targets := make([]string, 0, len(vocab.Categories))
	idByName := make(map[string]int64, len(vocab.Categories))
	targetByNorm := make(map[string]string, len(vocab.Categories))
	for _, option := range vocab.Categories {
		targets = append(targets, option.Name)
		idByName[option.Name] = option.ID
		if key := NormalizeKey(option.Name); key != "" {
			if _, exists := targetByNorm[key]; !exists {
				targetByNorm[key] = option.Name
			}
		}
	}

	// 1. Resolve exact normalized matches deterministically before calling AI.
	merged := ValueMapping{resolved: map[string]string{}}
	var unmappedKeys []string
	for key := range origByNorm {
		if exactTarget, ok := targetByNorm[key]; ok {
			merged.resolved[key] = exactTarget
		} else {
			unmappedKeys = append(unmappedKeys, key)
		}
	}

	// 2. Sort unmapped signals descending by product frequency (most common first).
	sort.Slice(unmappedKeys, func(i, j int) bool {
		return counts[unmappedKeys[i]] > counts[unmappedKeys[j]]
	})

	var sources []string
	for _, key := range unmappedKeys {
		sources = append(sources, origByNorm[key])
	}

	// 3. Batch-map top signals with AI and progress reporting.
	mapping := s.mapValuesBatched(ctx, session, ValueMapCategory, sources, targets, progress, merged)

	assigned := 0
	for _, p := range pending {
		signal, ok := signals[p]
		if !ok {
			continue
		}
		name, found := mapping.Lookup(signal)
		if !found {
			continue
		}
		id := idByName[name]
		if id <= 0 {
			continue
		}
		resolved := id
		p.CategoryID = &resolved
		assigned++
	}

	if assigned == 0 {
		return nil
	}
	return []string{fmt.Sprintf(
		"تصنيف تلقائي: %d صنف عبر %d مادة/علامة مطابقة لفئات المنصة.",
		assigned, mapping.Matched())}
}

// mapValuesBatched runs value mapping over high-frequency signals in bounded batches.
//
// Signals are ordered by frequency so the most common molecules are mapped first.
// The batch size is kept small (60) to prevent oversized LLM completions and
// timeouts, and capped at maxCategoryInferBatches so the import never stalls.
func (s *Service) mapValuesBatched(
	ctx context.Context, session *ImportSession, kind ValueMapKind, sources, targets []string,
	progress ProgressFunc, merged ValueMapping,
) ValueMapping {
	if merged.resolved == nil {
		merged.resolved = make(map[string]string)
	}
	if !session.Options.UseAI || s.mapper == nil || len(targets) == 0 || len(sources) == 0 {
		return merged
	}

	totalBatches := (len(sources) + categoryInferBatchSize - 1) / categoryInferBatchSize
	if totalBatches > maxCategoryInferBatches {
		totalBatches = maxCategoryInferBatches
	}

	for batchIdx := 0; batchIdx < totalBatches; batchIdx++ {
		if progress != nil {
			progress.report(ImportPhaseMapping, batchIdx+1, totalBatches)
		}
		start := batchIdx * categoryInferBatchSize
		end := min(start+categoryInferBatchSize, len(sources))

		batchCtx, cancel := context.WithTimeout(ctx, categoryInferBatchTimeout)
		batch := s.mapValues(batchCtx, session, kind, sources[start:end], targets)
		cancel()

		for k, v := range batch.resolved {
			if _, taken := merged.resolved[k]; !taken {
				merged.resolved[k] = v
			}
		}
	}
	return merged
}
