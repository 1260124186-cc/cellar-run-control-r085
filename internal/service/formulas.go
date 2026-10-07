package service

import (
	"sort"
	"time"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

func (s *Service) CreateFormula(input CreateFormulaInput) (domain.Formula, error) {
	now := s.clock.Now()
	formula := buildFormulaCandidate(input, now)
	if err := formula.Validate(); err != nil {
		return domain.Formula{}, err
	}
	id, err := generateEntityID("formula")
	if err != nil {
		return domain.Formula{}, domain.NewError(domain.CodeStorage, err.Error())
	}
	formula.ID = id
	nameKey := domain.NameKey(formula.Name)

	err = s.store.Execute(func(snapshot *domain.Snapshot) error {
		if err := checkFormulaNameAvailable(snapshot, nameKey); err != nil {
			return err
		}
		snapshot.Formulas[formula.ID] = formula
		snapshot.NameIndex[nameKey] = formula.ID
		return nil
	})
	if err != nil {
		return domain.Formula{}, err
	}
	return formula.Clone(), nil
}

// PreviewCreateFormula evaluates a formula creation against current state
// without assigning an identifier or persisting anything.
func (s *Service) PreviewCreateFormula(input CreateFormulaInput) ChangePreview {
	now := s.clock.Now()
	candidate := buildFormulaCandidate(input, now)
	assignment := &PreviewAssignment{EntityIDAssigned: true, InitialVersion: 1}
	return s.previewView(func(snapshot *domain.Snapshot) ChangePreview {
		preview := ChangePreview{Assignment: assignment}
		nameKey := domain.NameKey(candidate.Name)
		if err := candidate.Validate(); err != nil {
			preview.Conflict = conflictFromError(err)
			return preview
		}
		if err := checkFormulaNameAvailable(snapshot, nameKey); err != nil {
			preview.Conflict = conflictFromError(err)
			return preview
		}
		preview.Result = PendingFormula{Formula: candidate.Clone()}
		return preview
	})
}

func buildFormulaCandidate(input CreateFormulaInput, now time.Time) domain.Formula {
	formula := domain.Formula{
		Name:                  input.Name,
		Style:                 input.Style,
		TargetOriginalGravity: input.TargetOriginalGravity,
		TargetFinalGravity:    input.TargetFinalGravity,
		MaxFermentationTempC:  input.MaxFermentationTempC,
		MinimumDays:           input.MinimumDays,
		MaximumDays:           input.MaximumDays,
		Steps:                 []domain.FormulaStep{},
		State:                 domain.FormulaDraft,
		Version:               1,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	formula.Normalize()
	return formula
}

func checkFormulaNameAvailable(snapshot *domain.Snapshot, nameKey string) error {
	if _, exists := snapshot.NameIndex[nameKey]; exists {
		return domain.NewError(domain.CodeFormulaNameConflict,
			"a formula with the same normalized name already exists")
	}
	return nil
}

func applyFormulaSteps(
	snapshot *domain.Snapshot,
	id string,
	steps []domain.FormulaStep,
	now time.Time,
) (domain.Formula, error) {
	formula, ok := snapshot.Formulas[id]
	if !ok {
		return domain.Formula{}, domain.NewError(domain.CodeFormulaNotFound, "formula was not found")
	}
	if err := formula.SetSteps(steps); err != nil {
		return domain.Formula{}, err
	}
	formula.UpdatedAt = now
	return formula, nil
}

func (s *Service) SetFormulaSteps(
	id string,
	input SetFormulaStepsInput,
) (domain.Formula, error) {
	var updated domain.Formula
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		current, err := snapshotFormulaVersion(snapshot, id)
		if err != nil {
			return err
		}
		if err := checkExpected(current, input.ExpectedVersion); err != nil {
			return err
		}
		formula, err := applyFormulaSteps(snapshot, id, input.Steps, s.clock.Now())
		if err != nil {
			return err
		}
		snapshot.Formulas[id] = formula
		updated = formula.Clone()
		return nil
	})
	if err != nil {
		return domain.Formula{}, err
	}
	return updated, nil
}

// PreviewSetFormulaSteps reports whether replacing the step list would
// conflict and what the formula would look like after the revision.
func (s *Service) PreviewSetFormulaSteps(id string, input SetFormulaStepsInput) ChangePreview {
	return s.previewView(func(snapshot *domain.Snapshot) ChangePreview {
		current := formulaCurrentVersion(snapshot, id)
		if current == nil {
			return ChangePreview{
				ExpectedVersion: input.ExpectedVersion,
				Conflict: conflictFromError(
					domain.NewError(domain.CodeFormulaNotFound, "formula was not found")),
			}
		}
		if err := checkExpected(*current, input.ExpectedVersion); err != nil {
			return ChangePreview{
				CurrentVersion:  current,
				ExpectedVersion: input.ExpectedVersion,
				Conflict:        conflictFromError(err),
			}
		}
		formula, err := applyFormulaSteps(snapshot, id, input.Steps, s.clock.Now())
		return changePreview(current, input.ExpectedVersion, formula.Clone(), err)
	})
}

func snapshotFormulaVersion(snapshot *domain.Snapshot, id string) (int64, error) {
	formula, ok := snapshot.Formulas[id]
	if !ok {
		return 0, domain.NewError(domain.CodeFormulaNotFound, "formula was not found")
	}
	return formula.Version, nil
}

func applyFormulaApproval(snapshot *domain.Snapshot, id string, now time.Time) (domain.Formula, error) {
	formula, ok := snapshot.Formulas[id]
	if !ok {
		return domain.Formula{}, domain.NewError(domain.CodeFormulaNotFound, "formula was not found")
	}
	if err := formula.Approve(now); err != nil {
		return domain.Formula{}, err
	}
	return formula, nil
}

func (s *Service) ApproveFormula(id string, input VersionInput) (domain.Formula, error) {
	var updated domain.Formula
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		current, err := snapshotFormulaVersion(snapshot, id)
		if err != nil {
			return err
		}
		if err := checkExpected(current, input.ExpectedVersion); err != nil {
			return err
		}
		formula, err := applyFormulaApproval(snapshot, id, s.clock.Now())
		if err != nil {
			return err
		}
		snapshot.Formulas[id] = formula
		updated = formula.Clone()
		return nil
	})
	if err != nil {
		return domain.Formula{}, err
	}
	return updated, nil
}

// PreviewApproveFormula evaluates an approval transition without committing it.
func (s *Service) PreviewApproveFormula(id string, input VersionInput) ChangePreview {
	return s.previewView(func(snapshot *domain.Snapshot) ChangePreview {
		current := formulaCurrentVersion(snapshot, id)
		if current == nil {
			return ChangePreview{
				ExpectedVersion: input.ExpectedVersion,
				Conflict: conflictFromError(
					domain.NewError(domain.CodeFormulaNotFound, "formula was not found")),
			}
		}
		if err := checkExpected(*current, input.ExpectedVersion); err != nil {
			return ChangePreview{
				CurrentVersion:  current,
				ExpectedVersion: input.ExpectedVersion,
				Conflict:        conflictFromError(err),
			}
		}
		formula, err := applyFormulaApproval(snapshot, id, s.clock.Now())
		return changePreview(current, input.ExpectedVersion, formula.Clone(), err)
	})
}

func applyFormulaRetirement(snapshot *domain.Snapshot, id string, now time.Time) (domain.Formula, error) {
	formula, ok := snapshot.Formulas[id]
	if !ok {
		return domain.Formula{}, domain.NewError(domain.CodeFormulaNotFound, "formula was not found")
	}
	for _, run := range snapshot.Runs {
		if run.FormulaID == id && run.State != domain.RunCompleted &&
			run.State != domain.RunAborted {
			return domain.Formula{}, domain.NewError(domain.CodeFormulaInUse,
				"formula is attached to a run that is still open")
		}
	}
	if err := formula.Retire(now); err != nil {
		return domain.Formula{}, err
	}
	return formula, nil
}

func (s *Service) RetireFormula(id string, input VersionInput) (domain.Formula, error) {
	var updated domain.Formula
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		current, err := snapshotFormulaVersion(snapshot, id)
		if err != nil {
			return err
		}
		if err := checkExpected(current, input.ExpectedVersion); err != nil {
			return err
		}
		formula, err := applyFormulaRetirement(snapshot, id, s.clock.Now())
		if err != nil {
			return err
		}
		snapshot.Formulas[id] = formula
		updated = formula.Clone()
		return nil
	})
	if err != nil {
		return domain.Formula{}, err
	}
	return updated, nil
}

// PreviewRetireFormula evaluates a retirement transition, including the
// open-run guard, without committing it.
func (s *Service) PreviewRetireFormula(id string, input VersionInput) ChangePreview {
	return s.previewView(func(snapshot *domain.Snapshot) ChangePreview {
		current := formulaCurrentVersion(snapshot, id)
		if current == nil {
			return ChangePreview{
				ExpectedVersion: input.ExpectedVersion,
				Conflict: conflictFromError(
					domain.NewError(domain.CodeFormulaNotFound, "formula was not found")),
			}
		}
		if err := checkExpected(*current, input.ExpectedVersion); err != nil {
			return ChangePreview{
				CurrentVersion:  current,
				ExpectedVersion: input.ExpectedVersion,
				Conflict:        conflictFromError(err),
			}
		}
		formula, err := applyFormulaRetirement(snapshot, id, s.clock.Now())
		return changePreview(current, input.ExpectedVersion, formula.Clone(), err)
	})
}

func (s *Service) GetFormula(id string) (domain.Formula, error) {
	var found domain.Formula
	err := s.store.View(func(snapshot domain.Snapshot) error {
		formula, ok := snapshot.Formulas[id]
		if !ok {
			return domain.NewError(domain.CodeFormulaNotFound, "formula was not found")
		}
		found = formula.Clone()
		return nil
	})
	if err != nil {
		return domain.Formula{}, err
	}
	return found, nil
}

func (s *Service) ListFormulas() ([]domain.Formula, error) {
	var formulas []domain.Formula
	err := s.store.View(func(snapshot domain.Snapshot) error {
		formulas = make([]domain.Formula, 0, len(snapshot.Formulas))
		for _, formula := range snapshot.Formulas {
			formulas = append(formulas, formula.Clone())
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(formulas, func(i, j int) bool {
		if formulas[i].Name == formulas[j].Name {
			return formulas[i].ID < formulas[j].ID
		}
		return formulas[i].Name < formulas[j].Name
	})
	return formulas, nil
}

func timestampPointer(value time.Time) *time.Time {
	copy := value.UTC()
	return &copy
}
