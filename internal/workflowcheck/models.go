package workflowcheck

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

type formulaView struct {
	ID      string               `json:"id"`
	State   domain.FormulaState  `json:"state"`
	Version int64                `json:"version"`
	Steps   []domain.FormulaStep `json:"steps"`
}

type vesselView struct {
	ID          string             `json:"id"`
	State       domain.VesselState `json:"state"`
	Version     int64              `json:"version"`
	ActiveRunID *string            `json:"active_run_id"`
}

type runView struct {
	ID           string                      `json:"id"`
	State        domain.RunState             `json:"state"`
	Version      int64                       `json:"version"`
	VesselID     *string                     `json:"vessel_id"`
	Observations []domain.GravityObservation `json:"observations"`
	Metrics      *domain.RunMetrics          `json:"metrics"`
}

type pairView struct {
	Run    runView    `json:"run"`
	Vessel vesselView `json:"vessel"`
}

type previewConflictView struct {
	Code  string `json:"code"`
	Field string `json:"field"`
}

type previewAssignmentView struct {
	IDAssigned     bool  `json:"id_assigned"`
	InitialVersion int64 `json:"initial_version"`
	CodeAssigned   bool  `json:"code_assigned"`
}

type previewView struct {
	SnapshotRevision int64                      `json:"snapshot_revision"`
	CurrentVersion   *int64                     `json:"current_version"`
	ExpectedVersion  *int64                     `json:"expected_version"`
	Conflict         *previewConflictView       `json:"conflict"`
	Change           map[string]json.RawMessage `json:"change"`
	Assignment       *previewAssignmentView     `json:"assignment"`
}

type pendingFormulaView struct {
	ID      *string              `json:"id"`
	Name    string               `json:"name"`
	State   domain.FormulaState  `json:"state"`
	Version *int64               `json:"version"`
	Steps   []domain.FormulaStep `json:"steps"`
}

type pendingRunView struct {
	ID      *string         `json:"id"`
	Code    *string         `json:"code"`
	State   domain.RunState `json:"state"`
	Version *int64          `json:"version"`
}

func (p previewView) decodeFormula() (pendingFormulaView, error) {
	var value pendingFormulaView
	if err := json.Unmarshal(p.Change["formula"], &value); err != nil {
		return pendingFormulaView{}, err
	}
	return value, nil
}

func (p previewView) decodeVessel() (vesselView, error) {
	var value vesselView
	if err := json.Unmarshal(p.Change["vessel"], &value); err != nil {
		return vesselView{}, err
	}
	return value, nil
}

func (p previewView) decodeRun() (runView, error) {
	var value runView
	if err := json.Unmarshal(p.Change["run"], &value); err != nil {
		return runView{}, err
	}
	return value, nil
}

func (p previewView) decodePair() (pairView, error) {
	var value pairView
	if err := json.Unmarshal(p.Change["run"], &value.Run); err != nil {
		return pairView{}, err
	}
	if err := json.Unmarshal(p.Change["vessel"], &value.Vessel); err != nil {
		return pairView{}, err
	}
	return value, nil
}

func decodePreview(resp response) (previewView, error) {
	var value previewView
	if err := resp.decode(&value); err != nil {
		return previewView{}, err
	}
	return value, nil
}

func createFormulaPayload(name string) map[string]any {
	return map[string]any{
		"name":                    name,
		"style":                   "Cellar pilot",
		"target_original_gravity": 1.052,
		"target_final_gravity":    1.012,
		"max_fermentation_temp_c": 22.0,
		"minimum_days":            7,
		"maximum_days":            21,
	}
}

func formulaStepPayload() map[string]any {
	return map[string]any{
		"steps": []map[string]any{
			{
				"sequence":      1,
				"label":         "Heat and combine",
				"temperature_c": 66.0,
				"hold_minutes":  60,
			},
			{
				"sequence":      2,
				"label":         "Transfer and cool",
				"temperature_c": 20.0,
				"hold_minutes":  30,
			},
			{
				"sequence":      3,
				"label":         "Hold for primary activity",
				"temperature_c": 19.0,
				"hold_minutes":  240,
			},
		},
	}
}

func createRunPayload(code, formulaID string) map[string]any {
	return map[string]any{
		"code":                  code,
		"formula_id":            formulaID,
		"planned_volume_liters": 180.0,
		"notes":                 "pilot run",
	}
}

func verifiedAt(base time.Time, sequence int) time.Time {
	return base.Add(time.Duration(sequence) * 10 * time.Second)
}

func requireRunState(run runView, state domain.RunState) error {
	if run.State != state {
		return fmt.Errorf("expected run state %q, got %q", state, run.State)
	}
	return nil
}

func requireVesselState(vessel vesselView, state domain.VesselState) error {
	if vessel.State != state {
		return fmt.Errorf("expected vessel state %q, got %q", state, vessel.State)
	}
	return nil
}
