package httpapi

import (
	"net/http"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/service"
)

type previewConflictBody struct {
	Code   domain.Code `json:"code"`
	Detail string      `json:"detail"`
	Field  string      `json:"field,omitempty"`
}

type previewAssignmentBody struct {
	IDAssigned     bool  `json:"id_assigned"`
	InitialVersion int64 `json:"initial_version"`
	CodeAssigned   bool  `json:"code_assigned,omitempty"`
}

// changePreviewResponse is the read-only preflight envelope. The revision and
// versions describe what the operator is looking at right now; they become a
// real decision only when the matching commit is accepted.
type changePreviewResponse struct {
	SnapshotRevision int64                  `json:"snapshot_revision"`
	CurrentVersion   *int64                 `json:"current_version"`
	ExpectedVersion  *int64                 `json:"expected_version"`
	Conflict         *previewConflictBody   `json:"conflict"`
	Change           map[string]any         `json:"change"`
	Assignment       *previewAssignmentBody `json:"assignment,omitempty"`
}

func writePreview(response http.ResponseWriter, preview service.ChangePreview) {
	body := changePreviewResponse{
		SnapshotRevision: preview.SnapshotRevision,
		CurrentVersion:   preview.CurrentVersion,
		ExpectedVersion:  preview.ExpectedVersion,
	}
	if preview.Conflict != nil {
		body.Conflict = &previewConflictBody{
			Code:   preview.Conflict.Code,
			Detail: preview.Conflict.Detail,
			Field:  preview.Conflict.Field,
		}
	} else {
		body.Change = projectChange(preview.Result)
	}
	if preview.Assignment != nil {
		body.Assignment = &previewAssignmentBody{
			IDAssigned:     preview.Assignment.EntityIDAssigned,
			InitialVersion: preview.Assignment.InitialVersion,
			CodeAssigned:   preview.Assignment.CodeAssigned,
		}
	}
	writeJSON(response, http.StatusOK, body)
}

func projectChange(result any) map[string]any {
	switch value := result.(type) {
	case domain.Formula:
		return map[string]any{"formula": value}
	case domain.Vessel:
		return map[string]any{"vessel": value}
	case domain.FermentationRun:
		return map[string]any{"run": value}
	case service.RunVesselChange:
		return map[string]any{"run": value.Run, "vessel": value.Vessel}
	case service.PendingFormula:
		return map[string]any{"formula": pendingFormulaMap(value)}
	case service.PendingVessel:
		return map[string]any{"vessel": pendingVesselMap(value)}
	case service.PendingRun:
		return map[string]any{"run": pendingRunMap(value)}
	default:
		return nil
	}
}

func pendingFormulaMap(pending service.PendingFormula) map[string]any {
	formula := pending.Formula
	return map[string]any{
		"id":                      nil,
		"name":                    formula.Name,
		"style":                   formula.Style,
		"target_original_gravity": formula.TargetOriginalGravity,
		"target_final_gravity":    formula.TargetFinalGravity,
		"max_fermentation_temp_c": formula.MaxFermentationTempC,
		"minimum_days":            formula.MinimumDays,
		"maximum_days":            formula.MaximumDays,
		"steps":                   formula.Steps,
		"state":                   formula.State,
		"version":                 nil,
		"created_at":              formula.CreatedAt,
		"updated_at":              formula.UpdatedAt,
	}
}

func pendingVesselMap(pending service.PendingVessel) map[string]any {
	vessel := pending.Vessel
	return map[string]any{
		"id":              nil,
		"code":            vessel.Code,
		"capacity_liters": vessel.CapacityL,
		"state":           vessel.State,
		"version":         nil,
		"created_at":      vessel.CreatedAt,
		"updated_at":      vessel.UpdatedAt,
	}
}

func pendingRunMap(pending service.PendingRun) map[string]any {
	run := pending.Run
	projected := map[string]any{
		"id":                    nil,
		"formula_id":            run.FormulaID,
		"planned_volume_liters": run.PlannedVolumeL,
		"state":                 run.State,
		"observations":          run.Observations,
		"version":               nil,
		"created_at":            run.CreatedAt,
		"updated_at":            run.UpdatedAt,
	}
	if pending.CodeGenerated {
		projected["code"] = nil
	} else {
		projected["code"] = run.Code
	}
	return projected
}
