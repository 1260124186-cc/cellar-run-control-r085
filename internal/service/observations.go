package service

import "github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"

func (s *Service) AppendObservation(
	id string,
	input ObservationInput,
) (domain.FermentationRun, error) {
	var updated domain.FermentationRun
	if err := s.store.Execute(s.appendObservation(id, input, &updated)); err != nil {
		return domain.FermentationRun{}, err
	}
	return updated, nil
}

func (s *Service) PreflightAppendObservation(
	id string,
	input ObservationInput,
) (PreflightReport, domain.FermentationRun) {
	var updated domain.FermentationRun
	report := s.preflight(s.appendObservation(id, input, &updated), runVersionOf(id))
	return report, updated
}

func (s *Service) appendObservation(
	id string,
	input ObservationInput,
	updated *domain.FermentationRun,
) func(*domain.Snapshot) error {
	return func(snapshot *domain.Snapshot) error {
		run, ok := snapshot.Runs[id]
		if !ok {
			return domain.NewError(domain.CodeRunNotFound, "run was not found")
		}
		now := s.clock.Now()
		observation := domain.GravityObservation{
			RunID:        run.ID,
			Sequence:     input.Sequence,
			Gravity:      input.Gravity,
			TemperatureC: input.TemperatureC,
			ObservedAt:   input.ObservedAt.UTC(),
			Note:         input.Note,
			ReceivedAt:   now,
		}
		if err := run.AddObservation(observation); err != nil {
			return err
		}
		snapshot.Runs[id] = run.Clone()
		*updated = run.Clone()
		return nil
	}
}

func (s *Service) BeginConditioning(
	id string,
	input VersionInput,
) (domain.FermentationRun, error) {
	var updated domain.FermentationRun
	if err := s.store.Execute(s.beginConditioning(id, input, &updated)); err != nil {
		return domain.FermentationRun{}, err
	}
	return updated, nil
}

func (s *Service) PreflightBeginConditioning(
	id string,
	input VersionInput,
) (PreflightReport, domain.FermentationRun) {
	var updated domain.FermentationRun
	report := s.preflight(s.beginConditioning(id, input, &updated), runVersionOf(id))
	return report, updated
}

func (s *Service) beginConditioning(
	id string,
	input VersionInput,
	updated *domain.FermentationRun,
) func(*domain.Snapshot) error {
	return func(snapshot *domain.Snapshot) error {
		run, ok := snapshot.Runs[id]
		if !ok {
			return domain.NewError(domain.CodeRunNotFound, "run was not found")
		}
		if err := s.checkExpected(run.Version, input.ExpectedVersion); err != nil {
			return err
		}
		if err := run.BeginConditioning(s.clock.Now()); err != nil {
			return err
		}
		snapshot.Runs[id] = run.Clone()
		*updated = run.Clone()
		return nil
	}
}

func (s *Service) CompleteRun(
	id string,
	input VersionInput,
) (domain.FermentationRun, domain.Vessel, error) {
	var updatedRun domain.FermentationRun
	var updatedVessel domain.Vessel
	err := s.store.Execute(s.completeRun(id, input, &updatedRun, &updatedVessel))
	if err != nil {
		return domain.FermentationRun{}, domain.Vessel{}, err
	}
	return updatedRun, updatedVessel, nil
}

func (s *Service) PreflightCompleteRun(
	id string,
	input VersionInput,
) (PreflightReport, domain.FermentationRun, domain.Vessel) {
	var updatedRun domain.FermentationRun
	var updatedVessel domain.Vessel
	report := s.preflight(s.completeRun(id, input, &updatedRun, &updatedVessel),
		runVersionOf(id))
	return report, updatedRun, updatedVessel
}

func (s *Service) completeRun(
	id string,
	input VersionInput,
	updatedRun *domain.FermentationRun,
	updatedVessel *domain.Vessel,
) func(*domain.Snapshot) error {
	return func(snapshot *domain.Snapshot) error {
		run, ok := snapshot.Runs[id]
		if !ok {
			return domain.NewError(domain.CodeRunNotFound, "run was not found")
		}
		if err := s.checkExpected(run.Version, input.ExpectedVersion); err != nil {
			return err
		}
		formula, ok := snapshot.Formulas[run.FormulaID]
		if !ok {
			return domain.NewError(domain.CodeFormulaNotFound, "attached formula was not found")
		}
		if err := run.Complete(formula.TargetOriginalGravity, s.clock.Now()); err != nil {
			return err
		}
		if run.VesselID == nil {
			return domain.NewError(domain.CodeRunState, "run has no vessel")
		}
		vessel, ok := snapshot.Vessels[*run.VesselID]
		if !ok {
			return domain.NewError(domain.CodeVesselNotFound, "vessel was not found")
		}
		if err := vessel.StartCleaning(run.ID, s.clock.Now()); err != nil {
			return err
		}
		snapshot.Runs[id] = run.Clone()
		snapshot.Vessels[vessel.ID] = vessel
		*updatedRun = run.Clone()
		*updatedVessel = vessel
		return nil
	}
}
