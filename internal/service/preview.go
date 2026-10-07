package service

import (
	"errors"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

// PreviewConflict reports the stable error code that a real commit would hit
// if it were attempted against the snapshot observed by a preflight request.
type PreviewConflict struct {
	Code   domain.Code
	Detail string
	Field  string
}

// PreviewAssignment describes values that the real write would assign when it
// creates a new aggregate. A preflight never generates them; it only reports
// that the assignment happens server-side at commit time.
type PreviewAssignment struct {
	EntityIDAssigned bool
	CodeAssigned     bool
	InitialVersion   int64
}

// ChangePreview is the read-only result of evaluating a write request without
// applying it. Nothing in this structure is a persisted fact: a concurrent
// commit can invalidate it before the real write arrives.
type ChangePreview struct {
	SnapshotRevision int64
	CurrentVersion   *int64
	ExpectedVersion  *int64
	Conflict         *PreviewConflict
	Result           any
	Assignment       *PreviewAssignment
}

// RunVesselChange is the projected result of a write that changes both a run
// and its vessel (reserve, start, abort, complete).
type RunVesselChange struct {
	Run    domain.FermentationRun
	Vessel domain.Vessel
}

// PendingFormula is the projected aggregate of a formula creation that has not
// been assigned an identifier yet.
type PendingFormula struct {
	Formula domain.Formula
}

// PendingVessel is the projected aggregate of a vessel creation that has not
// been assigned an identifier yet.
type PendingVessel struct {
	Vessel domain.Vessel
}

// PendingRun is the projected aggregate of a run creation that has not been
// assigned an identifier yet. When CodeGenerated is true the run code is
// assigned server-side at commit time and is therefore not part of the
// projection.
type PendingRun struct {
	Run           domain.FermentationRun
	CodeGenerated bool
}

func conflictFromError(err error) *PreviewConflict {
	if err == nil {
		return nil
	}
	conflict := &PreviewConflict{
		Code:   domain.CodeStorage,
		Detail: "the proposed change could not be evaluated",
	}
	var domainErr *domain.Error
	if errors.As(err, &domainErr) {
		conflict.Code = domainErr.Code
		conflict.Detail = domainErr.Message
		conflict.Field = domainErr.Field
	}
	return conflict
}

// previewView evaluates a proposal against a detached copy of the current
// snapshot. The copy is never written back, so the projection cannot generate
// identifiers, advance revisions, or reach durable storage.
func (s *Service) previewView(visit func(snapshot *domain.Snapshot) ChangePreview) ChangePreview {
	var preview ChangePreview
	_ = s.store.View(func(snapshot domain.Snapshot) error {
		preview = visit(&snapshot)
		preview.SnapshotRevision = snapshot.Revision
		return nil
	})
	return preview
}

func formulaCurrentVersion(snapshot *domain.Snapshot, id string) *int64 {
	if formula, ok := snapshot.Formulas[id]; ok {
		version := formula.Version
		return &version
	}
	return nil
}

func vesselCurrentVersion(snapshot *domain.Snapshot, id string) *int64 {
	if vessel, ok := snapshot.Vessels[id]; ok {
		version := vessel.Version
		return &version
	}
	return nil
}

func runCurrentVersion(snapshot *domain.Snapshot, id string) *int64 {
	if run, ok := snapshot.Runs[id]; ok {
		version := run.Version
		return &version
	}
	return nil
}

func changePreview(
	current, expected *int64,
	result any,
	err error,
) ChangePreview {
	preview := ChangePreview{
		CurrentVersion:  current,
		ExpectedVersion: expected,
	}
	if err != nil {
		preview.Conflict = conflictFromError(err)
		return preview
	}
	preview.Result = result
	return preview
}
