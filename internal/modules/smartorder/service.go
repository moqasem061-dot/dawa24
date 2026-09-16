package smartorder

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Service is the use-case layer for smart ordering.
type Service struct {
	repo Repository
	gate AvailabilityGate
	log  *slog.Logger
}

// NewService constructs the service.
func NewService(repo Repository, log *slog.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// SetAvailabilityGate wires the unified purchase availability gate.
func (s *Service) SetAvailabilityGate(gate AvailabilityGate) {
	s.gate = gate
}

// StartOptions is what the buyer chose on the import screen.
type StartOptions struct {
	UserID            int64
	OrganizationID    int64
	BranchID          int64
	UploadID          *int64
	Filename          string
	Criteria          []Criterion
	TolerancePct      float64
	DefaultQuantity   int
	MaxBudget         *money.Amount
	UseSavingProducts bool
	UseAIMatching     bool
	// MinMatchScore is the buyer's أقل نسبة مطابقة, 0–1. Zero means the
	// platform default; NewConfig clamps it.
	MinMatchScore float64
	MatchLanguage string
}

// Start creates a run and remembers the configuration for next time.
func (s *Service) Start(ctx context.Context, opts StartOptions) (*Run, error) {
	if opts.BranchID == 0 {
		return nil, apperr.Validation("smartorder.branch_required",
			"choose the branch this order will be delivered to", nil)
	}

	profile := Profile{
		OrganizationID:    opts.OrganizationID,
		Criteria:          opts.Criteria,
		TolerancePct:      opts.TolerancePct,
		DefaultQuantity:   opts.DefaultQuantity,
		UseSavingProducts: opts.UseSavingProducts,
		UseAIMatching:     opts.UseAIMatching,
		MinMatchScore:     opts.MinMatchScore,
		MatchLanguage:     opts.MatchLanguage,
		LastBranchID:      &opts.BranchID,
	}

	run := &Run{
		OrganizationID: opts.OrganizationID,
		UserID:         opts.UserID,
		BranchID:       opts.BranchID,
		UploadID:       opts.UploadID,
		OriginalFile:   opts.Filename,
		Status:         StatusDraft,
		CurrentStep:    1,
		AI:             AIUsage{Enabled: opts.UseAIMatching},
	}

	cfg, err := NewConfig(0, opts.OrganizationID, profile, opts.MaxBudget)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CreateRun(ctx, run, cfg); err != nil {
		return nil, err
	}

	// Remembering the configuration is a convenience, not part of the run.
	// Failing the whole start because a preference could not be saved would be
	// a poor trade.
	if err := s.repo.SaveProfile(ctx, &profile); err != nil {
		s.log.WarnContext(ctx, "could not remember smart order preferences",
			"organization_id", opts.OrganizationID, "error", err)
	}
	return run, nil
}

// Get loads a run the caller is entitled to see by its public ID.
func (s *Service) Get(ctx context.Context, orgID int64, publicID string) (*Run, error) {
	return s.repo.GetRunByPublicID(ctx, orgID, publicID)
}

// GetByID loads a run by its internal numeric ID.
func (s *Service) GetByID(ctx context.Context, orgID, id int64) (*Run, error) {
	return s.repo.GetRunByID(ctx, orgID, id)
}

// History lists the buyer's previous runs.
func (s *Service) History(ctx context.Context, orgID int64, limit, offset int) ([]*Run, error) {
	return s.repo.ListRuns(ctx, orgID, limit, offset)
}

// ConfirmMapping saves the confirmed column mapping and readies the run.
//
// The mapping is validated here rather than only in the handler, because a run
// that starts on an unusable mapping produces confidently wrong results — worse
// than producing none.
func (s *Service) ConfirmMapping(ctx context.Context, run *Run, m *Mapping) error {
	if ok, missing := m.Valid(); !ok {
		return apperr.Validation("smartorder.mapping_incomplete",
			"map a column to the product name, SKU or barcode before starting",
			map[string]string{"missing": strings.Join(missing, ", ")})
	}
	m.RunID = run.ID
	m.OrganizationID = run.OrganizationID
	m.Confirmed = true
	if err := s.repo.SaveMapping(ctx, m); err != nil {
		return err
	}
	if err := run.TransitionTo(StatusMapping); err != nil {
		return err
	}
	return s.repo.UpdateRunStatus(ctx, run.ID, run.Status, run.CurrentStep, "")
}

// Queue hands the run to the worker.
func (s *Service) Queue(ctx context.Context, run *Run) error {
	if err := run.TransitionTo(StatusQueued); err != nil {
		return err
	}
	return s.repo.UpdateRunStatus(ctx, run.ID, run.Status, run.CurrentStep, "")
}

// Results returns a filtered page of lines, each carrying the catalogue name of
// the product it matched.
//
// The name is resolved here rather than by every screen that renders lines,
// because every one of them needs it: a results table, a review cart or an
// export that shows only "#255741" has given the buyer no way to tell whether
// the match is right. One extra query per page, keyed by the ids actually on
// it.
func (s *Service) Results(ctx context.Context, run *Run, f LineFilter) ([]*Line, int, error) {
	lines, total, err := s.repo.ListLines(ctx, run.ID, f)
	if err != nil {
		return nil, 0, err
	}
	s.attachProductNames(ctx, lines)
	return lines, total, nil
}

// FilterCounts returns accurate row counts for each independent smart order filter.
func (s *Service) FilterCounts(ctx context.Context, runID int64) (FilterCounts, error) {
	if s.repo == nil {
		return FilterCounts{}, nil
	}
	return s.repo.FilterCounts(ctx, runID)
}

// BlockedCounts is what the run will not order, split by why.
func (s *Service) BlockedCounts(ctx context.Context, runID int64) (BlockedCounts, error) {
	if s.repo == nil {
		return BlockedCounts{}, nil
	}
	return s.repo.BlockedCounts(ctx, runID)
}

// attachProductNames fills in the display name of each matched product.
//
// A failure here is never fatal: the page still renders, the ids are still
// correct, and the buyer sees one fewer column rather than an error screen.
func (s *Service) attachProductNames(ctx context.Context, lines []*Line) {
	ids := make([]int64, 0, len(lines))
	seen := make(map[int64]bool, len(lines))
	for _, l := range lines {
		if !l.Matched() || seen[*l.MatchedProductID] {
			continue
		}
		seen[*l.MatchedProductID] = true
		ids = append(ids, *l.MatchedProductID)
	}
	if len(ids) == 0 {
		return
	}
	names, err := s.repo.ProductNames(ctx, ids)
	if err != nil {
		s.log.WarnContext(ctx, "could not resolve matched product names", "error", err)
		return
	}
	for _, l := range lines {
		if l.Matched() {
			l.MatchedProductName = names[*l.MatchedProductID]
		}
	}
}

// Candidates returns every vendor considered for a line, rejected ones included.
func (s *Service) Candidates(ctx context.Context, orgID, lineID int64) ([]Candidate, error) {
	return s.repo.ListCandidates(ctx, orgID, lineID)
}

// SetQuantity applies a buyer's quantity edit.
func (s *Service) SetQuantity(ctx context.Context, orgID, lineID int64, qty float64) error {
	if qty < 0 {
		return apperr.Validation("smartorder.negative_quantity",
			"quantity cannot be negative", nil)
	}
	if sel, err := s.repo.GetSelection(ctx, orgID, lineID); err == nil && sel != nil {
		if cand, err := s.repo.GetCandidate(ctx, orgID, sel.CandidateID); err == nil && cand != nil {
			if cand.StockQty > 0 && qty > float64(cand.StockQty) {
				return apperr.Validation("smartorder.insufficient_stock",
					fmt.Sprintf("الكمية المطلوبة (%g) تتجاوز المخزون المتاح لدى المورد (%d)", qty, cand.StockQty), nil)
			}
		}
	}
	if err := s.repo.UpdateLineQuantity(ctx, orgID, lineID, qty); err != nil {
		return err
	}
	// A line dropped to zero is no longer ordered, so its supplier choice goes
	// with it — leaving a dangling selection would show the buyer a supplier for
	// something they are not buying.
	if qty == 0 {
		return s.repo.DeleteSelection(ctx, orgID, lineID)
	}
	if sel, err := s.repo.GetSelection(ctx, orgID, lineID); err == nil && sel != nil {
		if cand, err := s.repo.GetCandidate(ctx, orgID, sel.CandidateID); err == nil && cand != nil {
			if net, err := LineNet(cand.NetUnitPrice, qty); err == nil {
				sel.LineNet = net
				_ = s.repo.UpsertSelections(ctx, []*Selection{sel})
			}
		}
	}
	return nil
}

// SetDefaultQuantity applies a default quantity to all (or selected) lines in a run.
func (s *Service) SetDefaultQuantity(ctx context.Context, orgID, runID int64, lineIDs []int64, qty float64) error {
	if qty <= 0 {
		return apperr.Validation("smartorder.invalid_quantity",
			"quantity must be greater than zero", nil)
	}
	return s.repo.SetDefaultQuantity(ctx, orgID, runID, lineIDs, qty)
}

// ChooseSupplier overrides the automatic selection for one line.
func (s *Service) ChooseSupplier(ctx context.Context, orgID, lineID, candidateID int64) error {
	candidate, err := s.repo.GetCandidate(ctx, orgID, candidateID)
	if err != nil {
		return err
	}
	if candidate.LineID != lineID {
		return apperr.Validation("smartorder.candidate_mismatch",
			"that supplier is not an option for this line", nil)
	}
	if !candidate.Eligible {
		return apperr.Validation("smartorder.candidate_ineligible",
			"that supplier cannot deliver this line", nil)
	}
	line, err := s.repo.GetLine(ctx, orgID, lineID)
	if err != nil {
		return err
	}
	net, err := LineNet(candidate.NetUnitPrice, line.EffectiveQty)
	if err != nil {
		return err
	}
	return s.repo.UpsertSelections(ctx, []*Selection{{
		LineID:         lineID,
		OrganizationID: orgID,
		CandidateID:    candidateID,
		DecidedBy:      DecidedUser,
		UserOverridden: true,
		LineNet:        net,
	}})
}

// RemoveLine drops a line from the order.
func (s *Service) RemoveLine(ctx context.Context, orgID, lineID int64) error {
	line, err := s.repo.GetLine(ctx, orgID, lineID)
	if err != nil {
		return err
	}
	line.Outcome = OutcomeRemoved
	if err := s.repo.UpdateLines(ctx, []*Line{line}); err != nil {
		return err
	}
	return s.repo.DeleteSelection(ctx, orgID, lineID)
}

// MarkStale records that the configuration moved after results existed.
func (s *Service) MarkStale(ctx context.Context, run *Run) error {
	if err := run.MarkStale(); err != nil {
		return err
	}
	if run.Status != StatusStale {
		return nil
	}
	return s.repo.UpdateRunStatus(ctx, run.ID, run.Status, run.CurrentStep, "")
}

// Recalculate recomputes the order total and budget status after review edits.
func (s *Service) Recalculate(ctx context.Context, run *Run, cfg *Config) (money.Amount, error) {
	lines, _, err := s.repo.ListLines(ctx, run.ID, LineFilter{
		Outcome: string(OutcomeOrdered),
		All:     true,
	})
	if err != nil {
		return money.Amount{}, err
	}
	// Every selection of the run in one query. Asking per line made this scale
	// with the buyer's file: a nine-hundred-line run recalculated after each
	// quantity edit is nine hundred round trips per keystroke.
	selections, err := s.repo.ListSelectionsByRun(ctx, run.OrganizationID, run.ID)
	if err != nil {
		return money.Amount{}, err
	}

	total := money.Amount{}
	for _, l := range lines {
		sel, ok := selections[l.ID]
		if !ok {
			continue // a line without a selection contributes nothing
		}
		if total, err = total.Add(sel.LineNet); err != nil {
			return money.Amount{}, err
		}
	}

	run.EstimatedTotal = total
	exceeded, overage, hasBudget := cfg.BudgetStatus(total)
	if hasBudget {
		run.BudgetExceeded = &exceeded
		if exceeded {
			run.BudgetOverage = &overage
		} else {
			run.BudgetOverage = nil
		}
	}
	return total, s.repo.UpdateRunStats(ctx, run)
}

// Config returns the snapshot a run executed under.
func (s *Service) Config(ctx context.Context, runID int64) (*Config, error) {
	return s.repo.GetConfig(ctx, runID)
}

// Mapping returns the confirmed column mapping for a run.
func (s *Service) Mapping(ctx context.Context, runID int64) (*Mapping, error) {
	return s.repo.GetMapping(ctx, runID)
}

// Events returns progress records after a cursor, for the live progress stream.
func (s *Service) Events(ctx context.Context, runID, afterID int64) ([]*Event, error) {
	return s.repo.ListEvents(ctx, runID, afterID)
}

// Profile returns the buyer's remembered configuration.
func (s *Service) Profile(ctx context.Context, orgID int64) (*Profile, error) {
	return s.repo.GetProfile(ctx, orgID)
}

// StageLines persists the rows read from the buyer's file.
func (s *Service) StageLines(ctx context.Context, lines []*Line) error {
	return s.repo.InsertLines(ctx, lines)
}

// Selection returns the chosen supplier for one line.
func (s *Service) Selection(ctx context.Context, orgID, lineID int64) (*Selection, error) {
	return s.repo.GetSelection(ctx, orgID, lineID)
}

// SaveFile stores the uploaded workbook until the mapping is confirmed.
func (s *Service) SaveFile(ctx context.Context, runID, orgID int64, filename string, content []byte) error {
	return s.repo.SaveRunFile(ctx, runID, orgID, filename, content)
}

// File returns the stored workbook.
func (s *Service) File(ctx context.Context, runID, orgID int64) ([]byte, string, error) {
	return s.repo.GetRunFile(ctx, runID, orgID)
}

// DropFile removes the workbook once its rows are staged.
func (s *Service) DropFile(ctx context.Context, runID, orgID int64) error {
	return s.repo.DeleteRunFile(ctx, runID, orgID)
}
