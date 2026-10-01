package aicapabilities_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/moqasem061-dot/dawa24/internal/modules/aicapabilities"
	"github.com/moqasem061-dot/dawa24/internal/platform/gateway"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

type fakeGatewayWithoutStructuredOutput struct {
	lastRequest     gateway.Request
	responseContent string
}

func (f *fakeGatewayWithoutStructuredOutput) Capabilities(_ context.Context, _ gateway.Role) (gateway.ModelCapabilities, error) {
	return gateway.ModelCapabilities{
		StructuredOutput: false, // Model lacks native json_schema support
	}, nil
}

func (f *fakeGatewayWithoutStructuredOutput) Invoke(_ context.Context, req gateway.Request) (*gateway.Response, error) {
	f.lastRequest = req
	return &gateway.Response{
		Content: f.responseContent,
	}, nil
}

func (f *fakeGatewayWithoutStructuredOutput) Stream(context.Context, gateway.ChatRequest) (<-chan gateway.StreamEvent, error) {
	return nil, nil
}

func (f *fakeGatewayWithoutStructuredOutput) Transcribe(context.Context, gateway.TranscribeRequest) (string, error) {
	return "", nil
}

func (f *fakeGatewayWithoutStructuredOutput) Health(context.Context) error {
	return nil
}

func (f *fakeGatewayWithoutStructuredOutput) Enabled() bool {
	return true
}

func TestStructuredFallback_FencedAndProseResponses(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	fakeGW := &fakeGatewayWithoutStructuredOutput{}
	svc := aicapabilities.NewService(fakeGW, logger)

	req := matchflow.Batch{
		Route: "product.match.fallback-route",
		Catalog: []matchflow.CatalogEntry{
			{ProductID: 101, NameAR: "بنادول اكسترا", NameEN: "panadol extra"},
			{ProductID: 102, NameAR: "كونجستال", NameEN: "congestal"},
		},
		Items: []matchflow.Item{
			{Ref: 1, Text: "بانادول اكسترا 24 ق", Options: []int64{101, 102}},
			{Ref: 2, Text: "كونجستال أقراص", Options: []int64{101, 102}},
		},
	}

	// 1. Markdown-fenced JSON response validates successfully and sends plain JSON instruction (Schema is nil)
	fakeGW.responseContent = "```json\n{\"results\":[{\"ref\":1,\"product_id\":101,\"confidence\":0.95,\"reason\":\"same brand\"}]}\n```"
	decisions, err := svc.EnhanceMatches(ctx, req)
	assert.NoError(t, err)
	assert.Nil(t, fakeGW.lastRequest.Schema, "native json_schema must not be sent when gateway lacks structured output")
	assert.Equal(t, "product.match.fallback-route", fakeGW.lastRequest.Route)
	assert.Len(t, decisions, 1)
	assert.Equal(t, 1, decisions[0].Ref)
	assert.NotNil(t, decisions[0].ProductID)
	assert.Equal(t, int64(101), *decisions[0].ProductID)
	assert.Equal(t, 0.95, decisions[0].Confidence)

	// 2. Prose-wrapped JSON response validates successfully
	fakeGW.responseContent = "Here are the extracted product match decisions:\n{\"results\":[{\"ref\":2,\"product_id\":102,\"confidence\":0.92}]}\nPlease review the results above."
	decisions, err = svc.EnhanceMatches(ctx, req)
	assert.NoError(t, err)
	assert.Len(t, decisions, 1)
	assert.Equal(t, 2, decisions[0].Ref)
	assert.NotNil(t, decisions[0].ProductID)
	assert.Equal(t, int64(102), *decisions[0].ProductID)
	assert.Equal(t, 0.92, decisions[0].Confidence)

	// 3. Unknown ref (ref: 999) is rejected while valid refs (1 and 2) are accepted
	fakeGW.responseContent = "{\"results\":[{\"ref\":1,\"product_id\":101,\"confidence\":0.95},{\"ref\":999,\"product_id\":101,\"confidence\":0.88},{\"ref\":2,\"product_id\":102,\"confidence\":0.90}]}"
	decisions, err = svc.EnhanceMatches(ctx, req)
	assert.NoError(t, err)
	assert.Len(t, decisions, 2, "unknown ref 999 must be rejected and filtered out")
	assert.Equal(t, 1, decisions[0].Ref)
	assert.Equal(t, 2, decisions[1].Ref)

	// 4. v8 pm-answer-1 format with markdown fence and prose
	fakeGW.responseContent = "```json\n{\"a\":[{\"r\":1,\"s\":101,\"e\":{\"st\":\"a\"},\"c\":\"hi\"},{\"r\":2,\"s\":null,\"x\":\"ambiguous\"}]}\n```"
	decisions, err = svc.EnhanceMatches(ctx, req)
	assert.NoError(t, err)
	assert.Len(t, decisions, 2)
	assert.Equal(t, 1, decisions[0].Ref)
	assert.NotNil(t, decisions[0].ProductID)
	assert.Equal(t, int64(101), *decisions[0].ProductID)
	assert.Equal(t, 0.95, decisions[0].Confidence)
	assert.Equal(t, 2, decisions[1].Ref)
	assert.Nil(t, decisions[1].ProductID)
	assert.Equal(t, "ambiguous", decisions[1].Reason)
}
