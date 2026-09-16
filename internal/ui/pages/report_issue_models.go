package pages

import (
	"fmt"
	"net/url"

	"github.com/muhiya/dawa24-store/internal/modules/workflow"
)

// AdminReportIssuesData encapsulates all data required to render the admin issues queue.
type AdminReportIssuesData struct {
	Issues      []*workflow.ReportIssueDetail
	Total       int
	Stats       *workflow.ReportIssueStats
	Status      string
	IssueType   string
	Priority    string
	Search      string
	CurrentPage int
	TotalPages  int
	PageSize    int
}

// IssueTypeLabel returns the localized Arabic label for an issue type.
func IssueTypeLabel(issueType string) string {
	switch issueType {
	case "technical":
		return "مشكلة تقنية أو برمجية"
	case "order":
		return "مشكلة في طلب أو شحنة"
	case "billing":
		return "مشكلة مالية أو فواتير"
	case "suggestion":
		return "اقتراح تحسين وتطوير"
	case "quality":
		return "جودة الأصناف والمطابقة"
	case "delivery_delay":
		return "تأخر وصول الشحنة"
	default:
		if issueType == "" {
			return "عام"
		}
		return issueType
	}
}

// IssueStatusLabel returns the localized Arabic label for an issue status.
func IssueStatusLabel(status string) string {
	switch status {
	case "pending":
		return "قيد المراجعة"
	case "in_progress":
		return "جاري المعالجة"
	case "resolved":
		return "تم الحل بنجاح"
	default:
		return status
	}
}

// IssuePriorityLabel returns the localized Arabic label for priority.
func IssuePriorityLabel(priority string) string {
	switch priority {
	case "high":
		return "عاجل"
	case "medium":
		return "متوسط"
	case "low":
		return "منخفض"
	default:
		return "متوسط"
	}
}

// PaginationURL generates a URL query string with updated page number preserving existing filters.
func (d AdminReportIssuesData) PaginationURL(page int) string {
	q := url.Values{}
	if d.Status != "" {
		q.Set("status", d.Status)
	}
	if d.IssueType != "" {
		q.Set("type", d.IssueType)
	}
	if d.Priority != "" {
		q.Set("priority", d.Priority)
	}
	if d.Search != "" {
		q.Set("q", d.Search)
	}
	if page > 1 {
		q.Set("page", fmt.Sprintf("%d", page))
	}
	encoded := q.Encode()
	if encoded == "" {
		return "/admin/report-issues"
	}
	return "/admin/report-issues?" + encoded
}

// FilterURL generates a URL query string with updated status tab preserving search.
func (d AdminReportIssuesData) FilterURL(status string) string {
	q := url.Values{}
	if status != "" {
		q.Set("status", status)
	}
	if d.IssueType != "" {
		q.Set("type", d.IssueType)
	}
	if d.Priority != "" {
		q.Set("priority", d.Priority)
	}
	if d.Search != "" {
		q.Set("q", d.Search)
	}
	encoded := q.Encode()
	if encoded == "" {
		return "/admin/report-issues"
	}
	return "/admin/report-issues?" + encoded
}
