package assistant

import (
	"strings"
	"time"
)

const maxAttachmentToolCalls = maxToolCalls + 3*DefaultMaxTabularRows*MaxAttachmentsPerTurn

type turnBudget struct {
	calls    int
	rounds   int
	deadline time.Duration
}

func budgetForTurn(in TurnInput) turnBudget {
	budget := turnBudget{calls: maxToolCalls, rounds: maxToolRounds, deadline: turnDeadline}
	rows := 0
	for _, digest := range in.Digests {
		tableLines := 0
		for _, line := range strings.Split(digest.Text, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "|") {
				tableLines++
			}
		}
		rows += max(0, tableLines-2) // Header and separator are not products.
	}
	if rows > 0 {
		budget.calls = min(maxAttachmentToolCalls, maxToolCalls+3*rows)
		budget.rounds = min(60, maxToolRounds+2*((rows+9)/10))
		budget.deadline = 10 * time.Minute
	}
	return budget
}

func actionableEntity(entity Entity) bool {
	return entity.Kind == EntityProposal || entity.Kind == EntityExport
}
