package service

import (
	"sort"
	"time"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

// buildRunCandidate normalizes and validates the request fields that are
// independent of identifier and run-code assignment. When the client omits a
// code, the service assigns one at commit time; the stand-in keeps validation
// of the remaining fields on the same path as a real commit.
func buildRunCandidate(input CreateRunInput, now time.Time) (domain.FermentationRun, bool, error) {
	codeGenerated := false
	code := domain.NormalizeRunCode(input.Code)
	if code == "" {
		codeGenerated = true
		code = "RUN-PENDING"
	}
	run := domain.FermentationRun{
		Code:           code,
		FormulaID:      input.FormulaID,
		PlannedVolumeL: input.PlannedVolumeL,
		Notes:          input.Notes,
		State:          domain.RunPlanned,
		Observations:   []domain.GravityObservation{},
		Version:        1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := run.Validate(); err != nil {
		return domain.FermentationRun{}, codeGenerated, err
	}
	return run, codeGenerated, nil
}

func checkRunCreate(snapshot *domain.Snapshot, run *domain.FermentationRun, codeGenerated bool) error {
	formula, ok := snapshot.Formulas[run.FormulaID]
	if !ok {
		return domain.NewError(domain.CodeFormulaNotFound, "formula was not found")
	}
	if formula.State != domain.FormulaApproved {
		return domain.NewError(domain.CodeFormulaState,
			"run requires an approved formula")
	}
	if !codeGenerated {
		if _, exists := snapshot.RunIndex[run.Code]; exists {
			return domain.NewError(domain.CodeRunConflict,
				"a run with the same code already exists")
		}
	}
	return nil
}

func (s *Service) CreateRun(input CreateRunInput) (domain.FermentationRun, error) {
	now := s.clock.Now()
	run, codeGenerated, err := buildRunCandidate(input, now)
	if err != nil {
		return domain.FermentationRun{}, err
	}
	if codeGenerated {
		generated, err := generatedRunCode()
		if err != nil {
			return domain.FermentationRun{}, domain.NewError(domain.CodeStorage, err.Error())
		}
		run.Code = generated
	}
	id, err := generateEntityID("run")
	if err != nil {
		return domain.FermentationRun{}, domain.NewError(domain.CodeStorage, err.Error())
	}
	run.ID = id

	err = s.store.Execute(func(snapshot *domain.Snapshot) error {
		if err := checkRunCreate(snapshot, &run, false); err != nil {
			return err
		}
		snapshot.Runs[run.ID] = run.Clone()
		snapshot.RunIndex[run.Code] = run.ID
		return nil
	})
	if err != nil {
		return domain.FermentationRun{}, err
	}
	return run.Clone(), nil
}

// PreviewCreateRun evaluates run creation without assigning an identifier or
// persisting anything. When the code would be server-generated, the projected
// aggregate carries no code and the assignment says so.
func (s *Service) PreviewCreateRun(input CreateRunInput) ChangePreview {
	now := s.clock.Now()
	candidate, codeGenerated, err := buildRunCandidate(input, now)
	assignment := &PreviewAssignment{
		EntityIDAssigned: true,
		CodeAssigned:     codeGenerated,
		InitialVersion:   1,
	}
	return s.previewView(func(snapshot *domain.Snapshot) ChangePreview {
		preview := ChangePreview{Assignment: assignment}
		if err != nil {
			preview.Conflict = conflictFromError(err)
			return preview
		}
		if err := checkRunCreate(snapshot, &candidate, codeGenerated); err != nil {
			preview.Conflict = conflictFromError(err)
			return preview
		}
		projected := candidate.Clone()
		if codeGenerated {
			projected.Code = ""
		}
		preview.Result = PendingRun{Run: projected, CodeGenerated: codeGenerated}
		return preview
	})
}

func applyRunReservation(
	snapshot *domain.Snapshot,
	id, vesselID string,
	now time.Time,
) (domain.FermentationRun, domain.Vessel, error) {
	run, ok := snapshot.Runs[id]
	if !ok {
		return domain.FermentationRun{}, domain.Vessel{},
			domain.NewError(domain.CodeRunNotFound, "run was not found")
	}
	vessel, ok := snapshot.Vessels[vesselID]
	if !ok {
		return domain.FermentationRun{}, domain.Vessel{},
			domain.NewError(domain.CodeVesselNotFound, "vessel was not found")
	}
	if run.PlannedVolumeL > vessel.CapacityL {
		return domain.FermentationRun{}, domain.Vessel{},
			domain.FieldError(domain.CodeInvalidInput,
				"planned_volume_liters", "exceeds vessel capacity")
	}
	if err := vessel.Reserve(run.ID, now); err != nil {
		return domain.FermentationRun{}, domain.Vessel{}, err
	}
	if err := run.ReserveVessel(vessel.ID, now); err != nil {
		return domain.FermentationRun{}, domain.Vessel{}, err
	}
	return run, vessel, nil
}

func (s *Service) ReserveRun(
	id string,
	input ReserveRunInput,
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
		run, vessel, err := applyRunReservation(snapshot, id, input.VesselID, s.clock.Now())
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

// PreviewReserveRun evaluates the atomic vessel reservation against current
// state and projects both aggregates without committing.
func (s *Service) PreviewReserveRun(id string, input ReserveRunInput) ChangePreview {
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
		run, vessel, err := applyRunReservation(snapshot, id, input.VesselID, s.clock.Now())
		if err != nil {
			return changePreview(current, input.ExpectedVersion, nil, err)
		}
		return changePreview(current, input.ExpectedVersion,
			RunVesselChange{Run: run.Clone(), Vessel: vessel}, nil)
	})
}

func applyRunStart(
	snapshot *domain.Snapshot,
	id string,
	now time.Time,
) (domain.FermentationRun, domain.Vessel, error) {
	run, ok := snapshot.Runs[id]
	if !ok {
		return domain.FermentationRun{}, domain.Vessel{},
			domain.NewError(domain.CodeRunNotFound, "run was not found")
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
	if err := vessel.StartRun(run.ID, now); err != nil {
		return domain.FermentationRun{}, domain.Vessel{}, err
	}
	if err := run.StartFermentation(now); err != nil {
		return domain.FermentationRun{}, domain.Vessel{}, err
	}
	return run, vessel, nil
}

func snapshotRunVersion(snapshot *domain.Snapshot, id string) (int64, error) {
	run, ok := snapshot.Runs[id]
	if !ok {
		return 0, domain.NewError(domain.CodeRunNotFound, "run was not found")
	}
	return run.Version, nil
}

func (s *Service) StartRun(
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
		run, vessel, err := applyRunStart(snapshot, id, s.clock.Now())
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

// PreviewStartRun evaluates the fermentation start without committing it.
func (s *Service) PreviewStartRun(id string, input VersionInput) ChangePreview {
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
		run, vessel, err := applyRunStart(snapshot, id, s.clock.Now())
		if err != nil {
			return changePreview(current, input.ExpectedVersion, nil, err)
		}
		return changePreview(current, input.ExpectedVersion,
			RunVesselChange{Run: run.Clone(), Vessel: vessel}, nil)
	})
}

func applyRunAbort(
	snapshot *domain.Snapshot,
	id string,
	now time.Time,
) (domain.FermentationRun, domain.Vessel, error) {
	run, ok := snapshot.Runs[id]
	if !ok {
		return domain.FermentationRun{}, domain.Vessel{},
			domain.NewError(domain.CodeRunNotFound, "run was not found")
	}
	hadStarted := run.HadStarted()
	if err := run.Abort(now); err != nil {
		return domain.FermentationRun{}, domain.Vessel{}, err
	}
	var vessel domain.Vessel
	if run.VesselID != nil {
		vessel, ok = snapshot.Vessels[*run.VesselID]
		if !ok {
			return domain.FermentationRun{}, domain.Vessel{},
				domain.NewError(domain.CodeVesselNotFound, "vessel was not found")
		}
		if hadStarted {
			if err := vessel.StartCleaning(run.ID, now); err != nil {
				return domain.FermentationRun{}, domain.Vessel{}, err
			}
		} else if err := vessel.ReleaseReservation(run.ID, now); err != nil {
			return domain.FermentationRun{}, domain.Vessel{}, err
		}
	}
	return run, vessel, nil
}

func (s *Service) AbortRun(
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
		run, vessel, err := applyRunAbort(snapshot, id, s.clock.Now())
		if err != nil {
			return err
		}
		if run.VesselID != nil {
			snapshot.Vessels[vessel.ID] = vessel
		}
		snapshot.Runs[id] = run.Clone()
		updatedRun = run.Clone()
		updatedVessel = vessel
		return nil
	})
	if err != nil {
		return domain.FermentationRun{}, domain.Vessel{}, err
	}
	return updatedRun, updatedVessel, nil
}

// PreviewAbortRun evaluates the abort transition and its vessel side effect
// without committing either.
func (s *Service) PreviewAbortRun(id string, input VersionInput) ChangePreview {
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
		run, vessel, err := applyRunAbort(snapshot, id, s.clock.Now())
		if err != nil {
			return changePreview(current, input.ExpectedVersion, nil, err)
		}
		return changePreview(current, input.ExpectedVersion,
			RunVesselChange{Run: run.Clone(), Vessel: vessel}, nil)
	})
}

func (s *Service) GetRun(id string) (domain.FermentationRun, error) {
	var found domain.FermentationRun
	err := s.store.View(func(snapshot domain.Snapshot) error {
		run, ok := snapshot.Runs[id]
		if !ok {
			return domain.NewError(domain.CodeRunNotFound, "run was not found")
		}
		found = run.Clone()
		return nil
	})
	if err != nil {
		return domain.FermentationRun{}, err
	}
	return found, nil
}

func (s *Service) ListRuns() ([]domain.FermentationRun, error) {
	var runs []domain.FermentationRun
	err := s.store.View(func(snapshot domain.Snapshot) error {
		runs = make([]domain.FermentationRun, 0, len(snapshot.Runs))
		for _, run := range snapshot.Runs {
			runs = append(runs, run.Clone())
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].Code == runs[j].Code {
			return runs[i].ID < runs[j].ID
		}
		return runs[i].Code < runs[j].Code
	})
	return runs, nil
}
