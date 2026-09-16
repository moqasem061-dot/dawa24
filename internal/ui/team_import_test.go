package ui_test

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// TestDetectTeamColumns tests auto-detection of column headers in English and Arabic.
func TestDetectTeamColumns(t *testing.T) {
	headersArabic := []string{"اسم الموظف بالكامل", "البريد الإلكتروني", "رقم الموبايل", "الدور / الصلاحية", "المسمى الوظيفي", "الفرع", "كود الموظف"}
	sampleRows := [][]string{
		{"أحمد علي", "ahmed@example.com", "01012345678", "مدير", "مدير مبيعات", "الفرع الرئيسي", "EMP-01"},
	}

	cols := ui.DetectTeamColumns(headersArabic, sampleRows)
	assert.Equal(t, 0, cols.NameCol)
	assert.Equal(t, 1, cols.EmailCol)
	assert.Equal(t, 2, cols.PhoneCol)
	assert.Equal(t, 3, cols.RoleCol)
	assert.Equal(t, 4, cols.JobTitleCol)
	assert.Equal(t, 5, cols.BranchCol)
	assert.Equal(t, 6, cols.CodeCol)

	headersEnglish := []string{"Employee Name", "E-mail", "Phone Number", "Role", "Job Title", "Branch", "Staff ID"}
	colsEn := ui.DetectTeamColumns(headersEnglish, sampleRows)
	assert.Equal(t, 0, colsEn.NameCol)
	assert.Equal(t, 1, colsEn.EmailCol)
	assert.Equal(t, 2, colsEn.PhoneCol)
	assert.Equal(t, 3, colsEn.RoleCol)
	assert.Equal(t, 4, colsEn.JobTitleCol)
	assert.Equal(t, 5, colsEn.BranchCol)
	assert.Equal(t, 6, colsEn.CodeCol)
}

// TestMatchRoleByName tests fuzzy & semantic matching of raw role strings from Excel.
func TestMatchRoleByName(t *testing.T) {
	companyRoles := []pages.TeamRoleOption{
		{ID: 10, Key: "org_owner", Name: "مالك المنشأة", IsOwner: true},
		{ID: 20, Key: "org_admin", Name: "مدير المنشأة", IsOwner: false},
		{ID: 30, Key: "org_pharmacist", Name: "صيدلي مسؤول", IsOwner: false},
		{ID: 40, Key: "org_employee", Name: "موظف عادي", IsOwner: false},
	}

	// 1. Pharmacist
	rID, rKey, _, isAuto := ui.MatchRoleByName("دكتور صيدلي", companyRoles)
	assert.True(t, isAuto)
	assert.Equal(t, int64(30), rID)
	assert.Equal(t, "org_pharmacist", rKey)

	// 2. Manager / Admin
	rID, rKey, _, isAuto = ui.MatchRoleByName("مدير فرع", companyRoles)
	assert.True(t, isAuto)
	assert.Equal(t, int64(20), rID)
	assert.Equal(t, "org_admin", rKey)

	// 3. Cashier / Employee
	rID, rKey, _, isAuto = ui.MatchRoleByName("كاشير ومبيعات", companyRoles)
	assert.True(t, isAuto)
	assert.Equal(t, int64(40), rID)
	assert.Equal(t, "org_employee", rKey)

	// 4. Exact match
	rID, rKey, _, isAuto = ui.MatchRoleByName("صيدلي مسؤول", companyRoles)
	assert.True(t, isAuto)
	assert.Equal(t, int64(30), rID)
	assert.Equal(t, "org_pharmacist", rKey)

	// 5. Owner role must NEVER be matched or returned
	rID, rKey, _, isAuto = ui.MatchRoleByName("مالك المنشأة", companyRoles)
	assert.False(t, isAuto)
	assert.NotEqual(t, int64(10), rID)
	assert.NotEqual(t, "org_owner", rKey)

	rID, rKey, _, isAuto = ui.MatchRoleByName("owner", companyRoles)
	assert.False(t, isAuto)
	assert.NotEqual(t, int64(10), rID)
	assert.NotEqual(t, "org_owner", rKey)
}

// TestParseAndValidateTeamRows tests data validation (valid vs invalid emails, duplicate detection, branch assignment).
func TestParseAndValidateTeamRows(t *testing.T) {
	rawRows := [][]string{
		{"أحمد علي", "ahmed@example.com", "01012345678", "مدير", "مدير مبيعات", "فرع التجمع", "EMP-01"},
		{"سارة حسن", "sara@example.com", "01123456789", "صيدلي", "صيدلانية", "المقر الرئيسي", "EMP-02"},
		{"موظف مكرر البريد", "ahmed@example.com", "01234567890", "موظف", "مبيعات", "", "EMP-03"},
		{"موظف بدون بريد", "", "01511223344", "موظف", "عامل", "", "EMP-04"},
		{"", "", "", "", "", "", ""}, // Empty row to skip
	}

	cols := pages.TeamDetectedCols{
		NameCol:     0,
		EmailCol:    1,
		PhoneCol:    2,
		RoleCol:     3,
		JobTitleCol: 4,
		BranchCol:   5,
		CodeCol:     6,
	}

	companyRoles := []pages.TeamRoleOption{
		{ID: 10, Key: "org_admin", Name: "مدير المنشأة"},
		{ID: 20, Key: "org_pharmacist", Name: "صيدلي مسؤول"},
		{ID: 30, Key: "org_employee", Name: "موظف"},
	}

	branches := []pages.TeamBranchOption{
		{ID: 100, Name: "المقر الرئيسي", Code: "HQ"},
		{ID: 200, Name: "فرع التجمع", Code: "TAG"},
	}

	roleMap := map[string]int64{
		"مدير":  10,
		"صيدلي": 20,
		"موظف":  30,
	}

	rows := ui.ParseAndValidateTeamRows(rawRows, cols, roleMap, 30, companyRoles, branches)
	require.Len(t, rows, 4) // 4 non-empty rows

	// Row 1: Valid
	assert.True(t, rows[0].IsValid)
	assert.Equal(t, "ahmed@example.com", rows[0].Email)
	assert.Equal(t, int64(10), rows[0].AssignedRoleID)
	require.NotNil(t, rows[0].AssignedBranchID)
	assert.Equal(t, int64(200), *rows[0].AssignedBranchID)

	// Row 2: Valid
	assert.True(t, rows[1].IsValid)
	assert.Equal(t, "sara@example.com", rows[1].Email)
	assert.Equal(t, int64(20), rows[1].AssignedRoleID)
	require.NotNil(t, rows[1].AssignedBranchID)
	assert.Equal(t, int64(100), *rows[1].AssignedBranchID)

	// Row 3: Duplicate email -> Invalid
	assert.False(t, rows[2].IsValid)
	assert.Contains(t, rows[2].ValidationError, "مكرر")

	// Row 4: Missing email -> Invalid
	assert.False(t, rows[3].IsValid)
	assert.Contains(t, rows[3].ValidationError, "البريد الإلكتروني غير صحيح أو مفقود")
}

// TestGenerateTeamSampleExcel tests the downloadable Excel template generator.
func TestGenerateTeamSampleExcel(t *testing.T) {
	for _, orgType := range []string{"vendor", "customer"} {
		fileBytes, err := ui.GenerateTeamSampleExcel(orgType)
		require.NoError(t, err)
		require.NotEmpty(t, fileBytes)

		// Verify that it is a valid Excel spreadsheet
		f, err := excelize.OpenReader(bytes.NewReader(fileBytes))
		require.NoError(t, err)
		defer f.Close()

		rows, err := f.GetRows("الموظفون")
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(rows), 4) // header + at least 3 sample rows
		assert.Equal(t, "اسم الموظف بالكامل", rows[0][0])
	}
}

// TestTeamImportSessionStore tests in-memory session operations.
func TestTeamImportSessionStore(t *testing.T) {
	store := ui.NewTeamImportSessionStore()

	sess := store.NewSession(555, 12, "vendor", "test_team.xlsx", 10)
	require.NotEmpty(t, sess.ID)
	assert.Equal(t, int64(555), sess.OrganizationID)
	assert.Equal(t, pages.TeamPhaseUpload, sess.Phase)

	// Retrieve
	retrieved, ok := store.GetSession(sess.ID, 555)
	assert.True(t, ok)
	assert.Equal(t, sess.ID, retrieved.ID)

	// Isolation by org ID
	_, wrongOrg := store.GetSession(sess.ID, 999)
	assert.False(t, wrongOrg)

	// List
	list := store.ListSessions(555)
	assert.Len(t, list, 1)

	// Delete
	store.DeleteSession(sess.ID, 555)
	_, deleted := store.GetSession(sess.ID, 555)
	assert.False(t, deleted)
}

// TestVendorAndCustomerTeamImportEndpoints tests HTTP handlers for team imports.
func TestVendorAndCustomerTeamImportEndpoints(t *testing.T) {
	h := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(h)

	vendorActor := authctx.Actor{
		UserID:         10,
		OrganizationID: 100,
		OrgID:          100,
		OrgType:        "vendor",
		Permissions:    []string{"vendor.team.view", "vendor.team.create"},
	}

	customerActor := authctx.Actor{
		UserID:         20,
		OrganizationID: 200,
		OrgID:          200,
		OrgType:        "customer",
		Permissions:    []string{"pharmacy.team.view", "pharmacy.team.create"},
	}

	// 1. GET /vendor/team/import
	rec := doGET(t, r, "/vendor/team/import", vendorActor)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "استيراد فريق العمل والموظفين")

	// 2. GET /vendor/team/import/sample.xlsx
	rec = doGET(t, r, "/vendor/team/import/sample.xlsx", vendorActor)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", rec.Header().Get("Content-Type"))

	// 3. GET /customer/team/import
	rec = doGET(t, r, "/customer/team/import", customerActor)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "استيراد فريق العمل والموظفين")

	// 4. GET /customer/team/import/sample.xlsx
	rec = doGET(t, r, "/customer/team/import/sample.xlsx", customerActor)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", rec.Header().Get("Content-Type"))
}

// TestTeamImportFlow_MultipartUploadAndMapping tests complete import workflow.
func TestTeamImportFlow_MultipartUploadAndMapping(t *testing.T) {
	h := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(h)

	vendorActor := authctx.Actor{
		UserID:         10,
		OrganizationID: 100,
		OrgID:          100,
		OrgType:        "vendor",
		Permissions:    []string{"vendor.team.view", "vendor.team.create"},
	}

	// 1. Generate sample excel
	sampleBytes, err := ui.GenerateTeamSampleExcel("vendor")
	require.NoError(t, err)

	// 2. POST /vendor/team/import/upload
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "sample_team.xlsx")
	require.NoError(t, err)
	_, err = part.Write(sampleBytes)
	require.NoError(t, err)
	writer.Close()

	req := httptest.NewRequest("POST", "/vendor/team/import/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = req.WithContext(authctx.WithActor(req.Context(), vendorActor))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	redirectURL := rec.Header().Get("Location")
	require.Contains(t, redirectURL, "/vendor/team/import/")

	// Extract session ID
	parts := strings.Split(redirectURL, "/")
	sessionID := parts[len(parts)-1]

	// 3. GET session mapping stage
	rec = doGET(t, r, fmt.Sprintf("/vendor/team/import/%s", sessionID), vendorActor)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "ربط الأعمدة وتعيين أدوار الموظفين")

	// 4. POST mapping stage
	formValues := url.Values{
		"col_name":        []string{"0"},
		"col_email":       []string{"1"},
		"col_phone":       []string{"2"},
		"col_role":        []string{"3"},
		"col_job_title":   []string{"4"},
		"col_code":        []string{"5"},
		"col_branch":      []string{"6"},
		"default_role_id": []string{"1"},
	}
	rec = doPOST(t, r, fmt.Sprintf("/vendor/team/import/%s/map", sessionID), formValues, vendorActor)
	assert.Equal(t, http.StatusSeeOther, rec.Code)

	// 5. GET review stage
	rec = doGET(t, r, fmt.Sprintf("/vendor/team/import/%s", sessionID), vendorActor)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "مراجعة وتأكيد بيانات الموظفين")

	// 6. POST cancel
	rec = doPOST(t, r, fmt.Sprintf("/vendor/team/import/%s/cancel", sessionID), url.Values{}, vendorActor)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Contains(t, rec.Header().Get("Location"), "/vendor/team/import")
}
