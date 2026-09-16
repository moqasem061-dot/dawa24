package compare_test

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

type mockCompareRepo struct {
	plans         map[int64]*compare.Plan
	plansBySlug   map[string]*compare.Plan
	features      map[int64][]*compare.PlanFeature
	requests      map[int64]*compare.PlanRequest
	subscriptions map[int64]*compare.Subscription
	subUsers      map[int64][]*compare.SubscriptionUser
	sessions      map[int64][]*compare.UserSession
	files         map[int64]*compare.CompareFile
	fileRows      map[int64][]*compare.CompareFileRow
	savedMappings map[string]int64
	nextID        int64
}

func newMockCompareRepo() *mockCompareRepo {
	r := &mockCompareRepo{
		plans:         make(map[int64]*compare.Plan),
		plansBySlug:   make(map[string]*compare.Plan),
		features:      make(map[int64][]*compare.PlanFeature),
		requests:      make(map[int64]*compare.PlanRequest),
		subscriptions: make(map[int64]*compare.Subscription),
		subUsers:      make(map[int64][]*compare.SubscriptionUser),
		sessions:      make(map[int64][]*compare.UserSession),
		files:         make(map[int64]*compare.CompareFile),
		fileRows:      make(map[int64][]*compare.CompareFileRow),
		savedMappings: make(map[string]int64),
		nextID:        1,
	}

	// Seed basic plans
	basicPlan := &compare.Plan{
		ID:        1,
		Slug:      "compare-customer-basic",
		Name:      i18n.Text{"ar": "باقة الصيدليات", "en": "Pharmacy Basic"},
		IsActive:  true,
		IsPublic:  true,
		TrialDays: 7,
	}
	r.plans[1] = basicPlan
	r.plansBySlug[basicPlan.Slug] = basicPlan
	r.features[1] = []*compare.PlanFeature{
		{PlanID: 1, Key: "max_active_files", Value: "8", ValueType: "integer", IsActive: true},
		{PlanID: 1, Key: "max_concurrent_sessions", Value: "1", ValueType: "integer", IsActive: true},
		{PlanID: 1, Key: "ai_matching_enabled", Value: "true", ValueType: "boolean", IsActive: true},
	}
	basicPlan.Features = r.features[1]

	proPlan := &compare.Plan{
		ID:        2,
		Slug:      "compare-vendor-pro",
		Name:      i18n.Text{"ar": "باقة الموردين", "en": "Vendor Pro"},
		IsActive:  true,
		IsPublic:  true,
		TrialDays: 14,
	}
	r.plans[2] = proPlan
	r.plansBySlug[proPlan.Slug] = proPlan
	r.features[2] = []*compare.PlanFeature{
		{PlanID: 2, Key: "max_active_files", Value: "22", ValueType: "integer", IsActive: true},
		{PlanID: 2, Key: "max_concurrent_sessions", Value: "5", ValueType: "integer", IsActive: true},
		{PlanID: 2, Key: "ai_matching_enabled", Value: "true", ValueType: "boolean", IsActive: true},
	}
	proPlan.Features = r.features[2]

	r.nextID = 10
	return r
}

func (m *mockCompareRepo) ListPlans(ctx context.Context, onlyPublic bool) ([]*compare.Plan, error) {
	var list []*compare.Plan
	for _, p := range m.plans {
		if !onlyPublic || (p.IsActive && p.IsPublic) {
			list = append(list, p)
		}
	}
	return list, nil
}

func (m *mockCompareRepo) GetPlanByID(ctx context.Context, id int64) (*compare.Plan, error) {
	if p, ok := m.plans[id]; ok {
		return p, nil
	}
	return nil, apperr.NotFound("plan")
}

func (m *mockCompareRepo) GetPlanBySlug(ctx context.Context, slug string) (*compare.Plan, error) {
	if p, ok := m.plansBySlug[slug]; ok {
		return p, nil
	}
	return nil, apperr.NotFound("plan")
}

func (m *mockCompareRepo) CreatePlan(ctx context.Context, plan *compare.Plan) error {
	m.nextID++
	plan.ID = m.nextID
	m.plans[plan.ID] = plan
	m.plansBySlug[plan.Slug] = plan
	return nil
}

func (m *mockCompareRepo) UpdatePlan(ctx context.Context, plan *compare.Plan) error {
	m.plans[plan.ID] = plan
	m.plansBySlug[plan.Slug] = plan
	return nil
}

func (m *mockCompareRepo) DeletePlan(ctx context.Context, id int64) error {
	delete(m.plans, id)
	return nil
}

func (m *mockCompareRepo) ListPlanFeatures(ctx context.Context, planID int64) ([]*compare.PlanFeature, error) {
	return m.features[planID], nil
}

func (m *mockCompareRepo) SetPlanFeature(ctx context.Context, feature *compare.PlanFeature) error {
	m.features[feature.PlanID] = append(m.features[feature.PlanID], feature)
	return nil
}

func (m *mockCompareRepo) DeletePlanFeature(ctx context.Context, id int64) error {
	return nil
}

func (m *mockCompareRepo) CreatePlanRequest(ctx context.Context, req *compare.PlanRequest) error {
	m.nextID++
	req.ID = m.nextID
	m.requests[req.ID] = req
	return nil
}

func (m *mockCompareRepo) GetPlanRequestByID(ctx context.Context, id int64) (*compare.PlanRequest, error) {
	if r, ok := m.requests[id]; ok {
		return r, nil
	}
	return nil, apperr.NotFound("plan request")
}

func (m *mockCompareRepo) ListPlanRequestsByOrg(ctx context.Context, orgID int64) ([]*compare.PlanRequest, error) {
	var list []*compare.PlanRequest
	for _, r := range m.requests {
		if r.OrganizationID == orgID {
			list = append(list, r)
		}
	}
	return list, nil
}

func (m *mockCompareRepo) ListPendingPlanRequests(ctx context.Context) ([]*compare.PlanRequest, error) {
	var list []*compare.PlanRequest
	for _, r := range m.requests {
		if r.Status == compare.RequestPending {
			list = append(list, r)
		}
	}
	return list, nil
}

func (m *mockCompareRepo) ReviewPlanRequest(ctx context.Context, id int64, status compare.PlanRequestStatus, reviewerID int64, reason string) error {
	if r, ok := m.requests[id]; ok {
		r.Status = status
		r.ReviewedBy = &reviewerID
		now := time.Now().UTC()
		r.ReviewedAt = &now
		r.RejectionReason = reason
		return nil
	}
	return apperr.NotFound("plan request")
}

func (m *mockCompareRepo) CreateSubscription(ctx context.Context, sub *compare.Subscription) error {
	m.nextID++
	sub.ID = m.nextID
	m.subscriptions[sub.ID] = sub
	m.subUsers[sub.ID] = append(m.subUsers[sub.ID], &compare.SubscriptionUser{
		ID:             m.nextID,
		SubscriptionID: sub.ID,
		UserID:         sub.UserID,
		IsActive:       true,
	})
	return nil
}

func (m *mockCompareRepo) GetSubscriptionByID(ctx context.Context, id int64) (*compare.Subscription, error) {
	if s, ok := m.subscriptions[id]; ok {
		return s, nil
	}
	return nil, apperr.NotFound("subscription")
}

func (m *mockCompareRepo) GetActiveSubscription(ctx context.Context, userID int64, orgID *int64) (*compare.Subscription, error) {
	for _, s := range m.subscriptions {
		if s.Status == compare.SubActive && (s.EndsAt == nil || s.EndsAt.After(time.Now().UTC())) {
			if s.UserID == userID || (orgID != nil && s.OrganizationID != nil && *s.OrganizationID == *orgID) {
				if s.Plan == nil {
					s.Plan, _ = m.GetPlanByID(ctx, s.PlanID)
				}
				return s, nil
			}
		}
	}
	return nil, apperr.NotFound("active subscription")
}

func (m *mockCompareRepo) ListSubscriptionsByOrg(ctx context.Context, orgID int64) ([]*compare.Subscription, error) {
	var list []*compare.Subscription
	for _, s := range m.subscriptions {
		if s.OrganizationID != nil && *s.OrganizationID == orgID {
			list = append(list, s)
		}
	}
	return list, nil
}

func (m *mockCompareRepo) UpdateSubscriptionStatus(ctx context.Context, id int64, status compare.SubscriptionStatus) error {
	if s, ok := m.subscriptions[id]; ok {
		s.Status = status
		return nil
	}
	return apperr.NotFound("subscription")
}

func (m *mockCompareRepo) AssignSubscriptionUser(ctx context.Context, subID int64, userID int64) error {
	m.subUsers[subID] = append(m.subUsers[subID], &compare.SubscriptionUser{
		SubscriptionID: subID,
		UserID:         userID,
		IsActive:       true,
	})
	return nil
}

func (m *mockCompareRepo) RemoveSubscriptionUser(ctx context.Context, subID int64, userID int64) error {
	for _, u := range m.subUsers[subID] {
		if u.UserID == userID {
			u.IsActive = false
		}
	}
	return nil
}

func (m *mockCompareRepo) ListSubscriptionUsers(ctx context.Context, subID int64) ([]*compare.SubscriptionUser, error) {
	return m.subUsers[subID], nil
}

func (m *mockCompareRepo) IsUserAssignedToSubscription(ctx context.Context, subID int64, userID int64) (bool, error) {
	for _, u := range m.subUsers[subID] {
		if u.UserID == userID && u.IsActive {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockCompareRepo) UpsertUserSession(ctx context.Context, sess *compare.UserSession) error {
	m.nextID++
	sess.ID = m.nextID
	m.sessions[sess.UserID] = append(m.sessions[sess.UserID], sess)
	return nil
}

func (m *mockCompareRepo) TouchUserSession(ctx context.Context, sessionID string) error {
	return nil
}

func (m *mockCompareRepo) CountActiveUserSessions(ctx context.Context, userID int64) (int, error) {
	count := 0
	for _, s := range m.sessions[userID] {
		if s.IsActive {
			count++
		}
	}
	return count, nil
}

func (m *mockCompareRepo) ListActiveUserSessions(ctx context.Context, userID int64) ([]*compare.UserSession, error) {
	var list []*compare.UserSession
	for _, s := range m.sessions[userID] {
		if s.IsActive {
			list = append(list, s)
		}
	}
	return list, nil
}

func (m *mockCompareRepo) EvictOldestSessions(ctx context.Context, userID int64, keepCount int) error {
	sessions := m.sessions[userID]
	active := 0
	for _, s := range sessions {
		if s.IsActive {
			active++
		}
	}
	if active <= keepCount {
		return nil
	}
	toEvict := active - keepCount
	for _, s := range sessions {
		if s.IsActive && toEvict > 0 {
			s.IsActive = false
			now := time.Now().UTC()
			s.LoggedOutAt = &now
			toEvict--
		}
	}
	return nil
}

func (m *mockCompareRepo) DeactivateUserSession(ctx context.Context, sessionID string) error {
	for _, userSessions := range m.sessions {
		for _, s := range userSessions {
			if s.SessionID == sessionID {
				s.IsActive = false
			}
		}
	}
	return nil
}
