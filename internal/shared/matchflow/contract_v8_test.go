package matchflow_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

func TestValidateV8_Cases(t *testing.T) {
	req := matchflow.RequestView{
		ItemOptions: map[int][]int64{
			1: {101, 102},
			2: {201, 202},
			3: {301, 302},
			4: {401, 402},
		},
	}

	// 1. Markdown fence around JSON
	fencedJSON := []byte("```json\n{\"a\":[{\"r\":1,\"s\":101,\"e\":{\"st\":\"a\",\"fm\":\"a\"},\"c\":\"hi\"}]}\n```")
	valid, rejected, err := matchflow.ValidateV8(fencedJSON, req)
	assert.NoError(t, err)
	assert.Empty(t, rejected)
	assert.Len(t, valid, 1)
	assert.Equal(t, 1, valid[0].Ref)
	assert.Equal(t, int64(101), *valid[0].ProductID)
	assert.Equal(t, "hi", valid[0].Confidence)

	// 2. Prose around JSON
	proseJSON := []byte("The model answers below:\n{\"a\":[{\"r\":2,\"s\":null,\"x\":\"ambiguous\",\"e\":{\"st\":\"u\"}}]}\nEnd of response.")
	valid, rejected, err = matchflow.ValidateV8(proseJSON, req)
	assert.NoError(t, err)
	assert.Empty(t, rejected)
	assert.Len(t, valid, 1)
	assert.Equal(t, 2, valid[0].Ref)
	assert.Nil(t, valid[0].ProductID)
	assert.Equal(t, "ambiguous", valid[0].Abstain)

	// 3. Duplicate refs with different content (FR-074: discard both)
	dupDiffJSON := []byte("{\"a\":[{\"r\":1,\"s\":101},{\"r\":1,\"s\":102}]}")
	valid, rejected, err = matchflow.ValidateV8(dupDiffJSON, req)
	assert.NoError(t, err)
	assert.Empty(t, valid, "conflicting duplicates must be discarded")
	assert.Len(t, rejected, 2)
	assert.Equal(t, "ai_rejected_duplicate_conflict", rejected[0].Reason)
	assert.Equal(t, "ai_rejected_duplicate_conflict", rejected[1].Reason)

	// Duplicate refs with identical content accepted once
	dupSameJSON := []byte("{\"a\":[{\"r\":1,\"s\":101},{\"r\":1,\"s\":101}]}")
	valid, rejected, err = matchflow.ValidateV8(dupSameJSON, req)
	assert.NoError(t, err)
	assert.Empty(t, rejected)
	assert.Len(t, valid, 1)

	// 4. Not offered product ID
	notOfferedJSON := []byte("{\"a\":[{\"r\":1,\"s\":999}]}")
	valid, rejected, err = matchflow.ValidateV8(notOfferedJSON, req)
	assert.NoError(t, err)
	assert.Empty(t, valid)
	assert.Len(t, rejected, 1)
	assert.Equal(t, "ai_rejected_not_offered", rejected[0].Reason)

	// 5. Self-reported conflict on hard attribute (Step 3: ai_rejected_conflict)
	conflictJSON := []byte("{\"a\":[{\"r\":1,\"s\":101,\"e\":{\"st\":\"c\"}}]}")
	valid, rejected, err = matchflow.ValidateV8(conflictJSON, req)
	assert.NoError(t, err)
	assert.Empty(t, valid)
	assert.Len(t, rejected, 1)
	assert.Equal(t, "ai_rejected_conflict", rejected[0].Reason)

	// 6. Unknown ref (ref: 999 not in request)
	unknownRefJSON := []byte("{\"a\":[{\"r\":999,\"s\":101}]}")
	valid, rejected, err = matchflow.ValidateV8(unknownRefJSON, req)
	assert.NoError(t, err)
	assert.Empty(t, valid)
	assert.Len(t, rejected, 1)
	assert.Equal(t, "ai_rejected_unknown_ref", rejected[0].Reason)
}
