package ui

import (
	"fmt"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// normalizeArabicText simplifies Arabic text for fuzzy matching.
func normalizeArabicText(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	replacements := map[rune]rune{
		'أ': 'ا', 'إ': 'ا', 'آ': 'ا', 'ٱ': 'ا',
		'ة': 'ه', 'ى': 'ي', 'ؤ': 'و', 'ئ': 'ي',
		'ـ': ' ',
	}
	var sb strings.Builder
	for _, r := range s {
		if sub, ok := replacements[r]; ok {
			sb.WriteRune(sub)
		} else {
			sb.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(sb.String()), " ")
}

// MatchRoleByName finds the most appropriate platform role for a raw Excel role name.
func MatchRoleByName(rawRole string, companyRoles []TeamRoleOption, langOptional ...string) (int64, string, string, bool) {
	lang := "ar"
	if len(langOptional) > 0 && langOptional[0] != "" {
		lang = langOptional[0]
	}
	normRaw := normalizeArabicText(rawRole)

	// 1. Exact match on role name (excluding owner roles)
	for _, cr := range companyRoles {
		if !cr.IsOwner && cr.Key != "org_owner" && normalizeArabicText(cr.Name) == normRaw {
			return cr.ID, cr.Key, cr.Name, true
		}
	}

	// 2. Substring & Semantic matching
	type rolePattern struct {
		keywords []string
		keys     []string
	}
	patterns := []rolePattern{
		{
			keywords: []string{i18n.TDefault("w4_ui.s_146_146"), i18n.TDefault("w4_ui.s_147_147"), i18n.TDefault("w4_ui.s_148_148"), i18n.TDefault("w4_ui.s_149_149"), "pharmacist", "pharmacy"},
			keys:     []string{"org_pharmacist", "pharmacist", "staff_pharmacist"},
		},
		{
			keywords: []string{i18n.TDefault("w4_ui.s_150_150"), i18n.TDefault("w4_ui.s_151_151"), i18n.TDefault("w4_ui.s_152_152"), i18n.TDefault("w4_ui.s_153_153"), i18n.TDefault("w4_ui.s_154_154"), "manager", "admin", "supervisor", "lead"},
			keys:     []string{"org_admin", "admin", "branch_manager", "manager"},
		},
		{
			keywords: []string{i18n.TDefault("w4_ui.s_158_158"), i18n.TDefault("w4_ui.s_159_159"), i18n.TDefault("w4_ui.s_160_160"), i18n.TDefault("w4_ui.s_161_161"), i18n.TDefault("w4_ui.s_162_162"), i18n.TDefault("w4_ui.s_163_163"), "employee", "cashier", "sales", "accountant", "staff"},
			keys:     []string{"org_employee", "employee", "staff"},
		},
	}

	for _, pat := range patterns {
		matches := false
		for _, kw := range pat.keywords {
			if strings.Contains(normRaw, normalizeArabicText(kw)) {
				matches = true
				break
			}
		}
		if matches {
			// Find among companyRoles by key or name (excluding owner roles)
			for _, targetKey := range pat.keys {
				for _, cr := range companyRoles {
					if !cr.IsOwner && cr.Key != "org_owner" {
						if cr.Key == targetKey || strings.Contains(normalizeArabicText(cr.Name), normalizeArabicText(targetKey)) {
							return cr.ID, cr.Key, cr.Name, true
						}
					}
				}
			}
		}
	}

	// Fallback to the first non-owner role if available
	for _, cr := range companyRoles {
		if !cr.IsOwner && cr.Key != "org_owner" {
			return cr.ID, cr.Key, cr.Name, false
		}
	}

	return 0, "org_employee", i18n.T(lang, "team.import.default_role_name"), false
}

// ParseAndValidateTeamRows builds TeamImportRow items from raw rows and mappings.
func ParseAndValidateTeamRows(
	rawRows [][]string,
	cols TeamDetectedCols,
	roleMap map[string]int64, // rawRole -> roleID
	defaultRoleID int64,
	companyRoles []TeamRoleOption,
	branches []TeamBranchOption,
	langOptional ...string,
) []*TeamImportRow {
	lang := "ar"
	if len(langOptional) > 0 && langOptional[0] != "" {
		lang = langOptional[0]
	}
	var rows []*TeamImportRow

	roleByID := make(map[int64]TeamRoleOption)
	for _, cr := range companyRoles {
		roleByID[cr.ID] = cr
	}

	branchByName := make(map[string]TeamBranchOption)
	for _, b := range branches {
		branchByName[normalizeArabicText(b.Name)] = b
		if b.Code != "" {
			branchByName[normalizeArabicText(b.Code)] = b
		}
	}

	seenEmails := make(map[string]int)

	for i, rawRow := range rawRows {
		rowIndex := i + 1

		getValue := func(col int) string {
			if col >= 0 && col < len(rawRow) {
				return strings.TrimSpace(rawRow[col])
			}
			return ""
		}

		name := getValue(cols.NameCol)
		email := strings.ToLower(getValue(cols.EmailCol))
		phone := getValue(cols.PhoneCol)
		rawRole := getValue(cols.RoleCol)
		jobTitle := getValue(cols.JobTitleCol)
		rawBranch := getValue(cols.BranchCol)
		code := getValue(cols.CodeCol)

		// Skip completely empty rows
		if name == "" && email == "" && phone == "" {
			continue
		}

		// Fallback name if missing
		if name == "" && email != "" {
			name = strings.Split(email, "@")[0]
		}

		// Resolve role
		targetRoleID := defaultRoleID
		if rID, ok := roleMap[rawRole]; ok && rID > 0 {
			targetRoleID = rID
		}

		roleOpt, hasRole := roleByID[targetRoleID]
		roleKey := "org_employee"
		roleName := i18n.T(lang, "team.import.role_employee")
		if hasRole {
			roleKey = roleOpt.Key
			roleName = roleOpt.Name
		}

		// Fallback job title
		if jobTitle == "" {
			if rawRole != "" {
				jobTitle = rawRole
			} else {
				jobTitle = roleName
			}
		}

		// Fallback employee code
		if code == "" {
			code = fmt.Sprintf("EMP-%03d", rowIndex)
		}

		// Resolve branch
		var assignedBranchID *int64
		var assignedBranchName string
		if rawBranch != "" {
			normB := normalizeArabicText(rawBranch)
			if b, ok := branchByName[normB]; ok {
				assignedBranchID = &b.ID
				assignedBranchName = b.Name
			} else {
				// Partial match
				for _, b := range branches {
					if strings.Contains(normalizeArabicText(b.Name), normB) || strings.Contains(normB, normalizeArabicText(b.Name)) {
						assignedBranchID = &b.ID
						assignedBranchName = b.Name
						break
					}
				}
			}
		}

		// Validation
		isValid := true
		var valErr string

		if email == "" || !strings.Contains(email, "@") || !strings.Contains(email, ".") {
			isValid = false
			valErr = i18n.T(lang, "team.import.email_invalid_or_missing")
		} else if prevRow, duplicate := seenEmails[email]; duplicate {
			isValid = false
			valErr = fmt.Sprintf(i18n.T(lang, "team.import.email_duplicate_in_file"), prevRow)
		} else {
			seenEmails[email] = rowIndex
		}

		if name == "" {
			isValid = false
			if valErr != "" {
				valErr += " | "
			}
			valErr += i18n.T(lang, "team.import.employee_name_missing")
		}

		status := "ready"
		if !isValid {
			status = "skipped"
		}

		rows = append(rows, &TeamImportRow{
			Index:              rowIndex,
			RawName:            name,
			Email:              email,
			Phone:              phone,
			RawRole:            rawRole,
			AssignedRoleID:     targetRoleID,
			AssignedRoleKey:    roleKey,
			AssignedRoleName:   roleName,
			JobTitle:           jobTitle,
			EmployeeCode:       code,
			RawBranch:          rawBranch,
			AssignedBranchID:   assignedBranchID,
			AssignedBranchName: assignedBranchName,
			IsValid:            isValid,
			ValidationError:    valErr,
			ImportStatus:       status,
		})
	}

	return rows
}
