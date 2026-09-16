package tools_test

import (
	"context"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/tools"
)

type spyMemoryStore struct {
	saved   []*assistant.Memory
	deleted []int64
	list    []*assistant.Memory
}

func (s *spyMemoryStore) SaveMemory(ctx context.Context, mem *assistant.Memory) error {
	mem.ID = int64(len(s.saved) + 1)
	s.saved = append(s.saved, mem)
	return nil
}

func (s *spyMemoryStore) DeleteMemory(ctx context.Context, orgID, memoryID int64) error {
	s.deleted = append(s.deleted, memoryID)
	return nil
}

func (s *spyMemoryStore) ListMemories(ctx context.Context, orgID int64, userID *int64, limit int) ([]*assistant.Memory, error) {
	return s.list, nil
}

func TestMemoryTools(t *testing.T) {
	f := newFixture(t)
	spy := &spyMemoryStore{
		list: []*assistant.Memory{
			{ID: 1, OrganizationID: 1, Content: "نفضل استلام الشحنات صباحاً", Category: assistant.MemoryCategoryLogistics},
		},
	}
	f.reg.SetMemoryStore(spy)
	ph := pharmacist(1, 10)

	// 1. memory_remember
	out := f.reg.Dispatch(context.Background(), ph, 1, call("memory_remember", `{"content":"مواعيد الفرع من 9 صباحا","category":"logistics"}`))
	if out.Decision != string(tools.DecisionAllowed) {
		t.Fatalf("expected allowed, got %v: %s", out.Decision, out.Content)
	}
	if len(spy.saved) != 1 {
		t.Fatalf("expected 1 saved memory, got %d", len(spy.saved))
	}
	if spy.saved[0].Content != "مواعيد الفرع من 9 صباحا" {
		t.Errorf("unexpected content: %s", spy.saved[0].Content)
	}

	// 2. memory_list
	outList := f.reg.Dispatch(context.Background(), ph, 1, call("memory_list", `{}`))
	if outList.Decision != string(tools.DecisionAllowed) {
		t.Fatalf("expected allowed, got %v: %s", outList.Decision, outList.Content)
	}

	// 3. memory_forget
	outDel := f.reg.Dispatch(context.Background(), ph, 1, call("memory_forget", `{"memory_id":1}`))
	if outDel.Decision != string(tools.DecisionAllowed) {
		t.Fatalf("expected allowed, got %v: %s", outDel.Decision, outDel.Content)
	}
	if len(spy.deleted) != 1 || spy.deleted[0] != 1 {
		t.Fatalf("expected deleted id 1, got %v", spy.deleted)
	}
}
