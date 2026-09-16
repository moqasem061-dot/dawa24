package pages

type TeamMemberView struct {
	ID           int64
	UserID       int64
	Name         string
	Email        string
	Phone        string
	JobTitle     string
	EmployeeCode string
	BranchID     int64
	BranchName   string
	RoleKey      string
	RoleName     string
	// RoleID is the company's own role this member holds. Roles are per
	// company, so this is meaningless outside the organization that owns it.
	RoleID       int64
	IsActive     bool
	AvatarURL    string
	CreatedAt    string
}

type BranchOption struct {
	ID   int64
	Name string
}

type VendorTeamData struct {
	NoticeType    string
	NoticeMsg     string
	Members       []*TeamMemberView
	Branches      []*BranchOption
	// CompanyRoles are this company's own roles, the only ones assignable
	// here. A role belonging to another company is not in the list and would
	// be refused on submit even if it were.
	CompanyRoles  []TenantRoleOption
	// CanAssignRole gates the selector. Whoever may assign a role can hand
	// themselves everything that role holds, so it is its own permission.
	CanAssignRole bool
	CurrentUserID int64
	Page          int
	PerPage       int
	TotalCount    int
}

func countActiveMembers(members []*TeamMemberView) int {
	count := 0
	for _, m := range members {
		if m.IsActive {
			count++
		}
	}
	return count
}

func countManagers(members []*TeamMemberView) int {
	count := 0
	for _, m := range members {
		if m.RoleKey == "org_owner" || m.RoleKey == "org_manager" {
			count++
		}
	}
	return count
}

func getInitials(name string) string {
	runes := []rune(name)
	if len(runes) == 0 {
		return "م"
	}
	if len(runes) > 1 && (runes[0] == 'د' || runes[0] == 'D') && (runes[1] == '.' || runes[1] == ' ') {
		trimmed := []rune(name)
		for i := 2; i < len(trimmed); i++ {
			if trimmed[i] != ' ' && trimmed[i] != '.' {
				return string(trimmed[i])
			}
		}
	}
	return string(runes[0])
}

func ifElseStr(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}
