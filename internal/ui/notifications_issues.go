package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/workflow"
)

// notifyAdminNewIssue notifies all platform staff/administrators of a newly reported issue ticket.
func (h *UIHandler) notifyAdminNewIssue(ctx context.Context, issue *workflow.ReportIssue, reporterName, orgName string) {
	if issue == nil {
		return
	}

	typeStr := issueTypeArabic(issue.IssueType)
	priorityStr := issuePriorityArabic(issue.Priority)

	adminTitle := fmt.Sprintf("بلاغ دعم جديد (#%d): %s", issue.ID, typeStr)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("ورد بلاغ جديد رقم (#%d)", issue.ID))
	if reporterName != "" {
		sb.WriteString(fmt.Sprintf(" من المستخدم: %s", reporterName))
	}
	if orgName != "" {
		sb.WriteString(fmt.Sprintf(" (%s)", orgName))
	}
	sb.WriteString(".\n")
	sb.WriteString(fmt.Sprintf("• النوع: %s\n• الأولوية: %s\n", typeStr, priorityStr))
	if issue.OrderID != nil && *issue.OrderID > 0 {
		sb.WriteString(fmt.Sprintf("• رقم الطلب المرتبط: #%d\n", *issue.OrderID))
	}

	descSnippet := issue.Description
	runes := []rune(descSnippet)
	if len(runes) > 150 {
		descSnippet = string(runes[:150]) + "..."
	}
	sb.WriteString(fmt.Sprintf("• التفاصيل: %s\n", descSnippet))
	sb.WriteString("يمكنك مراجعة البلاغ واتخاذ الإجراء اللازم من لوحة تحكم الإدارة (/admin/report-issues).")

	h.dispatchAdminNotification(ctx, "workflow.issue.view", adminTitle, sb.String())
}

// notifyUserIssueResponse notifies the user who created the issue ticket about the admin's response.
func (h *UIHandler) notifyUserIssueResponse(ctx context.Context, issue *workflow.ReportIssue, newStatus, responseNotes string) {
	if issue == nil || issue.ReportedBy <= 0 {
		return
	}

	typeStr := issueTypeArabic(issue.IssueType)
	statusStr := issueStatusArabic(newStatus)

	var userTitle string
	if newStatus == "resolved" {
		userTitle = fmt.Sprintf("تم الرد وحل بلاغك (#%d): %s", issue.ID, typeStr)
	} else if newStatus == "in_progress" {
		userTitle = fmt.Sprintf("تحديث على بلاغك (#%d): جاري المعالجة", issue.ID)
	} else {
		userTitle = fmt.Sprintf("تحديث بشأن بلاغك (#%d)", issue.ID)
	}

	var sb strings.Builder
	cleanNotes := strings.TrimSpace(responseNotes)
	if cleanNotes != "" {
		sb.WriteString(fmt.Sprintf("قام فريق الدعم الفني بالرد على بلاغك رقم (#%d):\n\n\"%s\"\n\n", issue.ID, cleanNotes))
	} else {
		sb.WriteString(fmt.Sprintf("تم تحديث حالة بلاغك رقم (#%d) من قبل فريق الدعم الفني.\n\n", issue.ID))
	}
	sb.WriteString(fmt.Sprintf("الحالة الحالية: %s.\n", statusStr))
	sb.WriteString("يمكنك متابعة تفاصيل البلاغ وسجل تذاكرك من خلال صفحة الدعم الفني (/report-issue).")

	userBody := sb.String()

	// Dispatch in-app notification directly to the reporting user
	h.dispatchInAppNotification(ctx, issue.ReportedBy, issue.OrganizationID, "", userTitle, userBody)

	// If associated with an organization, also notify active members so it shows in the org's notifications feed
	if issue.OrganizationID != nil && *issue.OrganizationID > 0 {
		h.dispatchOrgNotification(ctx, *issue.OrganizationID, "", userTitle, userBody)
	}
}

// issueTypeArabic maps issue type keys to readable Arabic labels.
func issueTypeArabic(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "technical":
		return "مشكلة تقنية أو برمجية"
	case "order":
		return "مشكلة في طلب أو شحنة"
	case "billing":
		return "مشكلة مالية / فواتير / محفظة"
	case "quality":
		return "جودة المنتجات أو مطابقتها"
	case "suggestion":
		return "اقتراح تطوير"
	default:
		return "استفسار أو بلاغ عام"
	}
}

// issuePriorityArabic maps priority keys to readable Arabic labels.
func issuePriorityArabic(p string) string {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "high":
		return "عاجلة (تتطلب تدخلاً فورياً)"
	case "low":
		return "منخفضة"
	default:
		return "متوسطة (عادية)"
	}
}

// issueStatusArabic maps issue status keys to readable Arabic labels.
func issueStatusArabic(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "resolved":
		return "تم الحل بنجاح"
	case "in_progress":
		return "جاري المعالجة والمتابعة"
	default:
		return "قيد المراجعة"
	}
}
