package service

import (
	"sort"
	"time"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

func (s *Service) CreateVessel(input CreateVesselInput) (domain.Vessel, error) {
	now := s.clock.Now()
	vessel := buildVesselCandidate(input, now)
	if err := vessel.Validate(); err != nil {
		return domain.Vessel{}, err
	}
	id, err := generateEntityID("vessel")
	if err != nil {
		return domain.Vessel{}, domain.NewError(domain.CodeStorage, err.Error())
	}
	vessel.ID = id

	err = s.store.Execute(func(snapshot *domain.Snapshot) error {
		if err := checkVesselCodeAvailable(snapshot, vessel.Code); err != nil {
			return err
		}
		snapshot.Vessels[vessel.ID] = vessel
		snapshot.VesselIndex[vessel.Code] = vessel.ID
		return nil
	})
	if err != nil {
		return domain.Vessel{}, err
	}
	return vessel, nil
}

// PreviewCreateVessel evaluates a vessel creation without assigning an
// identifier or persisting anything.
func (s *Service) PreviewCreateVessel(input CreateVesselInput) ChangePreview {
	now := s.clock.Now()
	candidate := buildVesselCandidate(input, now)
	assignment := &PreviewAssignment{EntityIDAssigned: true, InitialVersion: 1}
	return s.previewView(func(snapshot *domain.Snapshot) ChangePreview {
		preview := ChangePreview{Assignment: assignment}
		if err := candidate.Validate(); err != nil {
			preview.Conflict = conflictFromError(err)
			return preview
		}
		if err := checkVesselCodeAvailable(snapshot, candidate.Code); err != nil {
			preview.Conflict = conflictFromError(err)
			return preview
		}
		preview.Result = PendingVessel{Vessel: candidate}
		return preview
	})
}

func buildVesselCandidate(input CreateVesselInput, now time.Time) domain.Vessel {
	return domain.Vessel{
		Code:      domain.NormalizeVesselCode(input.Code),
		CapacityL: input.CapacityL,
		State:     domain.VesselAvailable,
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func checkVesselCodeAvailable(snapshot *domain.Snapshot, code string) error {
	if _, exists := snapshot.VesselIndex[code]; exists {
		return domain.NewError(domain.CodeVesselConflict,
			"a vessel with the same code already exists")
	}
	return nil
}

func (s *Service) GetVessel(id string) (domain.Vessel, error) {
	var found domain.Vessel
	err := s.store.View(func(snapshot domain.Snapshot) error {
		vessel, ok := snapshot.Vessels[id]
		if !ok {
			return domain.NewError(domain.CodeVesselNotFound, "vessel was not found")
		}
		found = vessel
		return nil
	})
	if err != nil {
		return domain.Vessel{}, err
	}
	return found, nil
}

func (s *Service) ListVessels() ([]domain.Vessel, error) {
	var vessels []domain.Vessel
	err := s.store.View(func(snapshot domain.Snapshot) error {
		vessels = make([]domain.Vessel, 0, len(snapshot.Vessels))
		for _, vessel := range snapshot.Vessels {
			vessels = append(vessels, vessel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(vessels, func(i, j int) bool {
		if vessels[i].Code == vessels[j].Code {
			return vessels[i].ID < vessels[j].ID
		}
		return vessels[i].Code < vessels[j].Code
	})
	return vessels, nil
}

func applyFinishCleaning(
	snapshot *domain.Snapshot,
	id string,
	now time.Time,
) (domain.Vessel, error) {
	vessel, ok := snapshot.Vessels[id]
	if !ok {
		return domain.Vessel{}, domain.NewError(domain.CodeVesselNotFound, "vessel was not found")
	}
	if vessel.ActiveRunID == nil {
		return domain.Vessel{}, domain.NewError(domain.CodeVesselState,
			"cleaning vessel has no associated run")
	}
	run, ok := snapshot.Runs[*vessel.ActiveRunID]
	if !ok {
		return domain.Vessel{}, domain.NewError(domain.CodeRunNotFound,
			"associated run was not found")
	}
	if run.State != domain.RunCompleted && run.State != domain.RunAborted {
		return domain.Vessel{}, domain.NewError(domain.CodeRunState,
			"associated run is still open")
	}
	if err := vessel.FinishCleaning(now); err != nil {
		return domain.Vessel{}, err
	}
	return vessel, nil
}

func (s *Service) FinishCleaning(
	id string,
	input FinishCleaningInput,
) (domain.Vessel, error) {
	var updated domain.Vessel
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		current, err := snapshotVesselVersion(snapshot, id)
		if err != nil {
			return err
		}
		if err := checkExpected(current, input.ExpectedVersion); err != nil {
			return err
		}
		vessel, err := applyFinishCleaning(snapshot, id, s.clock.Now())
		if err != nil {
			return err
		}
		snapshot.Vessels[id] = vessel
		updated = vessel
		return nil
	})
	if err != nil {
		return domain.Vessel{}, err
	}
	return updated, nil
}

// PreviewFinishCleaning evaluates the cleaning completion guards without
// releasing the vessel.
func (s *Service) PreviewFinishCleaning(id string, input FinishCleaningInput) ChangePreview {
	return s.previewView(func(snapshot *domain.Snapshot) ChangePreview {
		current := vesselCurrentVersion(snapshot, id)
		if current == nil {
			return ChangePreview{
				ExpectedVersion: input.ExpectedVersion,
				Conflict: conflictFromError(
					domain.NewError(domain.CodeVesselNotFound, "vessel was not found")),
			}
		}
		if err := checkExpected(*current, input.ExpectedVersion); err != nil {
			return ChangePreview{
				CurrentVersion:  current,
				ExpectedVersion: input.ExpectedVersion,
				Conflict:        conflictFromError(err),
			}
		}
		vessel, err := applyFinishCleaning(snapshot, id, s.clock.Now())
		return changePreview(current, input.ExpectedVersion, vessel, err)
	})
}

func snapshotVesselVersion(snapshot *domain.Snapshot, id string) (int64, error) {
	vessel, ok := snapshot.Vessels[id]
	if !ok {
		return 0, domain.NewError(domain.CodeVesselNotFound, "vessel was not found")
	}
	return vessel.Version, nil
}

func applyVesselRetirement(snapshot *domain.Snapshot, id string, now time.Time) (domain.Vessel, error) {
	vessel, ok := snapshot.Vessels[id]
	if !ok {
		return domain.Vessel{}, domain.NewError(domain.CodeVesselNotFound, "vessel was not found")
	}
	if err := vessel.Retire(now); err != nil {
		return domain.Vessel{}, err
	}
	return vessel, nil
}

func (s *Service) RetireVessel(id string, input VersionInput) (domain.Vessel, error) {
	var updated domain.Vessel
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		current, err := snapshotVesselVersion(snapshot, id)
		if err != nil {
			return err
		}
		if err := checkExpected(current, input.ExpectedVersion); err != nil {
			return err
		}
		vessel, err := applyVesselRetirement(snapshot, id, s.clock.Now())
		if err != nil {
			return err
		}
		snapshot.Vessels[id] = vessel
		updated = vessel
		return nil
	})
	if err != nil {
		return domain.Vessel{}, err
	}
	return updated, nil
}

// PreviewRetireVessel evaluates a vessel retirement without committing it.
func (s *Service) PreviewRetireVessel(id string, input VersionInput) ChangePreview {
	return s.previewView(func(snapshot *domain.Snapshot) ChangePreview {
		current := vesselCurrentVersion(snapshot, id)
		if current == nil {
			return ChangePreview{
				ExpectedVersion: input.ExpectedVersion,
				Conflict: conflictFromError(
					domain.NewError(domain.CodeVesselNotFound, "vessel was not found")),
			}
		}
		if err := checkExpected(*current, input.ExpectedVersion); err != nil {
			return ChangePreview{
				CurrentVersion:  current,
				ExpectedVersion: input.ExpectedVersion,
				Conflict:        conflictFromError(err),
			}
		}
		vessel, err := applyVesselRetirement(snapshot, id, s.clock.Now())
		return changePreview(current, input.ExpectedVersion, vessel, err)
	})
}
