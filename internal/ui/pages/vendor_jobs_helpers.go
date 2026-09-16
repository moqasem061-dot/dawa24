package pages

import (
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/hr"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func formatSalaryForInput(amt money.Amount) string {
	if amt.IsZero() {
		return ""
	}
	if amt.Minor()%100 == 0 {
		return fmt.Sprintf("%d", amt.Minor()/100)
	}
	return amt.String()
}

type VendorJobItem struct {
	Job               *hr.JobOffer
	ApplicationsCount int
}

type VendorJobsData struct {
	Jobs              []*VendorJobItem
	Branches          []*org.Branch
	TotalCount        int
	PublishedCount    int
	ClosedCount       int
	TotalApplications int
	Page              int
	PerPage           int
	NoticeType        string
	NoticeMsg         string
}
