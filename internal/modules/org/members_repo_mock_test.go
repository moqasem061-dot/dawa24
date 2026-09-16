package org

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// The member-addressed half of the mock repository, split from org_test.go for
// the 400-line rule.

func (m *mockOrgRepo) GetMember(_ context.Context, orgID, memberID int64) (*Member, error) {
	for _, mem := range m.members[orgID] {
		if mem.ID == memberID {
			return mem, nil
		}
	}
	return nil, apperr.NotFound("member")
}

func (m *mockOrgRepo) UpdateMember(_ context.Context, orgID, memberID int64, patch MemberPatch) error {
	for _, mem := range m.members[orgID] {
		if mem.ID != memberID {
			continue
		}
		if patch.BranchID != nil {
			mem.BranchID = patch.BranchID
		}
		if patch.RoleKey != nil {
			mem.RoleKey = *patch.RoleKey
		}
		if patch.OrgRoleID != nil {
			mem.OrgRoleID = patch.OrgRoleID
		}
		if patch.EmployeeCode != nil {
			mem.EmployeeCode = *patch.EmployeeCode
		}
		if patch.JobTitle != nil {
			mem.JobTitle = *patch.JobTitle
		}
		if patch.IsActive != nil {
			mem.IsActive = *patch.IsActive
		}
		return nil
	}
	return apperr.NotFound("member")
}

func (m *mockOrgRepo) CountMembersByBranch(_ context.Context, orgID int64) (map[int64]int, error) {
	out := map[int64]int{}
	for _, mem := range m.members[orgID] {
		if mem.BranchID != nil {
			out[*mem.BranchID]++
		}
	}
	return out, nil
}

func (m *mockOrgRepo) MemberOrganizations(_ context.Context, userID int64) ([]int64, error) {
	var ids []int64
	for orgID, list := range m.members {
		for _, mem := range list {
			if mem.UserID == userID {
				ids = append(ids, orgID)
				break
			}
		}
	}
	return ids, nil
}
