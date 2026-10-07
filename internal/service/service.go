package service

import (
	"errors"
	"strings"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/clock"
	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/storage"
)

type Service struct {
	store *storage.Store
	clock clock.Clock
}

func New(store *storage.Store, timeSource clock.Clock) *Service {
	if timeSource == nil {
		timeSource = clock.System{}
	}
	return &Service{store: store, clock: timeSource}
}

func (s *Service) Revision() int64 {
	return s.store.Revision()
}

func (s *Service) checkExpected(current int64, expected *int64) error {
	if expected != nil && current != *expected {
		return domain.NewError(domain.CodeRevisionConflict,
			"the aggregate version does not match X-Expected-Version")
	}
	return nil
}

// preflight rehearses a write mutation against a disposable snapshot clone
// and reports what the committed write would do right now. The clone is
// discarded: no entity is created, no version advances, and no snapshot is
// written. currentOf reads the target aggregate version from the clone
// before the mutation runs.
func (s *Service) preflight(
	mutate func(*domain.Snapshot) error,
	currentOf func(domain.Snapshot) int64,
) PreflightReport {
	report := PreflightReport{WouldApply: true}
	err := s.store.Preview(func(snapshot *domain.Snapshot) error {
		report.Revision = snapshot.Revision
		report.CurrentVersion = currentOf(*snapshot)
		return mutate(snapshot)
	})
	if err != nil {
		report.WouldApply = false
		report.Conflict = preflightConflict(err)
	}
	return report
}

func preflightConflict(err error) *domain.Error {
	var domainErr *domain.Error
	if errors.As(err, &domainErr) {
		return domainErr
	}
	return domain.NewError(domain.CodeStorage, err.Error())
}

func formulaVersionOf(id string) func(domain.Snapshot) int64 {
	return func(snapshot domain.Snapshot) int64 {
		if formula, ok := snapshot.Formulas[id]; ok {
			return formula.Version
		}
		return 0
	}
}

func vesselVersionOf(id string) func(domain.Snapshot) int64 {
	return func(snapshot domain.Snapshot) int64 {
		if vessel, ok := snapshot.Vessels[id]; ok {
			return vessel.Version
		}
		return 0
	}
}

func runVersionOf(id string) func(domain.Snapshot) int64 {
	return func(snapshot domain.Snapshot) int64 {
		if run, ok := snapshot.Runs[id]; ok {
			return run.Version
		}
		return 0
	}
}

// noCurrentVersion reports a zero current version for writes that create a
// new aggregate and therefore have no existing target.
func noCurrentVersion(domain.Snapshot) int64 {
	return 0
}

func generateEntityID(prefix string) (string, error) {
	return domain.NewID(prefix)
}

func generatedRunCode() (string, error) {
	value, err := domain.NewID("run")
	if err != nil {
		return "", err
	}
	suffix := strings.ToUpper(strings.TrimPrefix(value, "run_"))
	return "RUN-" + suffix, nil
}
