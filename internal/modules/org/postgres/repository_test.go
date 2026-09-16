package postgres_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	dbfs "github.com/muhiya/dawa24-store/db"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/org/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func getTestDB(t *testing.T) *database.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database integration test in short mode")
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("TEST_DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := config.Database{
		URL:              dbURL,
		MaxConns:         5,
		MinConns:         1,
		MaxConnLifetime:  time.Hour,
		MaxConnIdleTime:  30 * time.Minute,
		StatementTimeout: 10 * time.Second,
	}

	db, err := database.Open(ctx, cfg)
	if err != nil {
		t.Skipf("cannot connect to database: %v", err)
	}

	migrations, err := database.LoadMigrations(dbfs.Migrations, "migrations")
	if err != nil {
		t.Fatalf("failed to load migrations: %v", err)
	}

	pending, err := db.PendingCount(ctx, migrations)
	if err != nil {
		t.Fatalf("cannot read migration state: %v", err)
	}
	if pending > 0 {
		t.Fatalf("%d migrations pending", pending)
	}
	return db
}

const testUserID int64 = 88590

func resetFixtures(t *testing.T, db *database.DB) {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	err := db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, `DELETE FROM org.organization_deletion_requests WHERE organization_id IN (SELECT id FROM org.organizations WHERE legal_name LIKE 'Test Org %')`); err != nil {
			return fmt.Errorf("delete deletion requests: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM org.organization_policies WHERE organization_id IN (SELECT id FROM org.organizations WHERE legal_name LIKE 'Test Org %')`); err != nil {
			return fmt.Errorf("delete policies: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM org.organization_followers WHERE user_id = $1`, testUserID); err != nil {
			return fmt.Errorf("delete followers: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM org.organization_reviews WHERE user_id = $1`, testUserID); err != nil {
			return fmt.Errorf("delete reviews: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM org.members WHERE user_id = $1`, testUserID); err != nil {
			return fmt.Errorf("delete members: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM org.branches WHERE organization_id IN (SELECT id FROM org.organizations WHERE legal_name LIKE 'Test Org %')`); err != nil {
			return fmt.Errorf("delete branches: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM org.organizations WHERE legal_name LIKE 'Test Org %'`); err != nil {
			return fmt.Errorf("delete orgs: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM identity.users WHERE id = $1`, testUserID); err != nil {
			return fmt.Errorf("delete user: %w", err)
		}

		// Create user for member, follower, review checks
		if _, err := tx.Exec(txCtx, `INSERT INTO identity.users (id, email, password_hash, name, role)
			VALUES ($1, 'user88590@example.com', 'x', '{"ar":"مستخدم","en":"User"}'::jsonb, 'customer')
			ON CONFLICT (id) DO UPDATE SET email = EXCLUDED.email`, testUserID); err != nil {
			return fmt.Errorf("insert user: %w", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reset fixtures: %v", err)
	}
}

func TestOrgRepository(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()
	resetFixtures(t, db)

	repo := postgres.NewRepository(db)
	ctx := context.Background()

	var orgID int64
	var branchID int64

	t.Run("Create and Get Organization", func(t *testing.T) {
		o := &org.Organization{
			LegalName:          "Test Org Alpha",
			CommercialRegister: "CR-ALPHA-01",
			Type:               org.TypeCustomer,
			Status:             org.StatusPending,
			CreditLimit:        money.FromMinor(500000),
			PaymentTermsDays:   30,
		}
		err := repo.CreateOrganization(ctx, o)
		if err != nil {
			t.Fatalf("failed to create org: %v", err)
		}
		if o.ID == 0 {
			t.Fatalf("expected non-zero org ID")
		}
		orgID = o.ID

		fetched, err := repo.GetOrganizationByID(ctx, orgID)
		if err != nil {
			t.Fatalf("failed to get org: %v", err)
		}
		if fetched.LegalName != o.LegalName {
			t.Errorf("got %q, want %q", fetched.LegalName, o.LegalName)
		}
		if fetched.CreditLimit.Minor() != 500000 {
			t.Errorf("got %d, want 500000", fetched.CreditLimit.Minor())
		}

		err = repo.UpdateOrganizationStatus(ctx, orgID, org.StatusApproved)
		if err != nil {
			t.Fatalf("failed to update status: %v", err)
		}

		o.LegalName = "Test Org Alpha Updated"
		err = repo.UpdateOrganization(ctx, o)
		if err != nil {
			t.Fatalf("failed to update org: %v", err)
		}

		list, err := repo.ListOrganizations(ctx, nil, nil, 10, 0)
		if err != nil || len(list) == 0 {
			t.Fatalf("failed to list orgs: %v", err)
		}
	})

	t.Run("Branches", func(t *testing.T) {
		b := &org.Branch{
			OrganizationID: orgID,
			Name:           nil,
			Code:           "BR-01",
			Address:        "123 Nile St",
			IsMain:         true,
		}
		if err := repo.CreateBranch(ctx, b); err != nil {
			t.Fatalf("failed to create branch: %v", err)
		}
		if b.ID == 0 {
			t.Fatal("expected generated branch ID")
		}
		branchID = b.ID

		fetched, err := repo.GetBranchByID(ctx, branchID)
		if err != nil {
			t.Fatalf("failed to get branch: %v", err)
		}
		if fetched.Code != "BR-01" {
			t.Errorf("got %q, want BR-01", fetched.Code)
		}

		branches, err := repo.ListBranchesByOrg(ctx, orgID)
		if err != nil || len(branches) == 0 {
			t.Fatalf("failed to list branches: %v", err)
		}

		b.Code = "BR-01-UPDATED"
		if err := repo.UpdateBranch(ctx, b); err != nil {
			t.Fatalf("failed to update branch: %v", err)
		}

		if err := repo.UnsetMainBranches(ctx, orgID); err != nil {
			t.Fatalf("failed to unset main branches: %v", err)
		}

		if err := repo.DeleteBranch(ctx, branchID, orgID); err != nil {
			t.Fatalf("failed to delete branch: %v", err)
		}
	})

	t.Run("Members", func(t *testing.T) {
		m := &org.Member{
			OrganizationID: orgID,
			UserID:         testUserID,
			RoleID:         1,
			IsActive:       true,
		}
		if err := repo.AddMember(ctx, m); err != nil {
			t.Fatalf("failed to add member: %v", err)
		}

		members, err := repo.ListMembersByOrg(ctx, orgID)
		if err != nil || len(members) == 0 {
			t.Fatalf("failed to list members: %v", err)
		}

		if err := repo.UpdateMemberRole(ctx, orgID, testUserID, "org_manager"); err != nil {
			t.Fatalf("failed to update member role: %v", err)
		}

		if err := repo.RemoveMember(ctx, orgID, testUserID); err != nil {
			t.Fatalf("failed to remove member: %v", err)
		}
	})

	t.Run("Followers and Reviews", func(t *testing.T) {
		following, err := repo.ToggleFollower(ctx, orgID, testUserID)
		if err != nil {
			t.Fatalf("failed to toggle follower: %v", err)
		}
		if !following {
			t.Error("expected to be following")
		}

		isF, err := repo.IsFollowing(ctx, orgID, testUserID)
		if err != nil || !isF {
			t.Fatalf("expected isFollowing to be true: %v", err)
		}

		rev := &org.Review{
			OrganizationID: orgID,
			UserID:         testUserID,
			Rating:         5,
			ReviewText:     "Great service",
			// ListReviewsByOrg returns approved reviews only, by design.
			IsApproved: true,
		}
		if err := repo.AddReview(ctx, rev); err != nil {
			t.Fatalf("failed to add review: %v", err)
		}

		reviews, err := repo.ListReviewsByOrg(ctx, orgID, 10, 0)
		if err != nil {
			t.Fatalf("failed to list reviews: %v", err)
		}
		// Reported separately from the error. Combining them printed
		// "failed to list reviews: <nil>", which says nothing about an empty result.
		if len(reviews) == 0 {
			t.Fatalf("listed 0 reviews for org %d after adding one", orgID)
		}
	})

	t.Run("Policies", func(t *testing.T) {
		p := &org.Policy{
			OrganizationID: orgID,
			PolicyType:     "returns",
			Title:          "Return Policy",
			Content:        "14 days return policy for sealed items",
			// ListPoliciesByOrg returns active policies only, by design.
			IsActive: true,
		}
		if err := repo.CreatePolicy(ctx, p); err != nil {
			t.Fatalf("failed to create policy: %v", err)
		}

		policies, err := repo.ListPoliciesByOrg(ctx, orgID)
		if err != nil {
			t.Fatalf("failed to list policies: %v", err)
		}
		if len(policies) == 0 {
			t.Fatalf("listed 0 policies for org %d after creating one", orgID)
		}
	})

	t.Run("Organization Deletion Requests", func(t *testing.T) {
		// 1. Create deletion request
		req := &org.OrganizationDeletionRequest{
			OrganizationID: orgID,
			RequestedBy:    testUserID,
			Reason:         "Testing org deletion flow",
		}
		if err := repo.CreateOrgDeletionRequest(ctx, req); err != nil {
			t.Fatalf("failed to create deletion request: %v", err)
		}
		if req.ID == 0 || req.Status != org.OrgDeletionStatusPending {
			t.Fatalf("expected pending request, got %+v", req)
		}

		// 2. Duplicate pending request must fail
		reqDup := &org.OrganizationDeletionRequest{
			OrganizationID: orgID,
			RequestedBy:    testUserID,
			Reason:         "Duplicate request",
		}
		if err := repo.CreateOrgDeletionRequest(ctx, reqDup); err == nil {
			t.Fatalf("expected duplicate pending request to fail")
		}

		// 3. Get pending deletion request
		pending, err := repo.GetPendingOrgDeletionRequest(ctx, orgID)
		if err != nil {
			t.Fatalf("failed to get pending deletion request: %v", err)
		}
		if pending == nil || pending.ID != req.ID {
			t.Fatalf("got %v, want %d", pending, req.ID)
		}

		// 4. List deletion requests with filter
		list, total, err := repo.ListOrgDeletionRequests(ctx, "pending", 10, 0)
		if err != nil {
			t.Fatalf("failed to list deletion requests: %v", err)
		}
		if total == 0 || len(list) == 0 {
			t.Fatalf("expected at least 1 pending request in list")
		}

		// 5. Cancel deletion request
		if err := repo.CancelOrgDeletionRequest(ctx, orgID, req.ID); err != nil {
			t.Fatalf("failed to cancel deletion request: %v", err)
		}
		cancelled, err := repo.GetOrgDeletionRequest(ctx, req.ID)
		if err != nil || cancelled.Status != org.OrgDeletionStatusCancelled {
			t.Fatalf("expected cancelled status, got %v (err: %v)", cancelled, err)
		}

		// 6. Create another request and approve it
		req2 := &org.OrganizationDeletionRequest{
			OrganizationID: orgID,
			RequestedBy:    testUserID,
			Reason:         "Second request for approval",
		}
		if err := repo.CreateOrgDeletionRequest(ctx, req2); err != nil {
			t.Fatalf("failed to create 2nd deletion request: %v", err)
		}

		reviewed, err := repo.ReviewOrgDeletionRequest(ctx, req2.ID, testUserID, true, "Approved for decommission")
		if err != nil {
			t.Fatalf("failed to approve deletion request: %v", err)
		}
		if reviewed == nil || reviewed.Status != org.OrgDeletionStatusApproved {
			t.Fatalf("expected approved status, got %v", reviewed)
		}

		// Verify organization is now marked terminal status 'deleted' and row intact
		deactivatedOrg, err := repo.GetOrganizationByID(ctx, orgID)
		if err != nil {
			t.Fatalf("failed to get organization: %v", err)
		}
		if deactivatedOrg.Status != org.StatusDeleted {
			t.Errorf("expected organization to have status 'deleted' after deletion approval, got %s", deactivatedOrg.Status)
		}
	})
}
