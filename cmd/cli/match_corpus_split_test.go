package main

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matcheval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitCorpus_NoFamilyCrossesSplits(t *testing.T) {
	cases := []matcheval.Case{
		// Cluster 1 (shared TF tf-1)
		{ID: "c1", TemplateFamily: "tf-1", ExpressionFamily: "ef-1"},
		{ID: "c2", TemplateFamily: "tf-1", ExpressionFamily: "ef-2"},
		// c3 links to c2 via ef-2, so c3 belongs to Cluster 1 transitively!
		{ID: "c3", TemplateFamily: "tf-2", ExpressionFamily: "ef-2"},

		// Cluster 2 (independent)
		{ID: "c4", TemplateFamily: "tf-3", ExpressionFamily: "ef-3"},
		{ID: "c5", TemplateFamily: "tf-3", ExpressionFamily: "ef-4"},

		// Cluster 3 (independent)
		{ID: "c6", TemplateFamily: "tf-4", ExpressionFamily: "ef-5"},
		{ID: "c7", TemplateFamily: "tf-5", ExpressionFamily: "ef-6"},
	}

	tuning, heldout, err := SplitCorpus(cases, 42, 0.5)
	require.NoError(t, err)

	assert.NotEmpty(t, tuning)
	assert.NotEmpty(t, heldout)
	assert.Equal(t, len(cases), len(tuning)+len(heldout))

	// Verify no family crosses splits
	tuningTF := make(map[string]bool)
	tuningEF := make(map[string]bool)
	for _, c := range tuning {
		assert.Equal(t, "tuning", c.Split)
		tuningTF[c.TemplateFamily] = true
		tuningEF[c.ExpressionFamily] = true
	}

	for _, c := range heldout {
		assert.Equal(t, "heldout", c.Split)
		assert.False(t, tuningTF[c.TemplateFamily], "TemplateFamily %s leaked between splits", c.TemplateFamily)
		assert.False(t, tuningEF[c.ExpressionFamily], "ExpressionFamily %s leaked between splits", c.ExpressionFamily)
	}

	// Verify transitive cluster integrity (c1, c2, c3 must all be in the same split)
	inTuning := make(map[string]bool)
	for _, c := range tuning {
		inTuning[c.ID] = true
	}
	assert.Equal(t, inTuning["c1"], inTuning["c2"], "c1 and c2 must be in the same split")
	assert.Equal(t, inTuning["c2"], inTuning["c3"], "c2 and c3 must be in the same split")

	// Verify determinism: same seed produces identical split
	tuning2, heldout2, err := SplitCorpus(cases, 42, 0.5)
	require.NoError(t, err)
	require.Equal(t, len(tuning), len(tuning2))
	for i := range tuning {
		assert.Equal(t, tuning[i].ID, tuning2[i].ID)
	}
	require.Equal(t, len(heldout), len(heldout2))
	for i := range heldout {
		assert.Equal(t, heldout[i].ID, heldout2[i].ID)
	}
}
