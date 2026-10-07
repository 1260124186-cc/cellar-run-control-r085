package service

import (
	"sort"
	"time"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

func (s *Service) CreateFormula(input CreateFormulaInput) (domain.Formula, error) {
	var created domain.Formula
	if err := s.store.Execute(s.createFormula(input, &created)); err != nil {
		return domain.Formula{}, err
	}
	return created, nil
}

func (s *Service) PreflightCreateFormula(
	input CreateFormulaInput,
) (PreflightReport, domain.Formula) {
	var created domain.Formula
	report := s.preflight(s.createFormula(input, &created), noCurrentVersion)
	return report, created
}

func (s *Service) createFormula(
	input CreateFormulaInput,
	created *domain.Formula,
) func(*domain.Snapshot) error {
	return func(snapshot *domain.Snapshot) error {
		now := s.clock.Now()
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
		if err := formula.Validate(); err != nil {
			return err
		}
		id, err := generateEntityID("formula")
		if err != nil {
			return domain.NewError(domain.CodeStorage, err.Error())
		}
		formula.ID = id
		nameKey := domain.NameKey(formula.Name)
		if _, exists := snapshot.NameIndex[nameKey]; exists {
			return domain.NewError(domain.CodeFormulaNameConflict,
				"a formula with the same normalized name already exists")
		}
		snapshot.Formulas[formula.ID] = formula
		snapshot.NameIndex[nameKey] = formula.ID
		*created = formula.Clone()
		return nil
	}
}

func (s *Service) SetFormulaSteps(
	id string,
	input SetFormulaStepsInput,
) (domain.Formula, error) {
	var updated domain.Formula
	if err := s.store.Execute(s.setFormulaSteps(id, input, &updated)); err != nil {
		return domain.Formula{}, err
	}
	return updated, nil
}

func (s *Service) PreflightSetFormulaSteps(
	id string,
	input SetFormulaStepsInput,
) (PreflightReport, domain.Formula) {
	var updated domain.Formula
	report := s.preflight(s.setFormulaSteps(id, input, &updated), formulaVersionOf(id))
	return report, updated
}

func (s *Service) setFormulaSteps(
	id string,
	input SetFormulaStepsInput,
	updated *domain.Formula,
) func(*domain.Snapshot) error {
	return func(snapshot *domain.Snapshot) error {
		formula, ok := snapshot.Formulas[id]
		if !ok {
			return domain.NewError(domain.CodeFormulaNotFound, "formula was not found")
		}
		if err := s.checkExpected(formula.Version, input.ExpectedVersion); err != nil {
			return err
		}
		if err := formula.SetSteps(input.Steps); err != nil {
			return err
		}
		formula.UpdatedAt = s.clock.Now()
		snapshot.Formulas[id] = formula
		*updated = formula.Clone()
		return nil
	}
}

func (s *Service) ApproveFormula(id string, input VersionInput) (domain.Formula, error) {
	var updated domain.Formula
	if err := s.store.Execute(s.approveFormula(id, input, &updated)); err != nil {
		return domain.Formula{}, err
	}
	return updated, nil
}

func (s *Service) PreflightApproveFormula(
	id string,
	input VersionInput,
) (PreflightReport, domain.Formula) {
	var updated domain.Formula
	report := s.preflight(s.approveFormula(id, input, &updated), formulaVersionOf(id))
	return report, updated
}

func (s *Service) approveFormula(
	id string,
	input VersionInput,
	updated *domain.Formula,
) func(*domain.Snapshot) error {
	return func(snapshot *domain.Snapshot) error {
		formula, ok := snapshot.Formulas[id]
		if !ok {
			return domain.NewError(domain.CodeFormulaNotFound, "formula was not found")
		}
		if err := s.checkExpected(formula.Version, input.ExpectedVersion); err != nil {
			return err
		}
		if err := formula.Approve(s.clock.Now()); err != nil {
			return err
		}
		snapshot.Formulas[id] = formula
		*updated = formula.Clone()
		return nil
	}
}

func (s *Service) RetireFormula(id string, input VersionInput) (domain.Formula, error) {
	var updated domain.Formula
	if err := s.store.Execute(s.retireFormula(id, input, &updated)); err != nil {
		return domain.Formula{}, err
	}
	return updated, nil
}

func (s *Service) PreflightRetireFormula(
	id string,
	input VersionInput,
) (PreflightReport, domain.Formula) {
	var updated domain.Formula
	report := s.preflight(s.retireFormula(id, input, &updated), formulaVersionOf(id))
	return report, updated
}

func (s *Service) retireFormula(
	id string,
	input VersionInput,
	updated *domain.Formula,
) func(*domain.Snapshot) error {
	return func(snapshot *domain.Snapshot) error {
		formula, ok := snapshot.Formulas[id]
		if !ok {
			return domain.NewError(domain.CodeFormulaNotFound, "formula was not found")
		}
		if err := s.checkExpected(formula.Version, input.ExpectedVersion); err != nil {
			return err
		}
		for _, run := range snapshot.Runs {
			if run.FormulaID == id && run.State != domain.RunCompleted &&
				run.State != domain.RunAborted {
				return domain.NewError(domain.CodeFormulaInUse,
					"formula is attached to a run that is still open")
			}
		}
		if err := formula.Retire(s.clock.Now()); err != nil {
			return err
		}
		snapshot.Formulas[id] = formula
		*updated = formula.Clone()
		return nil
	}
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
