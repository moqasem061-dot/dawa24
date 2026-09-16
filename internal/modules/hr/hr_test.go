package hr_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/hr"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type mockHRRepo struct {
	employees map[int64]*hr.Employee
	workTimes []*hr.WorkTime
	nextID    int64
}

func newMockHRRepo() *mockHRRepo {
	return &mockHRRepo{
		employees: map[int64]*hr.Employee{},
		nextID:    1,
	}
}

func (m *mockHRRepo) CreateEmployee(_ context.Context, e *hr.Employee) error {
	e.ID = m.nextID
	m.nextID++
	m.employees[e.ID] = e
	return nil
}

func (m *mockHRRepo) GetEmployeeByID(_ context.Context, id int64) (*hr.Employee, error) {
	e, ok := m.employees[id]
	if !ok {
		return nil, apperr.NotFound("employee")
	}
	return e, nil
}

func (m *mockHRRepo) ListEmployees(_ context.Context, limit, offset int) ([]*hr.Employee, error) {
	var list []*hr.Employee
	for _, e := range m.employees {
		list = append(list, e)
	}
	return list, nil
}

func (m *mockHRRepo) SaveWorkTimes(_ context.Context, times []*hr.WorkTime) error {
	m.workTimes = times
	return nil
}

func (m *mockHRRepo) ListWorkTimes(_ context.Context) ([]*hr.WorkTime, error) {
	return m.workTimes, nil
}

func (m *mockHRRepo) ListPublishedJobs(_ context.Context, _, _ int) ([]*hr.JobOffer, error) {
	return nil, nil
}

func (m *mockHRRepo) ListPublishedJobsWithTotal(_ context.Context, _, _ int) ([]*hr.JobOffer, int, error) {
	return nil, 0, nil
}

func (m *mockHRRepo) ListAllJobsWithTotal(_ context.Context, _, _ int) ([]*hr.JobOffer, int, error) {
	return nil, 0, nil
}

func (m *mockHRRepo) ListAllJobsFiltered(_ context.Context, _ hr.AdminJobFilter) ([]*hr.JobOffer, int, error) {
	return nil, 0, nil
}

func (m *mockHRRepo) GetJobOfferByID(_ context.Context, _ int64) (*hr.JobOffer, error) {
	return nil, nil
}

func (m *mockHRRepo) CreateJobOffer(_ context.Context, j *hr.JobOffer) error {
	j.ID = 1
	return nil
}

func (m *mockHRRepo) UpdateJobOffer(_ context.Context, _ *hr.JobOffer) error {
	return nil
}

func (m *mockHRRepo) DeleteJobOffer(_ context.Context, _, _ int64) error {
	return nil
}

func (m *mockHRRepo) ToggleJobOfferStatus(_ context.Context, _, _ int64) error {
	return nil
}

func (m *mockHRRepo) ListJobsByOrg(_ context.Context, _ int64, _, _ int) ([]*hr.JobOffer, error) {
	return nil, nil
}

func (m *mockHRRepo) ListJobsByOrgWithTotal(_ context.Context, _ int64, _, _ int) ([]*hr.JobOffer, int, error) {
	return nil, 0, nil
}

func (m *mockHRRepo) GetJobStatsByOrg(_ context.Context, _ int64) (hr.JobStatsResult, error) {
	return hr.JobStatsResult{}, nil
}

func (m *mockHRRepo) CreateJobApplication(_ context.Context, a *hr.JobApplication) error {
	a.ID = 1
	return nil
}

func (m *mockHRRepo) ListApplicationsByOffer(_ context.Context, _ int64, _, _ int) ([]*hr.JobApplication, error) {
	return nil, nil
}

func (m *mockHRRepo) CountApplicationsByOffer(_ context.Context, _ int64) (int, error) {
	return 0, nil
}

func (m *mockHRRepo) ListApplicationsByUser(_ context.Context, _ int64) ([]*hr.JobApplication, error) {
	return nil, nil
}

func (m *mockHRRepo) GetApplicationByID(_ context.Context, id int64) (*hr.JobApplication, error) {
	return &hr.JobApplication{ID: id, Status: "pending"}, nil
}

func (m *mockHRRepo) AcceptAndOnboardApplicant(_ context.Context, in hr.AcceptApplicantInput) (*hr.JobApplication, error) {
	return &hr.JobApplication{ID: in.ApplicationID, Status: "accepted", AssignedRoleKey: in.RoleKey}, nil
}

func (m *mockHRRepo) RejectApplicant(_ context.Context, orgID, appID int64, notes string) (*hr.JobApplication, error) {
	return &hr.JobApplication{ID: appID, Status: "rejected", Notes: notes}, nil
}

func (m *mockHRRepo) GetJobSeekerProfile(_ context.Context, userID int64) (*hr.JobSeekerProfile, error) {
	return &hr.JobSeekerProfile{ID: 1, UserID: userID, Specialisation: "pharmacist"}, nil
}

func (m *mockHRRepo) UpsertJobSeekerProfile(_ context.Context, p *hr.JobSeekerProfile) error {
	p.ID = 1
	return nil
}

func TestHREmployeesAndWorkTimes(t *testing.T) {

	ctx := database.WithTenant(context.Background(), 30)
	repo := newMockHRRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := hr.NewService(repo, logger)

	// 1. Create Employee
	emp, err := svc.CreateEmployee(ctx, &hr.Employee{
		UserID:         55,
		EmployeeCode:   "EMP-001",
		JobTitle:       "Pharmacist",
		BaseSalary:     money.MustParse("12000.00"),
		VariableSalary: money.MustParse("2000.00"),
	})
	if err != nil {
		t.Fatalf("CreateEmployee failed: %v", err)
	}
	if emp.BaseSalary != money.MustParse("12000.00") || emp.Status != "active" {
		t.Errorf("unexpected employee state: %+v", emp)
	}

	// 2. Save Work Times
	err = svc.SaveWorkTimes(ctx, []*hr.WorkTime{
		{DayNameAr: "السبت", DayNameEn: "Saturday", OpenTime: "08:00", CloseTime: "22:00", SortOrder: 1},
	})
	if err != nil {
		t.Fatalf("SaveWorkTimes failed: %v", err)
	}

	times, _ := svc.ListWorkTimes(ctx)
	if len(times) != 1 || times[0].DayNameEn != "Saturday" {
		t.Errorf("unexpected work times: %+v", times)
	}
}
