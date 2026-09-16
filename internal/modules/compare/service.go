package compare

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/storage"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// ClientInfo captures client environment details for session device cap tracking.
type ClientInfo struct {
	SessionID       string
	DeviceUUID      string
	DeviceName      string
	DeviceType      string
	Platform        string
	PlatformVersion string
	Browser         string
	BrowserVersion  string
	IPAddress       string
	UserAgent       string
	Country         string
	City            string
}

// Service coordinates compare plans, subscriptions, user seats, and entitlements.
type Service struct {
	repo      Repository
	log       *slog.Logger
	aiMatcher AIMatcher
	storage   *storage.Client
	// cache holds the market board's total and supplier list. Nil is a
	// supported state: every read falls through to the database.
	cache Cache
}

// NewService creates a new compare service.
func NewService(repo Repository, log *slog.Logger) *Service {
	return &Service{
		repo: repo,
		log:  log,
	}
}

// SetAIMatcher configures the optional AI matching capability (Wave B / Plan V5 §2.6).
func (s *Service) SetAIMatcher(m AIMatcher) {
	s.aiMatcher = m
}

// AIMatchingAvailable reports whether AI matching is enabled.
func (s *Service) AIMatchingAvailable() bool {
	return s != nil && s.aiMatcher != nil
}

// SetStorage configures the object storage client for downloading uploaded files.
func (s *Service) SetStorage(st *storage.Client) {
	s.storage = st
}

// EntitlementFor answers "what may this user do in the compare tool right now?".
// All subscription/plan paywalls are removed as per user directive.
func (s *Service) EntitlementFor(ctx context.Context, userID, orgID int64) (Entitlement, error) {
	return Entitlement{
		Active:            true,
		PlanSlug:          "unlimited",
		MaxActiveFiles:    100,
		MaxSessions:       10,
		AIMatchingEnabled: true,
	}, nil
}

// EnforceSessionCap logs the current user session and evicts oldest sessions if exceeding max allowed (Laravel parity).
func (s *Service) EnforceSessionCap(ctx context.Context, userID int64, subUserID *int64, maxSessions int, client ClientInfo) error {
	if maxSessions <= 0 {
		maxSessions = 1
	}

	sess := &UserSession{
		SubscriptionUserID: subUserID,
		UserID:             userID,
		SessionID:          client.SessionID,
		DeviceUUID:         client.DeviceUUID,
		IsActive:           true,
		DeviceName:         client.DeviceName,
		DeviceType:         client.DeviceType,
		Platform:           client.Platform,
		PlatformVersion:    client.PlatformVersion,
		Browser:            client.Browser,
		BrowserVersion:     client.BrowserVersion,
		IPAddress:          client.IPAddress,
		UserAgent:          client.UserAgent,
		Country:            client.Country,
		City:               client.City,
		LoggedInAt:         time.Now().UTC(),
		LastActivityAt:     time.Now().UTC(),
	}

	if err := s.repo.UpsertUserSession(ctx, sess); err != nil {
		return err
	}

	// Check active session count and evict oldest if necessary
	count, err := s.repo.CountActiveUserSessions(ctx, userID)
	if err != nil {
		return err
	}

	if count > maxSessions {
		s.log.InfoContext(ctx, "evicting oldest compare sessions for user", "user_id", userID, "active", count, "cap", maxSessions)
		return s.repo.EvictOldestSessions(ctx, userID, maxSessions)
	}

	return nil
}

// ListPlans returns public pricing tiers or all tiers for admins.
func (s *Service) ListPlans(ctx context.Context, onlyPublic bool) ([]*Plan, error) {
	return s.repo.ListPlans(ctx, onlyPublic)
}

// GetPlan retrieves a single plan by ID or slug.
func (s *Service) GetPlan(ctx context.Context, id int64) (*Plan, error) {
	return s.repo.GetPlanByID(ctx, id)
}

func (s *Service) GetPlanBySlug(ctx context.Context, slug string) (*Plan, error) {
	return s.repo.GetPlanBySlug(ctx, slug)
}

// CreatePlan adds a new plan (requires admin permission).
func (s *Service) CreatePlan(ctx context.Context, plan *Plan) (*Plan, error) {
	if plan.Slug == "" {
		return nil, apperr.Validation("plan.slug_required", "Plan slug is required.", nil)
	}
	if plan.Name.IsEmpty() {
		return nil, apperr.Validation("plan.name_required", "Plan name is required.", nil)
	}
	if err := s.repo.CreatePlan(ctx, plan); err != nil {
		return nil, err
	}
	return plan, nil
}

// UpdatePlan modifies plan metadata and prices.
func (s *Service) UpdatePlan(ctx context.Context, plan *Plan) error {
	return s.repo.UpdatePlan(ctx, plan)
}

// DeletePlan soft-deletes a plan.
func (s *Service) DeletePlan(ctx context.Context, id int64) error {
	return s.repo.DeletePlan(ctx, id)
}

// RequestPlan submits a self-serve enrollment request from a customer or vendor.
func (s *Service) RequestPlan(ctx context.Context, planID, orgID, userID int64, notes string) (*PlanRequest, error) {
	if planID <= 0 || orgID <= 0 || userID <= 0 {
		return nil, apperr.Validation("request.invalid_params", "Plan, organization, and user IDs are required.", nil)
	}

	req := &PlanRequest{
		PlanID:         planID,
		OrganizationID: orgID,
		UserID:         userID,
		Status:         RequestPending,
		Notes:          notes,
	}

	if err := s.repo.CreatePlanRequest(ctx, req); err != nil {
		return nil, err
	}
	return req, nil
}

// ListPlanRequests retrieves requests for an organization.
func (s *Service) ListPlanRequests(ctx context.Context, orgID int64) ([]*PlanRequest, error) {
	return s.repo.ListPlanRequestsByOrg(ctx, orgID)
}

// ListPendingPlanRequests lists requests for admin review.
func (s *Service) ListPendingPlanRequests(ctx context.Context) ([]*PlanRequest, error) {
	return s.repo.ListPendingPlanRequests(ctx)
}

// ReviewPlanRequest processes approval or rejection of a plan request by an administrator.
func (s *Service) ReviewPlanRequest(ctx context.Context, requestID int64, approve bool, reviewerID int64, reason string) error {
	req, err := s.repo.GetPlanRequestByID(ctx, requestID)
	if err != nil {
		return err
	}

	if !approve {
		return s.repo.ReviewPlanRequest(ctx, requestID, RequestRejected, reviewerID, reason)
	}

	// Approval creates the subscription
	plan, err := s.repo.GetPlanByID(ctx, req.PlanID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	var endsAt *time.Time
	if plan.TrialDays > 0 {
		t := now.AddDate(0, 0, plan.TrialDays)
		endsAt = &t
	} else {
		t := now.AddDate(0, 1, 0) // default 1 month
		endsAt = &t
	}

	sub := &Subscription{
		PlanID:         req.PlanID,
		OrganizationID: &req.OrganizationID,
		UserID:         req.UserID,
		BillingPeriod:  "monthly",
		PaymentMethod:  "cash",
		StartsAt:       now,
		EndsAt:         endsAt,
		Status:         SubActive,
		Seats:          1,
	}

	if err := s.repo.CreateSubscription(ctx, sub); err != nil {
		return err
	}

	return s.repo.ReviewPlanRequest(ctx, requestID, RequestApproved, reviewerID, "Approved")
}

// SubscribeDirectly creates a direct active subscription (e.g. for self-serve or testing).
func (s *Service) SubscribeDirectly(ctx context.Context, planSlug string, orgID *int64, userID int64, period string) (*Subscription, error) {
	plan, err := s.repo.GetPlanBySlug(ctx, planSlug)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	var endsAt *time.Time
	switch period {
	case "yearly":
		t := now.AddDate(1, 0, 0)
		endsAt = &t
	case "lifetime":
		endsAt = nil
	case "trial":
		days := plan.TrialDays
		if days <= 0 {
			days = 7
		}
		t := now.AddDate(0, 0, days)
		endsAt = &t
	default:
		t := now.AddDate(0, 1, 0)
		endsAt = &t
		period = "monthly"
	}

	sub := &Subscription{
		PlanID:         plan.ID,
		OrganizationID: orgID,
		UserID:         userID,
		BillingPeriod:  period,
		PaymentMethod:  "cash",
		StartsAt:       now,
		EndsAt:         endsAt,
		Status:         SubActive,
		Seats:          1,
	}

	if err := s.repo.CreateSubscription(ctx, sub); err != nil {
		return nil, err
	}
	sub.Plan = plan
	return sub, nil
}

// UploadCompareFile validates user entitlement, applies auto-archive retention if at max active capacity, and creates the compare file.
func (s *Service) UploadCompareFile(ctx context.Context, userID int64, orgID *int64, supplierName, originalFilename, mimeType string, sizeBytes int64, storageKey string) (*CompareFile, []string, error) {
	ent, err := s.EntitlementFor(ctx, userID, orgIDValue(orgID))
	if err != nil {
		return nil, nil, err
	}
	if !ent.Active {
		return nil, nil, apperr.Forbidden("compare.unentitled", "An active compare subscription is required to upload supplier files.")
	}

	if supplierName == "" {
		supplierName = strings.TrimSuffix(originalFilename, ".xlsx")
		supplierName = strings.TrimSuffix(supplierName, ".xls")
		supplierName = strings.TrimSuffix(supplierName, ".csv")
	}

	// 50MB file size cap check
	if sizeBytes > 50*1024*1024 {
		return nil, nil, apperr.Validation("file.too_large", "File size exceeds 50MB limit.", nil)
	}

	archivedNames, err := s.MakeRoomForFiles(ctx, userID, orgID, 1)
	if err != nil {
		return nil, nil, err
	}

	file := &CompareFile{
		OrganizationID:   orgID,
		UserID:           userID,
		SupplierName:     supplierName,
		OriginalFilename: originalFilename,
		StorageKey:       storageKey,
		MIMEType:         mimeType,
		SizeBytes:        sizeBytes,
		Status:           FileUploaded,
	}

	if err := s.repo.CreateFile(ctx, file); err != nil {
		return nil, nil, err
	}

	return file, archivedNames, nil
}

// evictMu serialises the read-count-then-archive step of an upload.
//
// That step is a check-then-act on shared state, and the batch uploader runs
// six of them at once. Concurrently, every worker read the same active count,
// every worker decided the quota was full, and every worker then archived
// "everything beyond keepCount" — which by then included the files its siblings
// had just created. Upload eight files and two survived: the rest were archived
// by their own batch. The lock makes count-and-archive atomic; MakeRoomForFiles
// makes it happen once for a whole batch rather than once per file.
var evictMu sync.Mutex

// MakeRoomForFiles archives the oldest active compare files, if any need to be
// archived, so that `incoming` more can be created without exceeding the
// caller's plan. It returns the names it archived, for the notice the screen
// shows.
//
// Call it once per batch, before uploading any of the batch's files. Calling it
// per file inside a parallel batch is what the lock below exists to survive,
// not a supported way to use it.
func (s *Service) MakeRoomForFiles(ctx context.Context, userID int64, orgID *int64, incoming int) ([]string, error) {
	if incoming <= 0 {
		return nil, nil
	}
	ent, err := s.EntitlementFor(ctx, userID, orgIDValue(orgID))
	if err != nil {
		return nil, err
	}
	if ent.MaxActiveFiles <= 0 {
		return nil, nil
	}

	evictMu.Lock()
	defer evictMu.Unlock()

	activeCount, err := s.repo.CountActiveFiles(ctx, userID, orgID)
	if err != nil {
		// A failed count must not archive anything: archiving on a guess is how
		// a user loses files they never replaced.
		return nil, nil
	}
	if activeCount+incoming <= ent.MaxActiveFiles {
		return nil, nil
	}

	keepCount := ent.MaxActiveFiles - incoming
	if keepCount < 0 {
		keepCount = 0
	}
	reason := fmt.Sprintf(i18n.T("ar", "err.compare_quota_exceeded"), strconv.Itoa(ent.MaxActiveFiles))
	archived, _ := s.repo.ArchiveOldestFiles(ctx, userID, orgID, keepCount, reason)
	return archived, nil
}

// orgIDValue flattens an optional organisation to the zero value the
// entitlement lookup expects.
func orgIDValue(orgID *int64) int64 {
	if orgID == nil {
		return 0
	}
	return *orgID
}
