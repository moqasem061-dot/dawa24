package productmatch

// ReasonCode identifies why a candidate was accepted, flagged, or rejected.
type ReasonCode string

const (
	ReasonIdentifierGTIN          ReasonCode = "identifier_gtin"
	ReasonIdentifierCatalogCode   ReasonCode = "identifier_catalog_code"
	ReasonIdentifierSupplierCode  ReasonCode = "identifier_supplier_code"
	ReasonIdentifierInvalid       ReasonCode = "identifier_invalid"
	ReasonIdentifierDuplicate     ReasonCode = "identifier_duplicate"
	ReasonIdentifierVsDescription ReasonCode = "identifier_vs_description"

	ReasonExactName             ReasonCode = "exact_name"
	ReasonSimilarity            ReasonCode = "similarity"
	ReasonSimilarityLettersOnly ReasonCode = "similarity_letters_only"

	ReasonKnowledgeHuman    ReasonCode = "knowledge_human"
	ReasonKnowledgeCurated  ReasonCode = "knowledge_curated"
	ReasonKnowledgePlatform ReasonCode = "knowledge_platform"
	ReasonKnowledgeStale    ReasonCode = "knowledge_stale"
	ReasonKnowledgeRejected ReasonCode = "knowledge_rejected"

	ReasonAISelected            ReasonCode = "ai_selected"
	ReasonAIAbstained           ReasonCode = "ai_abstained"
	ReasonAIRejectedNotOffered  ReasonCode = "ai_rejected_not_offered"
	ReasonAIRejectedConflict    ReasonCode = "ai_rejected_conflict"
	ReasonAIRejectedUnqualified ReasonCode = "ai_rejected_unqualified"
	ReasonAIDisagrees           ReasonCode = "ai_disagrees"

	ReasonStrengthConflict      ReasonCode = "strength_conflict"
	ReasonComponentConflict     ReasonCode = "component_conflict"
	ReasonCombinationConflict   ReasonCode = "combination_conflict"
	ReasonModifierConflict      ReasonCode = "modifier_conflict"
	ReasonFormConflict          ReasonCode = "form_conflict"
	ReasonRouteConflict         ReasonCode = "route_conflict"
	ReasonReleaseConflict       ReasonCode = "release_conflict"
	ReasonLetterConflict        ReasonCode = "letter_conflict"
	ReasonConcentrationConflict ReasonCode = "concentration_conflict"
	ReasonDimensionConflict     ReasonCode = "dimension_conflict"
	ReasonBrandConflict         ReasonCode = "brand_conflict"

	ReasonAttributeUnstated      ReasonCode = "attribute_unstated"
	ReasonPackUnresolved         ReasonCode = "pack_unresolved"
	ReasonPackConflict           ReasonCode = "pack_conflict"
	ReasonPackNeedsRelationship  ReasonCode = "pack_needs_relationship"
	ReasonAmbiguousSiblings      ReasonCode = "ambiguous_siblings"
	ReasonNoCandidate            ReasonCode = "no_candidate"
	ReasonRetrievalIncomplete    ReasonCode = "retrieval_incomplete"
	ReasonCatalogUnavailable     ReasonCode = "catalog_unavailable"
	ReasonCatalogChanged         ReasonCode = "catalog_changed"
	ReasonInvalidRow             ReasonCode = "invalid_row"
	ReasonExcludedHeader         ReasonCode = "excluded_header"
	ReasonExcludedTotal          ReasonCode = "excluded_total"
	ReasonExcludedBlank          ReasonCode = "excluded_blank"
	ReasonExcludedUser           ReasonCode = "excluded_user"
	ReasonNumericUninterpretable ReasonCode = "numeric_uninterpretable"
)

// Reason represents a structured decision explanation with optional parameters.
type Reason struct {
	Code   ReasonCode        `json:"code"`
	Params map[string]string `json:"params,omitempty"`
}

// AllReasonCodes returns every declared ReasonCode.
func AllReasonCodes() []ReasonCode {
	return []ReasonCode{
		ReasonIdentifierGTIN,
		ReasonIdentifierCatalogCode,
		ReasonIdentifierSupplierCode,
		ReasonIdentifierInvalid,
		ReasonIdentifierDuplicate,
		ReasonIdentifierVsDescription,
		ReasonExactName,
		ReasonSimilarity,
		ReasonSimilarityLettersOnly,
		ReasonKnowledgeHuman,
		ReasonKnowledgeCurated,
		ReasonKnowledgePlatform,
		ReasonKnowledgeStale,
		ReasonKnowledgeRejected,
		ReasonAISelected,
		ReasonAIAbstained,
		ReasonAIRejectedNotOffered,
		ReasonAIRejectedConflict,
		ReasonAIRejectedUnqualified,
		ReasonAIDisagrees,
		ReasonStrengthConflict,
		ReasonComponentConflict,
		ReasonCombinationConflict,
		ReasonModifierConflict,
		ReasonFormConflict,
		ReasonRouteConflict,
		ReasonReleaseConflict,
		ReasonLetterConflict,
		ReasonConcentrationConflict,
		ReasonDimensionConflict,
		ReasonBrandConflict,
		ReasonAttributeUnstated,
		ReasonPackUnresolved,
		ReasonPackConflict,
		ReasonPackNeedsRelationship,
		ReasonAmbiguousSiblings,
		ReasonNoCandidate,
		ReasonRetrievalIncomplete,
		ReasonCatalogUnavailable,
		ReasonCatalogChanged,
		ReasonInvalidRow,
		ReasonExcludedHeader,
		ReasonExcludedTotal,
		ReasonExcludedBlank,
		ReasonExcludedUser,
		ReasonNumericUninterpretable,
	}
}

// I18nKey returns the localization key path for this reason code.
func (rc ReasonCode) I18nKey() string {
	return "match.reason." + string(rc)
}
