package assistant

import (
	"fmt"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/assistant/actions"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/rbac"
)

type actionableReference Entity

func (reference actionableReference) EntityRef() Entity { return Entity(reference) }

func TestAttachmentOrderKeepsEveryProposalBeyondSearchPage(t *testing.T) {
	for _, channel := range []actions.Channel{actions.ChannelWeb, actions.ChannelTelegram} {
		t.Run(string(channel), func(t *testing.T) {
			var csv strings.Builder
			csv.WriteString("name,quantity\n")
			for row := 1; row <= 80; row++ {
				fmt.Fprintf(&csv, "Product %d with detailed concentration and packaging information repeated for this full order,2\n", row)
			}
			table, ok := ReadTabularContent([]byte(csv.String()), "order.csv", DefaultMaxTabularRows)
			if !ok || len(table) <= DigestMaxChars || !strings.Contains(tabularContext(table), "Product 80") {
				t.Fatal("spreadsheet lost rows at the summary byte limit")
			}
			budget := budgetForTurn(TurnInput{Channel: channel, Digests: []AttachmentDigest{{Filename: "order.csv", Text: tabularContext(table)}}})
			if budget.calls < 80*2 || budget.rounds <= maxToolRounds {
				t.Fatalf("spreadsheet retained the single-question budget: %+v", budget)
			}
			var entities []Entity
			for row := 1; row <= MaxEntitiesPerTurn; row++ {
				entities = MergeEntities(entities, Entity{Kind: EntityOrder, ID: int64(row)})
			}
			for row := 1; row <= 80; row++ {
				entities = MergeEntities(entities, Entity{Kind: EntityProposal, ID: int64(row), Label: fmt.Sprintf("Confirm product %d", row), Proposal: &ProposalCard{ID: fmt.Sprint(row)}})
			}
			linked := (&Service{}).linkEntities(authctx.Actor{Scope: rbac.ScopePharmacy}, "", entities)
			if len(linked) != 80 {
				t.Fatalf("%s retained %d of 80 actionable proposals", channel, len(linked))
			}
			references := make([]actionableReference, len(linked))
			for index, entity := range linked {
				references[index] = actionableReference(entity)
			}
			if got := CollectEntities(references); len(got) != 80 {
				t.Fatalf("collector kept only %d proposal cards", len(got))
			}
		})
	}
}

func TestOrdinaryQuestionRetainsBoundedBudget(t *testing.T) {
	budget := budgetForTurn(TurnInput{Text: "Show my orders"})
	if budget.calls != maxToolCalls || budget.rounds != maxToolRounds || budget.deadline != turnDeadline {
		t.Fatalf("ordinary question budget changed: %+v", budget)
	}
}
