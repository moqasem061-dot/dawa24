package ui

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

type TeamImportPhase = pages.TeamImportPhase

const (
	TeamPhaseUpload    = pages.TeamPhaseUpload
	TeamPhaseMapping   = pages.TeamPhaseMapping
	TeamPhaseReview    = pages.TeamPhaseReview
	TeamPhaseCompleted = pages.TeamPhaseCompleted
)

type TeamDetectedCols = pages.TeamDetectedCols
type ExcelRoleInfo = pages.ExcelRoleInfo
type TeamImportRow = pages.TeamImportRow
type TeamRoleOption = pages.TeamRoleOption
type TeamBranchOption = pages.TeamBranchOption
type TeamImportSession = pages.TeamImportSession
type TeamImportView = pages.TeamImportView

// Global in-memory session store for team imports.
var globalTeamImportSessionStore = NewTeamImportSessionStore()

type TeamImportSessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*TeamImportSession
}

func NewTeamImportSessionStore() *TeamImportSessionStore {
	store := &TeamImportSessionStore{
		sessions: make(map[string]*TeamImportSession),
	}
	// Periodic cleanup of sessions older than 4 hours.
	go func() {
		defer func() {
			_ = recover()
		}()
		for {
			time.Sleep(30 * time.Minute)
			store.cleanup(4 * time.Hour)
		}
	}()
	return store
}

func (s *TeamImportSessionStore) cleanup(maxAge time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().Add(-maxAge)
	for id, sess := range s.sessions {
		if sess.UpdatedAt.Before(cutoff) {
			delete(s.sessions, id)
		}
	}
}

func (s *TeamImportSessionStore) NewSession(orgID, userID int64, orgType, filename string, totalRows int) *TeamImportSession {
	s.mu.Lock()
	defer s.mu.Unlock()

	randBytes := make([]byte, 12)
	_, _ = rand.Read(randBytes)
	sessionID := hex.EncodeToString(randBytes)

	now := time.Now()
	sess := &TeamImportSession{
		ID:             sessionID,
		OrganizationID: orgID,
		OrgType:        orgType,
		UserID:         userID,
		Filename:       filename,
		Phase:          TeamPhaseUpload,
		TotalRows:      totalRows,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.sessions[sessionID] = sess
	return sess
}

func (s *TeamImportSessionStore) GetSession(sessionID string, orgID int64) (*TeamImportSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[sessionID]
	if !ok || sess.OrganizationID != orgID {
		return nil, false
	}
	return sess, true
}

func (s *TeamImportSessionStore) ListSessions(orgID int64) []*TeamImportSession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var list []*TeamImportSession
	for _, sess := range s.sessions {
		if sess.OrganizationID == orgID {
			list = append(list, sess)
		}
	}
	return list
}

func (s *TeamImportSessionStore) DeleteSession(sessionID string, orgID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[sessionID]; ok && sess.OrganizationID == orgID {
		delete(s.sessions, sessionID)
	}
}

// DetectTeamColumns analyzes headers and sample rows to auto-detect employee columns.
func DetectTeamColumns(headers []string, sampleRows [][]string) TeamDetectedCols {
	cols := TeamDetectedCols{
		NameCol:     -1,
		EmailCol:    -1,
		PhoneCol:    -1,
		RoleCol:     -1,
		JobTitleCol: -1,
		BranchCol:   -1,
		CodeCol:     -1,
		NotesCol:    -1,
	}

	nameKeywords := []string{i18n.TDefault("w4_ui.s_105_105"), i18n.TDefault("w4_ui.s_106_106"), i18n.TDefault("w4_ui.s_107_107"), i18n.TDefault("w4_ui.s_108_108"), i18n.TDefault("w4_ui.s_109_109"), i18n.TDefault("w4_ui.s_110_110"), i18n.TDefault("w4_ui.s_30_30"), "employee name", "staff name", "full name", "name", "username"}
	emailKeywords := []string{i18n.T("ar", "excel.col.email"), i18n.TDefault("w4_ui.s_111_111"), i18n.TDefault("w4_ui.s_112_112"), i18n.TDefault("w4_ui.s_113_113"), i18n.TDefault("w4_ui.s_114_114"), i18n.TDefault("w4_ui.s_115_115"), "email", "e-mail", "mail"}
	phoneKeywords := []string{i18n.T("ar", "excel.col.phone"), i18n.TDefault("w4_ui.s_116_116"), i18n.TDefault("w4_ui.s_117_117"), i18n.TDefault("w4_ui.s_118_118"), i18n.TDefault("w4_ui.s_119_119"), i18n.TDefault("w4_ui.s_120_120"), i18n.TDefault("w4_ui.s_121_121"), i18n.TDefault("w4_ui.s_122_122"), i18n.TDefault("w4_ui.s_123_123"), "phone", "mobile", "tel", "cellphone"}
	roleKeywords := []string{i18n.TDefault("w4_ui.s_124_124"), i18n.TDefault("w4_ui.s_125_125"), i18n.TDefault("w4_ui.s_126_126"), i18n.TDefault("w4_ui.s_127_127"), i18n.TDefault("w4_ui.s_128_128"), i18n.TDefault("w4_ui.s_129_129"), "role", "roles", "permission", "user role"}
	jobTitleKeywords := []string{i18n.T("ar", "excel.col.job_title"), i18n.TDefault("w4_ui.s_130_130"), i18n.TDefault("w4_ui.s_131_131"), i18n.TDefault("w4_ui.s_132_132"), "job title", "position", "title", "designation"}
	branchKeywords := []string{i18n.TDefault("w4_ui.s_133_133"), i18n.TDefault("w4_ui.s_134_134"), i18n.TDefault("w4_ui.s_135_135"), i18n.TDefault("w4_ui.s_136_136"), i18n.TDefault("w4_ui.s_137_137"), i18n.TDefault("w4_ui.s_138_138"), "branch", "warehouse", "store", "location"}
	codeKeywords := []string{i18n.T("ar", "excel.col.emp_code"), i18n.TDefault("w4_ui.s_139_139"), i18n.TDefault("w4_ui.s_2_2"), i18n.TDefault("w4_ui.s_140_140"), i18n.TDefault("w4_ui.s_141_141"), i18n.TDefault("w4_ui.s_142_142"), "employee code", "emp code", "code", "staff id", "employee id", "emp id"}
	notesKeywords := []string{i18n.TDefault("w4_ui.s_56_56"), i18n.TDefault("w4_ui.s_143_143"), i18n.TDefault("w4_ui.s_144_144"), i18n.TDefault("w4_ui.s_145_145"), "notes", "remarks", "national id"}

	matchesHeader := func(header string, keywords []string) bool {
		normH := normalizeArabicText(header)
		for _, kw := range keywords {
			normKW := normalizeArabicText(kw)
			if normH == normKW || strings.Contains(normH, normKW) {
				return true
			}
		}
		return false
	}

	for i, h := range headers {
		hTrim := strings.TrimSpace(h)
		if hTrim == "" {
			continue
		}

		if cols.EmailCol == -1 && matchesHeader(hTrim, emailKeywords) {
			cols.EmailCol = i
			continue
		}
		if cols.PhoneCol == -1 && matchesHeader(hTrim, phoneKeywords) {
			cols.PhoneCol = i
			continue
		}
		if cols.NameCol == -1 && matchesHeader(hTrim, nameKeywords) {
			cols.NameCol = i
			continue
		}
		if cols.RoleCol == -1 && matchesHeader(hTrim, roleKeywords) {
			cols.RoleCol = i
			continue
		}
		if cols.JobTitleCol == -1 && matchesHeader(hTrim, jobTitleKeywords) {
			cols.JobTitleCol = i
			continue
		}
		if cols.BranchCol == -1 && matchesHeader(hTrim, branchKeywords) {
			cols.BranchCol = i
			continue
		}
		if cols.CodeCol == -1 && matchesHeader(hTrim, codeKeywords) {
			cols.CodeCol = i
			continue
		}
		if cols.NotesCol == -1 && matchesHeader(hTrim, notesKeywords) {
			cols.NotesCol = i
			continue
		}
	}

	// Content inspection fallback for sample rows if email or phone are still missing
	if cols.EmailCol == -1 && len(sampleRows) > 0 {
		for colIdx := range headers {
			emailHits := 0
			for _, row := range sampleRows {
				if colIdx < len(row) && strings.Contains(row[colIdx], "@") && strings.Contains(row[colIdx], ".") {
					emailHits++
				}
			}
			if emailHits > 0 {
				cols.EmailCol = colIdx
				break
			}
		}
	}

	return cols
}

// extractUniqueExcelRoles extracts all distinct role strings from the role column.
func extractUniqueExcelRoles(rawRows [][]string, roleCol int, companyRoles []TeamRoleOption) []*ExcelRoleInfo {
	roleCounts := make(map[string]int)
	if roleCol >= 0 {
		for _, row := range rawRows {
			if roleCol < len(row) {
				roleName := strings.TrimSpace(row[roleCol])
				if roleName != "" {
					roleCounts[roleName]++
				}
			}
		}
	}

	var results []*ExcelRoleInfo
	for rawRole, count := range roleCounts {
		matchedID, matchedKey, matchedName, isAuto := MatchRoleByName(rawRole, companyRoles)
		results = append(results, &ExcelRoleInfo{
			RawName:        rawRole,
			RowCount:       count,
			MatchedRoleID:  matchedID,
			MatchedRoleKey: matchedKey,
			MatchedName:    matchedName,
			IsAutoMatched:  isAuto,
		})
	}
	return results
}

// GenerateTeamSampleExcel creates a downloadable sample Excel file for employee import.
func GenerateTeamSampleExcel(orgType string) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()

	sheetName := i18n.T("ar", "excel.sheet.employees")
	f.SetSheetName("Sheet1", sheetName)

	headers := []string{
		i18n.T("ar", "excel.col.full_name"),
		i18n.T("ar", "excel.col.email"),
		i18n.T("ar", "excel.col.phone"),
		i18n.T("ar", "excel.col.role"),
		i18n.T("ar", "excel.col.job_title"),
		i18n.T("ar", "excel.col.emp_code"),
		i18n.T("ar", "excel.col.branch_warehouse"),
	}

	var sampleData [][]string
	if orgType == "vendor" {
		sampleData = [][]string{
			{i18n.T("ar", "excel.sample.vendor_emp1_name"), "ahmed.ali@example.com", "01012345678", i18n.T("ar", "role.branch_manager"), i18n.T("ar", "excel.sample.vendor_emp1_title"), "V-EMP-01", i18n.T("ar", "excel.sample.vendor_emp1_wh")},
			{i18n.T("ar", "excel.sample.vendor_emp2_name"), "sara.hassan@example.com", "01123456789", i18n.T("ar", "role.sales_rep"), i18n.T("ar", "excel.sample.vendor_emp2_title"), "V-EMP-02", i18n.T("ar", "excel.sample.vendor_emp2_wh")},
			{i18n.T("ar", "excel.sample.vendor_emp3_name"), "mohamed.kareem@example.com", "01234567890", i18n.T("ar", "role.data_entry"), i18n.T("ar", "excel.sample.vendor_emp3_title"), "V-EMP-03", i18n.T("ar", "excel.sample.vendor_emp1_wh")},
			{i18n.T("ar", "excel.sample.vendor_emp4_name"), "heba.fouad@example.com", "01511223344", i18n.T("ar", "role.accountant"), i18n.T("ar", "excel.sample.vendor_emp4_title"), "V-EMP-04", i18n.T("ar", "excel.sample.vendor_emp4_wh")},
		}
	} else {
		sampleData = [][]string{
			{i18n.T("ar", "excel.sample.pharm_emp1_name"), "dr.ahmed@pharmacy.com", "01012345678", i18n.T("ar", "role.pharmacist"), i18n.T("ar", "excel.sample.pharm_emp1_title"), "PH-001", i18n.T("ar", "excel.sample.vendor_emp1_wh")},
			{i18n.T("ar", "excel.sample.pharm_emp2_name"), "dr.reem@pharmacy.com", "01123456789", i18n.T("ar", "role.pharmacist"), i18n.T("ar", "excel.sample.pharm_emp2_title"), "PH-002", i18n.T("ar", "excel.sample.pharm_emp2_wh")},
			{i18n.T("ar", "excel.sample.pharm_emp3_name"), "tarek@pharmacy.com", "01234567890", i18n.T("ar", "role.data_entry"), i18n.T("ar", "excel.sample.pharm_emp3_title"), "PH-003", i18n.T("ar", "excel.sample.vendor_emp1_wh")},
			{i18n.T("ar", "excel.sample.pharm_emp4_name"), "mona@pharmacy.com", "01511223344", i18n.T("ar", "role.branch_manager"), i18n.T("ar", "excel.sample.pharm_emp4_title"), "PH-004", i18n.T("ar", "excel.sample.pharm_emp4_wh")},
		}
	}

	// Style header
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "#FFFFFF", Size: 11},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#0284C7"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})

	for colIdx, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, 1)
		_ = f.SetCellValue(sheetName, cell, h)
		_ = f.SetCellStyle(sheetName, cell, cell, headerStyle)
		_ = f.SetColWidth(sheetName, string(rune('A'+colIdx)), string(rune('A'+colIdx)), 22)
	}

	for rowIdx, row := range sampleData {
		for colIdx, val := range row {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, rowIdx+2)
			_ = f.SetCellValue(sheetName, cell, val)
		}
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
