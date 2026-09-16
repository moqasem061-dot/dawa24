package identity

import "context"

// The moderator-hierarchy half of mockRepo, in its own file so
// service_test.go stays inside the 400-line limit AGENTS.md sets.

func (m *mockRepo) ListModerators(ctx context.Context) ([]*Moderator, error) {
	return nil, nil
}

func (m *mockRepo) ModeratorSubordinateIDs(ctx context.Context, parentID int64) ([]int64, error) {
	return nil, nil
}

func (m *mockRepo) ModeratorParentID(ctx context.Context, userID int64) (*int64, error) {
	if m.moderatorParents != nil {
		return m.moderatorParents[userID], nil
	}
	return nil, nil
}

func (m *mockRepo) SetModeratorParent(ctx context.Context, userID int64, parentID *int64, actorID int64) error {
	if m.moderatorParents == nil {
		m.moderatorParents = make(map[int64]*int64)
	}
	m.moderatorParents[userID] = parentID
	return nil
}
