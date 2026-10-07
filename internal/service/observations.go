package service

import (
	"time"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

func buildObservation(runID string, input ObservationInput, now time.Time) domain.GravityObservation {
	return domain.GravityObservation{
		RunID:        runID,
		Sequence:     input.Sequence,
		Gravity:      input.Gravity,
		TemperatureC: input.TemperatureC,
		ObservedAt:   input.ObservedAt.UTC(),
		Note:         input.Note,
		ReceivedAt:   now,
	}
}

func applyObservation(
	snapshot *domain.Snapshot,
	id string,
	observation domain.GravityObservation,
) (domain.FermentationRun, error) {
	run, ok := snapshot.Runs[id]
	if !ok {
		return domain.FermentationRun{},
			domain.NewError(domain.CodeRunNotFound, "run was not found")
	}
	if err := run.AddObservation(observation); err != nil {
		return domain.FermentationRun{}, err
	}
	return run, nil
}

func (s *Service) AppendObservation(
	id string,
	input ObservationInput,
) (domain.FermentationRun, error) {
	var updated domain.FermentationRun
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		observation := buildObservation(id, input, s.clock.Now())
		run, err := applyObservation(snapshot, id, observation)
		if err != nil {
			return err
		}
		snapshot.Runs[id] = run.Clone()
		updated = run.Clone()
		return nil
	})
	if err != nil {
		return domain.FermentationRun{}, err
	}
	return updated, nil
}

// PreviewAppendObservation appends the observation to a detached copy of the
// run and projects the result. It never stores the receipt.
func (s *Service) PreviewAppendObservation(id string, input ObservationInput) ChangePreview {
	return s.previewView(func(snapshot *domain.Snapshot) ChangePreview {
		current := runCurrentVersion(snapshot, id)
		observation := buildObservation(id, input, s.clock.Now())
		run, err := applyObservation(snapshot, id, observation)
		if err != nil {
			return ChangePreview{
				CurrentVersion: current,
				Conflict:       conflictFromError(err),
			}
		}
		return changePreview(current, nil, run.Clone(), nil)
	})
}

func applyConditioning(
	snapshot *domain.Snapshot,
	id string,
	now time.Time,
) (domain.FermentationRun, error) {
	run, ok := snapshot.Runs[id]
	if !ok {
		return domain.FermentationRun{},
			domain.NewError(domain.CodeRunNotFound, "run was not found")
	}
	if err := run.BeginConditioning(now); err != nil {
		return domain.FermentationRun{}, err
	}
	return run, nil
}

func (s *Service) BeginConditioning(
	id string,
	input VersionInput,
) (domain.FermentationRun, error) {
	var updated domain.FermentationRun
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		current, err := snapshotRunVersion(snapshot, id)
		if err != nil {
			return err
		}
		if err := checkExpected(current, input.ExpectedVersion); err != nil {
			return err
		}
		run, err := applyConditioning(snapshot, id, s.clock.Now())
		if err != nil {
			return err
		}
		snapshot.Runs[id] = run.Clone()
		updated = run.Clone()
		return nil
	})
	if err != nil {
		return domain.FermentationRun{}, err
	}
	return updated, nil
}

// PreviewBeginConditioning evaluates the conditioning guard without advancing
// the run.
func (s *Service) PreviewBeginConditioning(id string, input VersionInput) ChangePreview {
	return s.previewView(func(snapshot *domain.Snapshot) ChangePreview {
		current := runCurrentVersion(snapshot, id)
		if current == nil {
			return ChangePreview{
				ExpectedVersion: input.ExpectedVersion,
				Conflict: conflictFromError(
					domain.NewError(domain.CodeRunNotFound, "run was not found")),
			}
		}
		if err := checkExpected(*current, input.ExpectedVersion); err != nil {
			return ChangePreview{
				CurrentVersion:  current,
				ExpectedVersion: input.ExpectedVersion,
				Conflict:        conflictFromError(err),
			}
		}
		run, err := applyConditioning(snapshot, id, s.clock.Now())
		return changePreview(current, input.ExpectedVersion, run.Clone(), err)
	})
}

func applyRunCompletion(
	snapshot *domain.Snapshot,
	id string,
	now time.Time,
) (domain.FermentationRun, domain.Vessel, error) {
	run, ok := snapshot.Runs[id]
	if !ok {
		return domain.FermentationRun{}, domain.Vessel{},
			domain.NewError(domain.CodeRunNotFound, "run was not found")
	}
	formula, ok := snapshot.Formulas[run.FormulaID]
	if !ok {
		return domain.FermentationRun{}, domain.Vessel{},
			domain.NewError(domain.CodeFormulaNotFound, "attached formula was not found")
	}
	if err := run.Complete(formula.TargetOriginalGravity, now); err != nil {
		return domain.FermentationRun{}, domain.Vessel{}, err
	}
	if run.VesselID == nil {
		return domain.FermentationRun{}, domain.Vessel{},
			domain.NewError(domain.CodeRunState, "run has no vessel")
	}
	vessel, ok := snapshot.Vessels[*run.VesselID]
	if !ok {
		return domain.FermentationRun{}, domain.Vessel{},
			domain.NewError(domain.CodeVesselNotFound, "vessel was not found")
	}
	if err := vessel.StartCleaning(run.ID, now); err != nil {
		return domain.FermentationRun{}, domain.Vessel{}, err
	}
	return run, vessel, nil
}

func (s *Service) CompleteRun(
	id string,
	input VersionInput,
) (domain.FermentationRun, domain.Vessel, error) {
	var updatedRun domain.FermentationRun
	var updatedVessel domain.Vessel
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		current, err := snapshotRunVersion(snapshot, id)
		if err != nil {
			return err
		}
		if err := checkExpected(current, input.ExpectedVersion); err != nil {
			return err
		}
		run, vessel, err := applyRunCompletion(snapshot, id, s.clock.Now())
		if err != nil {
			return err
		}
		snapshot.Runs[id] = run.Clone()
		snapshot.Vessels[vessel.ID] = vessel
		updatedRun = run.Clone()
		updatedVessel = vessel
		return nil
	})
	if err != nil {
		return domain.FermentationRun{}, domain.Vessel{}, err
	}
	return updatedRun, updatedVessel, nil
}

// PreviewCompleteRun evaluates completion, including metric calculation and
// the vessel cleaning side effect, without committing either.
func (s *Service) PreviewCompleteRun(id string, input VersionInput) ChangePreview {
	return s.previewView(func(snapshot *domain.Snapshot) ChangePreview {
		current := runCurrentVersion(snapshot, id)
		if current == nil {
			return ChangePreview{
				ExpectedVersion: input.ExpectedVersion,
				Conflict: conflictFromError(
					domain.NewError(domain.CodeRunNotFound, "run was not found")),
			}
		}
		if err := checkExpected(*current, input.ExpectedVersion); err != nil {
			return ChangePreview{
				CurrentVersion:  current,
				ExpectedVersion: input.ExpectedVersion,
				Conflict:        conflictFromError(err),
			}
		}
		run, vessel, err := applyRunCompletion(snapshot, id, s.clock.Now())
		if err != nil {
			return changePreview(current, input.ExpectedVersion, nil, err)
		}
		return changePreview(current, input.ExpectedVersion,
			RunVesselChange{Run: run.Clone(), Vessel: vessel}, nil)
	})
}
