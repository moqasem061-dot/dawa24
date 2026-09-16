package evals_test

import (
	"context"
	"log/slog"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/evals"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// The online half of the eval: given a real question and the real tool
// schemas, does the model actually ASK for data?
//
// This is deliberately narrower than "is the answer correct". It measures tool
// SELECTION and nothing else — the model is offered the tools a granted user of
// that dashboard would see, and the run records which tool it called, if any.
// No tool is dispatched, so this needs the Gateway and no database, no tenant
// and no live rows.
//
// That narrowness is the point. The work order's first diagnostic question is
// "is a tool call even being emitted?", and it cites a measured case where the
// previous default model spent its whole output budget and returned no tool
// call at all. A rate measured here answers that question directly, separates a
// model problem from a data problem, and costs a few hundred tokens per case
// instead of a full turn.
//
// It is opt-in because it spends money:
//
//	GATEWAY_ENABLED=true GATEWAY_VIRTUAL_KEY=… ASSISTANT_EVAL=1 \
//	  go test ./internal/modules/assistant/evals/ -run TestLiveToolSelection -v
//
// ASSISTANT_EVAL_LIMIT caps how many cases per dashboard are sent (default 10),
// so a routine check is cheap and a full baseline is one variable away.

func TestLiveToolSelection(t *testing.T) {
	if os.Getenv("ASSISTANT_EVAL") == "" {
		t.Skip("set ASSISTANT_EVAL=1 (with a configured Gateway) to run the live eval")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Skipf("no usable configuration: %v", err)
	}
	if !cfg.Gateway.Enabled || cfg.Gateway.VirtualKey == "" {
		t.Skip("Gateway is not enabled; the assistant degrades to unavailable and there is nothing to measure")
	}

	client := gateway.New(cfg.Gateway, slog.Default())
	if !client.Enabled() {
		t.Skip("Gateway client reports itself disabled")
	}

	cases, err := evals.Load()
	if err != nil {
		t.Fatal(err)
	}
	reg := newRegistry()
	perScope := limitPerScope()

	type tally struct {
		asked   int
		called  int // emitted any tool call
		matched int // emitted the tool the corpus expected
		empty   int // no tool call and no text: the failure the work order names
		refused int
	}
	totals := map[rbac.Scope]*tally{}

	for _, scope := range []rbac.Scope{rbac.ScopePharmacy, rbac.ScopeVendor, rbac.ScopeAdmin} {
		actor := grantedActor(t, scope, reg)
		agent, ok := assistant.AgentFor(actor)
		if !ok {
			t.Fatalf("%s: no agent", scope)
		}
		schemas := reg.Schemas(actor)
		tl := &tally{}
		totals[scope] = tl

		sent := 0
		for _, c := range cases {
			if c.DashboardScope() != scope || sent >= perScope {
				continue
			}
			sent++
			tl.asked++

			name, text := askOnce(t, client, agent, schemas, c)
			switch {
			case name != "":
				tl.called++
				if name == c.Tool {
					tl.matched++
				}
				if c.MustRefuse() {
					// A refusal case that produced a tool call is the finding
					// this corpus exists to catch.
					t.Errorf("%s: %s expects a refusal, model called %q", scope, c.ID, name)
				}
			case c.MustRefuse():
				tl.refused++
			case text == "":
				tl.empty++
			}
		}
	}

	var asked, called, matched, empty int
	for _, scope := range []rbac.Scope{rbac.ScopePharmacy, rbac.ScopeVendor, rbac.ScopeAdmin} {
		tl := totals[scope]
		if tl == nil || tl.asked == 0 {
			continue
		}
		asked += tl.asked
		called += tl.called
		matched += tl.matched
		empty += tl.empty
		t.Logf("%-9s asked %2d — emitted a tool call %2d (%3.0f%%), expected tool %2d (%3.0f%%), empty %d, correctly refused %d",
			scope, tl.asked, tl.called, pct(tl.called, tl.asked),
			tl.matched, pct(tl.matched, tl.asked), tl.empty, tl.refused)
	}
	t.Logf("TOTAL asked %d — tool call %d (%.0f%%), expected tool %d (%.0f%%), empty answers %d",
		asked, called, pct(called, asked), matched, pct(matched, asked), empty)

	if asked > 0 && called == 0 {
		t.Errorf("the model emitted no tool call on any of %d questions — "+
			"check the model behind RolePrimary and the token budget", asked)
	}
}

// askOnce sends one question and reports the first tool the model asked for,
// plus any prose it produced. It never dispatches the call.
func askOnce(
	t *testing.T,
	client gateway.Client,
	agent assistant.AgentConfig,
	schemas []gateway.ToolSpec,
	c evals.Case,
) (toolName, text string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	events, err := client.Stream(ctx, gateway.ChatRequest{
		Role: gateway.RolePrimary,
		Messages: []gateway.ChatMessage{
			{Role: "system", Text: agent.SystemPrompt},
			{Role: "user", Text: c.Question},
		},
		Tools:       schemas,
		MaxTokens:   4000,
		Temperature: 0.3,
	})
	if err != nil {
		t.Logf("%s: stream failed: %v", c.ID, err)
		return "", ""
	}

	var body string
	for ev := range events {
		if ev.Err != nil {
			t.Logf("%s: stream error: %v", c.ID, ev.Err)
			return "", body
		}
		body += ev.Delta
		if ev.Done && len(ev.ToolCalls) > 0 {
			names := make([]string, 0, len(ev.ToolCalls))
			for _, call := range ev.ToolCalls {
				names = append(names, call.Name)
			}
			sort.Strings(names)
			return ev.ToolCalls[0].Name, body
		}
	}
	return "", body
}

// limitPerScope caps the spend of one run.
func limitPerScope() int {
	const fallback = 10
	raw := os.Getenv("ASSISTANT_EVAL_LIMIT")
	if raw == "" {
		return fallback
	}
	n := 0
	for _, r := range raw {
		if r < '0' || r > '9' {
			return fallback
		}
		n = n*10 + int(r-'0')
	}
	if n <= 0 {
		return fallback
	}
	return n
}
