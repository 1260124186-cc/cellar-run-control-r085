package service

import (
	"time"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

type CreateFormulaInput struct {
	Name                  string
	Style                 string
	TargetOriginalGravity float64
	TargetFinalGravity    float64
	MaxFermentationTempC  float64
	MinimumDays           int
	MaximumDays           int
}

type SetFormulaStepsInput struct {
	ExpectedVersion *int64
	Steps           []domain.FormulaStep
}

type VersionInput struct {
	ExpectedVersion *int64
}

type CreateVesselInput struct {
	Code      string
	CapacityL float64
}

type CreateRunInput struct {
	Code           string
	FormulaID      string
	PlannedVolumeL float64
	Notes          string
}

type ReserveRunInput struct {
	ExpectedVersion *int64
	VesselID        string
}

type ObservationInput struct {
	Sequence     int
	Gravity      float64
	TemperatureC float64
	ObservedAt   time.Time
	Note         string
}

type FinishCleaningInput struct {
	ExpectedVersion *int64
}

// PreflightReport describes a read-only rehearsal of a write. It tells the
// operator what the committed write would do right now without creating
// entities, advancing versions, or writing the snapshot. A concurrent
// change between the preflight and the real commit is settled by the
// committed write under the existing conflict rules; the report itself is
// never persisted.
type PreflightReport struct {
	// CurrentVersion is the version of the target aggregate when the
	// preflight ran. It is zero when the target does not exist yet.
	CurrentVersion int64
	// Revision is the snapshot revision when the preflight ran.
	Revision int64
	// WouldApply reports whether the same write would succeed if it were
	// submitted immediately with the same input and expected version.
	WouldApply bool
	// Conflict is the error the committed write would return right now.
	// It is nil when WouldApply is true.
	Conflict *domain.Error
}
