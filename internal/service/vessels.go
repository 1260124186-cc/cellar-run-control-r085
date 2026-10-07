package service

import (
	"sort"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

func (s *Service) CreateVessel(input CreateVesselInput) (domain.Vessel, error) {
	var created domain.Vessel
	if err := s.store.Execute(s.createVessel(input, &created)); err != nil {
		return domain.Vessel{}, err
	}
	return created, nil
}

func (s *Service) PreflightCreateVessel(
	input CreateVesselInput,
) (PreflightReport, domain.Vessel) {
	var created domain.Vessel
	report := s.preflight(s.createVessel(input, &created), noCurrentVersion)
	return report, created
}

func (s *Service) createVessel(
	input CreateVesselInput,
	created *domain.Vessel,
) func(*domain.Snapshot) error {
	return func(snapshot *domain.Snapshot) error {
		now := s.clock.Now()
		vessel := domain.Vessel{
			Code:      domain.NormalizeVesselCode(input.Code),
			CapacityL: input.CapacityL,
			State:     domain.VesselAvailable,
			Version:   1,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := vessel.Validate(); err != nil {
			return err
		}
		id, err := generateEntityID("vessel")
		if err != nil {
			return domain.NewError(domain.CodeStorage, err.Error())
		}
		vessel.ID = id
		if _, exists := snapshot.VesselIndex[vessel.Code]; exists {
			return domain.NewError(domain.CodeVesselConflict,
				"a vessel with the same code already exists")
		}
		snapshot.Vessels[vessel.ID] = vessel
		snapshot.VesselIndex[vessel.Code] = vessel.ID
		*created = vessel
		return nil
	}
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

func (s *Service) FinishCleaning(
	id string,
	input FinishCleaningInput,
) (domain.Vessel, error) {
	var updated domain.Vessel
	if err := s.store.Execute(s.finishCleaning(id, input, &updated)); err != nil {
		return domain.Vessel{}, err
	}
	return updated, nil
}

func (s *Service) PreflightFinishCleaning(
	id string,
	input FinishCleaningInput,
) (PreflightReport, domain.Vessel) {
	var updated domain.Vessel
	report := s.preflight(s.finishCleaning(id, input, &updated), vesselVersionOf(id))
	return report, updated
}

func (s *Service) finishCleaning(
	id string,
	input FinishCleaningInput,
	updated *domain.Vessel,
) func(*domain.Snapshot) error {
	return func(snapshot *domain.Snapshot) error {
		vessel, ok := snapshot.Vessels[id]
		if !ok {
			return domain.NewError(domain.CodeVesselNotFound, "vessel was not found")
		}
		if err := s.checkExpected(vessel.Version, input.ExpectedVersion); err != nil {
			return err
		}
		if vessel.ActiveRunID == nil {
			return domain.NewError(domain.CodeVesselState,
				"cleaning vessel has no associated run")
		}
		run, ok := snapshot.Runs[*vessel.ActiveRunID]
		if !ok {
			return domain.NewError(domain.CodeRunNotFound,
				"associated run was not found")
		}
		if run.State != domain.RunCompleted && run.State != domain.RunAborted {
			return domain.NewError(domain.CodeRunState,
				"associated run is still open")
		}
		if err := vessel.FinishCleaning(s.clock.Now()); err != nil {
			return err
		}
		snapshot.Vessels[id] = vessel
		*updated = vessel
		return nil
	}
}

func (s *Service) RetireVessel(id string, input VersionInput) (domain.Vessel, error) {
	var updated domain.Vessel
	if err := s.store.Execute(s.retireVessel(id, input, &updated)); err != nil {
		return domain.Vessel{}, err
	}
	return updated, nil
}

func (s *Service) PreflightRetireVessel(
	id string,
	input VersionInput,
) (PreflightReport, domain.Vessel) {
	var updated domain.Vessel
	report := s.preflight(s.retireVessel(id, input, &updated), vesselVersionOf(id))
	return report, updated
}

func (s *Service) retireVessel(
	id string,
	input VersionInput,
	updated *domain.Vessel,
) func(*domain.Snapshot) error {
	return func(snapshot *domain.Snapshot) error {
		vessel, ok := snapshot.Vessels[id]
		if !ok {
			return domain.NewError(domain.CodeVesselNotFound, "vessel was not found")
		}
		if err := s.checkExpected(vessel.Version, input.ExpectedVersion); err != nil {
			return err
		}
		if err := vessel.Retire(s.clock.Now()); err != nil {
			return err
		}
		snapshot.Vessels[id] = vessel
		*updated = vessel
		return nil
	}
}
