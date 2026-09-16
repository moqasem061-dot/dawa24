package org

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

func profileService() (*Service, *mockOrgRepo) {
	repo := newMockOrgRepo()
	return NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil))), repo
}

// The identity section waits for a moderator; every other section applies now.
//
// Those four fields — legal name, trade name, commercial register, tax number —
// are what the platform checked against the company's papers when it approved
// them. Letting a supplier rewrite them afterwards leaves an approved record
// that no longer describes what was approved. A phone number is not that, and
// making someone wait for a moderator to fix one would be its own bug.
func TestProfileSectionApprovalPolicy(t *testing.T) {
	ctx := context.Background()

	t.Run("identity waits for review", func(t *testing.T) {
		svc, repo := profileService()
		res, err := svc.SaveProfileSection(ctx, SaveProfileSection{
			OrganizationID: 1, UserID: 7, Section: SectionIdentity,
			Fields: ProfileFields{"legal_name": "شركة ويزر فارما", "trade_name_ar": "ويزر"},
		})
		if err != nil {
			t.Fatalf("SaveProfileSection: %v", err)
		}
		if res.Applied {
			t.Error("an identity change applied immediately; it must be reviewed")
		}
		if res.Request == nil || res.Request.Status != ChangePending {
			t.Fatal("no pending request was opened")
		}
		stored, _ := repo.ReadProfileSection(ctx, 1, SectionIdentity)
		if stored["legal_name"] == "شركة ويزر فارما" {
			t.Error("the organization was changed before the request was decided")
		}
	})

	for _, section := range []ProfileSection{SectionLimits, SectionContact, SectionDescription, SectionMedia} {
		t.Run(string(section)+" applies immediately", func(t *testing.T) {
			svc, repo := profileService()
			fields := ProfileFields{"phone": "0100", "email": "a@b.c", "address": "x",
				"organization_number": "1", "description_ar": "وصف", "description_en": "desc",
				"image": "/logo.png", "coverage_image": "/cover.png",
				"min_order_price": "10.00", "max_order_price": "500.00"}
			res, err := svc.SaveProfileSection(ctx, SaveProfileSection{
				OrganizationID: 1, UserID: 7, Section: section, Fields: fields,
			})
			if err != nil {
				t.Fatalf("SaveProfileSection: %v", err)
			}
			if !res.Applied || res.Request != nil {
				t.Error("the section did not apply immediately")
			}
			if stored, _ := repo.ReadProfileSection(ctx, 1, section); len(stored) == 0 {
				t.Error("nothing was written")
			}
		})
	}
}

// One open request per section. A second submission has to say so rather than
// queueing a second answer to the same question.
func TestProfileSectionRefusesASecondPendingRequest(t *testing.T) {
	ctx := context.Background()
	svc, _ := profileService()

	in := SaveProfileSection{
		OrganizationID: 1, UserID: 7, Section: SectionIdentity,
		Fields: ProfileFields{"legal_name": "الأول"},
	}
	if _, err := svc.SaveProfileSection(ctx, in); err != nil {
		t.Fatalf("first request: %v", err)
	}
	in.Fields = ProfileFields{"legal_name": "الثاني"}
	if _, err := svc.SaveProfileSection(ctx, in); err == nil {
		t.Error("a second pending request for the same section was accepted")
	}
}

// A request that changes nothing is not opened at all: it would block the
// section behind a pending marker and cost a moderator a decision for no reason.
func TestProfileSectionIgnoresANoOpChange(t *testing.T) {
	ctx := context.Background()
	svc, repo := profileService()

	_ = repo.ApplyProfileSection(ctx, 1, SectionIdentity, ProfileFields{"legal_name": "نفس الاسم"})

	res, err := svc.SaveProfileSection(ctx, SaveProfileSection{
		OrganizationID: 1, UserID: 7, Section: SectionIdentity,
		Fields: ProfileFields{"legal_name": "نفس الاسم"},
	})
	if err != nil {
		t.Fatalf("SaveProfileSection: %v", err)
	}
	if res.Request != nil {
		t.Error("a request was opened for a change that changes nothing")
	}
}

// Approving applies the proposal; rejecting leaves the company alone and has to
// say why.
func TestProfileChangeDecision(t *testing.T) {
	ctx := context.Background()

	t.Run("approve applies", func(t *testing.T) {
		svc, repo := profileService()
		res, _ := svc.SaveProfileSection(ctx, SaveProfileSection{
			OrganizationID: 1, UserID: 7, Section: SectionIdentity,
			Fields: ProfileFields{"legal_name": "الاسم الجديد"},
		})
		if _, err := svc.DecideProfileChangeRequest(ctx, res.Request.ID, 99, true, ""); err != nil {
			t.Fatalf("approve: %v", err)
		}
		stored, _ := repo.ReadProfileSection(ctx, 1, SectionIdentity)
		if stored["legal_name"] != "الاسم الجديد" {
			t.Errorf("approval did not apply the change, stored %q", stored["legal_name"])
		}
	})

	t.Run("reject leaves the company alone", func(t *testing.T) {
		svc, repo := profileService()
		res, _ := svc.SaveProfileSection(ctx, SaveProfileSection{
			OrganizationID: 1, UserID: 7, Section: SectionIdentity,
			Fields: ProfileFields{"legal_name": "اسم مرفوض"},
		})
		if _, err := svc.DecideProfileChangeRequest(ctx, res.Request.ID, 99, false, "الأوراق غير مطابقة"); err != nil {
			t.Fatalf("reject: %v", err)
		}
		stored, _ := repo.ReadProfileSection(ctx, 1, SectionIdentity)
		if stored["legal_name"] == "اسم مرفوض" {
			t.Error("a rejected change was applied")
		}
	})

	t.Run("a rejection must give a reason", func(t *testing.T) {
		svc, _ := profileService()
		res, _ := svc.SaveProfileSection(ctx, SaveProfileSection{
			OrganizationID: 1, UserID: 7, Section: SectionIdentity,
			Fields: ProfileFields{"legal_name": "اسم"},
		})
		if _, err := svc.DecideProfileChangeRequest(ctx, res.Request.ID, 99, false, ""); err == nil {
			t.Error("a rejection with no reason was accepted")
		}
	})

	t.Run("a decided request cannot be decided twice", func(t *testing.T) {
		svc, _ := profileService()
		res, _ := svc.SaveProfileSection(ctx, SaveProfileSection{
			OrganizationID: 1, UserID: 7, Section: SectionIdentity,
			Fields: ProfileFields{"legal_name": "اسم"},
		})
		if _, err := svc.DecideProfileChangeRequest(ctx, res.Request.ID, 99, true, ""); err != nil {
			t.Fatalf("first decision: %v", err)
		}
		if _, err := svc.DecideProfileChangeRequest(ctx, res.Request.ID, 99, false, "تراجع"); err == nil {
			t.Error("a decided request was decided again")
		}
	})
}

// Withdrawing frees the section for a new attempt.
func TestProfileChangeWithdrawal(t *testing.T) {
	ctx := context.Background()
	svc, _ := profileService()

	res, _ := svc.SaveProfileSection(ctx, SaveProfileSection{
		OrganizationID: 1, UserID: 7, Section: SectionIdentity,
		Fields: ProfileFields{"legal_name": "اسم"},
	})
	if err := svc.WithdrawProfileChangeRequest(ctx, 1, res.Request.ID); err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if _, err := svc.SaveProfileSection(ctx, SaveProfileSection{
		OrganizationID: 1, UserID: 7, Section: SectionIdentity,
		Fields: ProfileFields{"legal_name": "اسم آخر"},
	}); err != nil {
		t.Errorf("the section is still blocked after a withdrawal: %v", err)
	}

	// And one company cannot withdraw another's request.
	res2, _ := svc.SaveProfileSection(ctx, SaveProfileSection{
		OrganizationID: 2, UserID: 8, Section: SectionIdentity,
		Fields: ProfileFields{"legal_name": "منشأة أخرى"},
	})
	if err := svc.WithdrawProfileChangeRequest(ctx, 1, res2.Request.ID); err == nil {
		t.Error("one organization withdrew another's request")
	}
}
