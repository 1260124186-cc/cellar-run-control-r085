package workflowcheck

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

// changePreviews exercises the read-only preflight entry points end to end.
// It verifies that a preview reports the current version, evaluates the same
// conflict rules as a commit, projects the resulting change, and never creates
// an entity, advances a version, bumps the snapshot revision, or writes state.
func changePreviews(api *client) error {
	if err := previewFormulaWorkflow(api); err != nil {
		return err
	}
	if err := previewVesselRunWorkflow(api); err != nil {
		return err
	}
	if err := previewObservationsWorkflow(api); err != nil {
		return err
	}
	return nil
}

func previewFormulaWorkflow(api *client) error {
	createdPreview, err := api.callPreview("POST", "/v1/formulas",
		createFormulaPayload("Preview Pale"), nil)
	if err != nil {
		return err
	}
	if err := requireStatus(createdPreview, 200, "preview create formula"); err != nil {
		return err
	}
	preview, err := decodePreview(createdPreview)
	if err != nil {
		return err
	}
	if err := requireRevision(api, preview, "preview create formula"); err != nil {
		return err
	}
	if err := requireNoConflict(preview, "preview create formula"); err != nil {
		return err
	}
	pending, err := preview.decodeFormula()
	if err != nil {
		return err
	}
	if pending.ID != nil || pending.Version != nil {
		return errInvalidWorkflow("preview assigned an identifier or version")
	}
	if pending.State != domain.FormulaDraft {
		return errInvalidWorkflow("preview formula did not project the draft state")
	}
	if preview.Assignment == nil || !preview.Assignment.IDAssigned ||
		preview.Assignment.InitialVersion != 1 {
		return errInvalidWorkflow("preview did not describe server-side id assignment")
	}
	if count, err := formulaCount(api); err != nil {
		return err
	} else if count != 0 {
		return errInvalidWorkflow("formula preview created an entity")
	}

	invalidPayload := createFormulaPayload("Preview Invalid")
	invalidPayload["target_original_gravity"] = 1.5
	invalidPreviewResp, err := api.callPreview("POST", "/v1/formulas", invalidPayload, nil)
	if err != nil {
		return err
	}
	invalidPreview, err := decodePreview(invalidPreviewResp)
	if err != nil {
		return err
	}
	if err := requireConflict(invalidPreview, string(domain.CodeInvalidInput),
		"preview invalid formula"); err != nil {
		return err
	}
	if invalidPreview.Change != nil {
		return errInvalidWorkflow("conflicting preview projected a change")
	}

	createdResponse, err := api.call("POST", "/v1/formulas",
		createFormulaPayload("Preview Pale"), nil)
	if err != nil {
		return err
	}
	if err := requireStatus(createdResponse, 201, "commit formula"); err != nil {
		return err
	}
	var formula formulaView
	if err := createdResponse.decode(&formula); err != nil {
		return err
	}

	duplicatePreview, err := api.callPreview("POST", "/v1/formulas",
		createFormulaPayload("  preview   pale "), nil)
	if err != nil {
		return err
	}
	duplicate, err := decodePreview(duplicatePreview)
	if err != nil {
		return err
	}
	if err := requireConflict(duplicate, string(domain.CodeFormulaNameConflict),
		"preview duplicate formula name"); err != nil {
		return err
	}
	if count, err := formulaCount(api); err != nil {
		return err
	} else if count != 1 {
		return errInvalidWorkflow("duplicate preview changed the formula collection")
	}

	approveEarly, err := api.callPreview("POST",
		"/v1/formulas/"+formula.ID+"/approve", nil, nil)
	if err != nil {
		return err
	}
	early, err := decodePreview(approveEarly)
	if err != nil {
		return err
	}
	if err := requireConflict(early, string(domain.CodeInvalidInput),
		"preview approve without steps"); err != nil {
		return err
	}
	if early.CurrentVersion == nil || *early.CurrentVersion != 1 {
		return errInvalidWorkflow("preview did not report current formula version")
	}

	stepsPreviewResp, err := api.callPreview("PUT",
		"/v1/formulas/"+formula.ID+"/steps", formulaStepPayload(), ptrInt64(1))
	if err != nil {
		return err
	}
	stepsPreview, err := decodePreview(stepsPreviewResp)
	if err != nil {
		return err
	}
	if err := requireNoConflict(stepsPreview, "preview set steps"); err != nil {
		return err
	}
	var stepsProjected formulaView
	if err := json.Unmarshal(stepsPreview.Change["formula"], &stepsProjected); err != nil {
		return err
	}
	if stepsProjected.Version != 2 || len(stepsProjected.Steps) != 3 {
		return errInvalidWorkflow("steps preview projected the wrong revision")
	}
	if err := requireFormulaVersion(api, formula.ID, 1, domain.FormulaDraft,
		"steps preview must not commit"); err != nil {
		return err
	}

	stepsCommit, err := api.call("PUT", "/v1/formulas/"+formula.ID+"/steps",
		formulaStepPayload(), ptrInt64(1))
	if err != nil {
		return err
	}
	if err := requireStatus(stepsCommit, 200, "commit set steps"); err != nil {
		return err
	}

	staleApprove, err := api.callPreview("POST",
		"/v1/formulas/"+formula.ID+"/approve", nil, ptrInt64(1))
	if err != nil {
		return err
	}
	stale, err := decodePreview(staleApprove)
	if err != nil {
		return err
	}
	if err := requireConflict(stale, string(domain.CodeRevisionConflict),
		"preview stale approve"); err != nil {
		return err
	}
	if stale.CurrentVersion == nil || *stale.CurrentVersion != 2 {
		return errInvalidWorkflow("stale preview reported the wrong current version")
	}

	approvePreviewResp, err := api.callPreview("POST",
		"/v1/formulas/"+formula.ID+"/approve", nil, ptrInt64(2))
	if err != nil {
		return err
	}
	approvePreview, err := decodePreview(approvePreviewResp)
	if err != nil {
		return err
	}
	if err := requireNoConflict(approvePreview, "preview approve"); err != nil {
		return err
	}
	if err := requireFormulaVersion(api, formula.ID, 2, domain.FormulaDraft,
		"approve preview must not commit"); err != nil {
		return err
	}

	approveCommit, err := api.call("POST", "/v1/formulas/"+formula.ID+"/approve",
		nil, ptrInt64(2))
	if err != nil {
		return err
	}
	if err := requireStatus(approveCommit, 200, "commit approve"); err != nil {
		return err
	}
	if err := approveCommit.decode(&formula); err != nil {
		return err
	}

	// The preview says retirement is clean while no run uses the formula. A
	// concurrent run creation then makes the real commit fail under the same
	// rules: the preview never became a persisted fact.
	retirePreviewResp, err := api.callPreview("POST",
		"/v1/formulas/"+formula.ID+"/retire", nil, ptrInt64(formula.Version))
	if err != nil {
		return err
	}
	retirePreview, err := decodePreview(retirePreviewResp)
	if err != nil {
		return err
	}
	if err := requireNoConflict(retirePreview, "preview retire"); err != nil {
		return err
	}
	if err := requireFormulaVersion(api, formula.ID, formula.Version,
		domain.FormulaApproved, "retire preview must not commit"); err != nil {
		return err
	}

	runCommit, err := api.call("POST", "/v1/runs",
		createRunPayload("RUN-PREVIEW-USES", formula.ID), nil)
	if err != nil {
		return err
	}
	if err := requireStatus(runCommit, 201, "concurrent run before retire commit"); err != nil {
		return err
	}
	retireCommit, err := api.call("POST", "/v1/formulas/"+formula.ID+"/retire",
		nil, ptrInt64(formula.Version))
	if err != nil {
		return err
	}
	if err := requireCode(retireCommit, 409, string(domain.CodeFormulaInUse),
		"commit retire after concurrent use"); err != nil {
		return err
	}
	return nil
}

func previewVesselRunWorkflow(api *client) error {
	formula, err := ensureApprovedFormula(api, "Preview Formula")
	if err != nil {
		return err
	}

	vesselResponse, err := api.call("POST", "/v1/vessels", map[string]any{
		"code":            "VSL-301",
		"capacity_liters": 500.0,
	}, nil)
	if err != nil {
		return err
	}
	if err := requireStatus(vesselResponse, 201, "commit vessel"); err != nil {
		return err
	}
	var vessel vesselView
	if err := vesselResponse.decode(&vessel); err != nil {
		return err
	}

	dupVessel, err := api.callPreview("POST", "/v1/vessels", map[string]any{
		"code":            "VSL-301",
		"capacity_liters": 500.0,
	}, nil)
	if err != nil {
		return err
	}
	dup, err := decodePreview(dupVessel)
	if err != nil {
		return err
	}
	if err := requireConflict(dup, string(domain.CodeVesselConflict),
		"preview duplicate vessel code"); err != nil {
		return err
	}

	smallVessel, err := api.callPreview("POST", "/v1/vessels", map[string]any{
		"code":            "VSL-303",
		"capacity_liters": 2.0,
	}, nil)
	if err != nil {
		return err
	}
	small, err := decodePreview(smallVessel)
	if err != nil {
		return err
	}
	if err := requireConflict(small, string(domain.CodeInvalidInput),
		"preview undersized vessel"); err != nil {
		return err
	}

	retireVesselPreview, err := api.callPreview("POST",
		"/v1/vessels/"+vessel.ID+"/retire", nil, ptrInt64(vessel.Version))
	if err != nil {
		return err
	}
	retireVessel, err := decodePreview(retireVesselPreview)
	if err != nil {
		return err
	}
	if err := requireNoConflict(retireVessel, "preview retire vessel"); err != nil {
		return err
	}
	if err := requireVesselVersion(api, vessel.ID, 1, domain.VesselAvailable,
		"retire vessel preview must not commit"); err != nil {
		return err
	}

	secondVesselResponse, err := api.call("POST", "/v1/vessels", map[string]any{
		"code":            "VSL-302",
		"capacity_liters": 400.0,
	}, nil)
	if err != nil {
		return err
	}
	if err := requireStatus(secondVesselResponse, 201, "commit second vessel"); err != nil {
		return err
	}
	var secondVessel vesselView
	if err := secondVesselResponse.decode(&secondVessel); err != nil {
		return err
	}

	runResponse, err := api.call("POST", "/v1/runs",
		createRunPayload("RUN-PREVIEW-A", formula.ID), nil)
	if err != nil {
		return err
	}
	if err := requireStatus(runResponse, 201, "commit run"); err != nil {
		return err
	}
	var run runView
	if err := runResponse.decode(&run); err != nil {
		return err
	}

	draftResponse, err := api.call("POST", "/v1/formulas",
		createFormulaPayload("Preview Draft"), nil)
	if err != nil {
		return err
	}
	var draftFormula formulaView
	if err := draftResponse.decode(&draftFormula); err != nil {
		return err
	}
	draftRunPreview, err := api.callPreview("POST", "/v1/runs",
		createRunPayload("RUN-PREVIEW-DRAFT", draftFormula.ID), nil)
	if err != nil {
		return err
	}
	draftRun, err := decodePreview(draftRunPreview)
	if err != nil {
		return err
	}
	if err := requireConflict(draftRun, string(domain.CodeFormulaState),
		"preview run against draft formula"); err != nil {
		return err
	}

	dupRunPreview, err := api.callPreview("POST", "/v1/runs",
		createRunPayload("RUN-PREVIEW-A", formula.ID), nil)
	if err != nil {
		return err
	}
	dupRun, err := decodePreview(dupRunPreview)
	if err != nil {
		return err
	}
	if err := requireConflict(dupRun, string(domain.CodeRunConflict),
		"preview duplicate run code"); err != nil {
		return err
	}

	generatedCodePreview, err := api.callPreview("POST", "/v1/runs", map[string]any{
		"formula_id":            formula.ID,
		"planned_volume_liters": 120.0,
		"notes":                 "generated code",
	}, nil)
	if err != nil {
		return err
	}
	generated, err := decodePreview(generatedCodePreview)
	if err != nil {
		return err
	}
	if err := requireNoConflict(generated, "preview generated-code run"); err != nil {
		return err
	}
	pendingRun, err := decodePendingRun(generated)
	if err != nil {
		return err
	}
	if pendingRun.Code != nil || pendingRun.ID != nil || pendingRun.Version != nil {
		return errInvalidWorkflow("preview leaked a generated code, id, or version")
	}
	if generated.Assignment == nil || !generated.Assignment.CodeAssigned {
		return errInvalidWorkflow("preview did not mark the run code as server-assigned")
	}

	reservePreviewResp, err := api.callPreview("POST",
		"/v1/runs/"+run.ID+"/reserve",
		map[string]any{"vessel_id": vessel.ID}, ptrInt64(run.Version))
	if err != nil {
		return err
	}
	reservePreview, err := decodePreview(reservePreviewResp)
	if err != nil {
		return err
	}
	if err := requireNoConflict(reservePreview, "preview reserve"); err != nil {
		return err
	}
	pair, err := reservePreview.decodePair()
	if err != nil {
		return err
	}
	if err := requireRunState(pair.Run, domain.RunReserved); err != nil {
		return err
	}
	if err := requireVesselState(pair.Vessel, domain.VesselReserved); err != nil {
		return err
	}
	if err := requireRunVersion(api, run.ID, 1, domain.RunPlanned,
		"reserve preview must not commit"); err != nil {
		return err
	}
	if err := requireVesselVersion(api, vessel.ID, 1, domain.VesselAvailable,
		"reserve preview must not change the vessel"); err != nil {
		return err
	}

	reserveCommit, err := api.call("POST", "/v1/runs/"+run.ID+"/reserve",
		map[string]any{"vessel_id": vessel.ID}, ptrInt64(1))
	if err != nil {
		return err
	}
	if err := requireStatus(reserveCommit, 200, "commit reserve"); err != nil {
		return err
	}
	if err := reserveCommit.decode(&pair); err != nil {
		return err
	}

	staleReserve, err := api.callPreview("POST", "/v1/runs/"+run.ID+"/reserve",
		map[string]any{"vessel_id": vessel.ID}, ptrInt64(1))
	if err != nil {
		return err
	}
	stale, err := decodePreview(staleReserve)
	if err != nil {
		return err
	}
	if err := requireConflict(stale, string(domain.CodeRevisionConflict),
		"preview stale reserve"); err != nil {
		return err
	}

	occupiedReserve, err := api.callPreview("POST", "/v1/runs/"+run.ID+"/reserve",
		map[string]any{"vessel_id": vessel.ID}, ptrInt64(pair.Run.Version))
	if err != nil {
		return err
	}
	occupied, err := decodePreview(occupiedReserve)
	if err != nil {
		return err
	}
	if occupied.Conflict == nil {
		return errInvalidWorkflow("expected a conflict re-reserving an occupied vessel")
	}

	startPreviewResp, err := api.callPreview("POST", "/v1/runs/"+run.ID+"/start",
		nil, ptrInt64(pair.Run.Version))
	if err != nil {
		return err
	}
	startPreview, err := decodePreview(startPreviewResp)
	if err != nil {
		return err
	}
	if err := requireNoConflict(startPreview, "preview start"); err != nil {
		return err
	}
	if err := requireRunVersion(api, run.ID, pair.Run.Version, domain.RunReserved,
		"start preview must not commit"); err != nil {
		return err
	}
	startCommit, err := api.call("POST", "/v1/runs/"+run.ID+"/start",
		nil, ptrInt64(pair.Run.Version))
	if err != nil {
		return err
	}
	if err := requireStatus(startCommit, 200, "commit start"); err != nil {
		return err
	}
	if err := startCommit.decode(&pair); err != nil {
		return err
	}

	// A second run/vessel pair validates the abort projection, including the
	// vessel release side effect.
	secondRunResponse, err := api.call("POST", "/v1/runs",
		createRunPayload("RUN-PREVIEW-B", formula.ID), nil)
	if err != nil {
		return err
	}
	var secondRun runView
	if err := secondRunResponse.decode(&secondRun); err != nil {
		return err
	}
	secondReserve, err := api.call("POST", "/v1/runs/"+secondRun.ID+"/reserve",
		map[string]any{"vessel_id": secondVessel.ID}, ptrInt64(secondRun.Version))
	if err != nil {
		return err
	}
	var secondPair pairView
	if err := secondReserve.decode(&secondPair); err != nil {
		return err
	}
	abortPreviewResp, err := api.callPreview("POST",
		"/v1/runs/"+secondRun.ID+"/abort", nil, ptrInt64(secondPair.Run.Version))
	if err != nil {
		return err
	}
	abortPreview, err := decodePreview(abortPreviewResp)
	if err != nil {
		return err
	}
	if err := requireNoConflict(abortPreview, "preview abort before fermentation"); err != nil {
		return err
	}
	abortPair, err := abortPreview.decodePair()
	if err != nil {
		return err
	}
	if err := requireRunState(abortPair.Run, domain.RunAborted); err != nil {
		return err
	}
	if err := requireVesselState(abortPair.Vessel, domain.VesselAvailable); err != nil {
		return err
	}
	if err := requireRunVersion(api, secondRun.ID, secondPair.Run.Version,
		domain.RunReserved, "abort preview must not commit"); err != nil {
		return err
	}
	if err := requireVesselVersion(api, secondVessel.ID, secondPair.Vessel.Version,
		domain.VesselReserved, "abort preview must not release the vessel"); err != nil {
		return err
	}
	abortCommit, err := api.call("POST", "/v1/runs/"+secondRun.ID+"/abort",
		nil, ptrInt64(secondPair.Run.Version))
	if err != nil {
		return err
	}
	if err := requireStatus(abortCommit, 200, "commit abort"); err != nil {
		return err
	}

	// A malformed opt-in header is a client error and never reaches a commit.
	before, err := vesselCount(api)
	if err != nil {
		return err
	}
	badHeader, err := api.callHeader("POST", "/v1/vessels", "yes", map[string]any{
		"code":            "VSL-999",
		"capacity_liters": 100.0,
	})
	if err != nil {
		return err
	}
	if err := requireCode(badHeader, 400, string(domain.CodeInvalidInput),
		"malformed preview header"); err != nil {
		return err
	}
	after, err := vesselCount(api)
	if err != nil {
		return err
	}
	if before != after {
		return errInvalidWorkflow("malformed preview header changed vessel collection")
	}
	return nil
}

func previewObservationsWorkflow(api *client) error {
	formula, err := ensureApprovedFormula(api, "Preview Observation Formula")
	if err != nil {
		return err
	}
	vessel, err := createCheckVessel(api, "VSL-404", 500)
	if err != nil {
		return err
	}
	started, err := createStartedRun(api, formula.ID, vessel.ID, "RUN-PREVIEW-OBS")
	if err != nil {
		return err
	}
	runID := started.Run.ID
	version := started.Run.Version
	base := verifiedAtBase()

	firstPreviewResp, err := api.callPreview("POST", "/v1/runs/"+runID+"/observations",
		observationPayload(1, 1.050, 20.0, verifiedAt(base, 1), "preview one"),
		nil)
	if err != nil {
		return err
	}
	firstPreview, err := decodePreview(firstPreviewResp)
	if err != nil {
		return err
	}
	if err := requireNoConflict(firstPreview, "preview observation"); err != nil {
		return err
	}
	projectedRun, err := firstPreview.decodeRun()
	if err != nil {
		return err
	}
	if projectedRun.Version != version+1 || len(projectedRun.Observations) != 1 {
		return errInvalidWorkflow("observation preview projected the wrong run state")
	}
	if err := requireObservationCount(api, runID, 0,
		"observation preview must not append"); err != nil {
		return err
	}

	firstCommit, err := api.call("POST", "/v1/runs/"+runID+"/observations",
		observationPayload(1, 1.050, 20.0, verifiedAt(base, 1), "commit one"),
		ptrInt64(version))
	if err != nil {
		return err
	}
	var committed runView
	if err := firstCommit.decode(&committed); err != nil {
		return err
	}
	version = committed.Version

	secondCommit, err := api.call("POST", "/v1/runs/"+runID+"/observations",
		observationPayload(2, 1.030, 20.2, verifiedAt(base, 2), "commit two"),
		ptrInt64(version))
	if err != nil {
		return err
	}
	if err := secondCommit.decode(&committed); err != nil {
		return err
	}
	version = committed.Version

	nonMonotonic, err := api.callPreview("POST", "/v1/runs/"+runID+"/observations",
		observationPayload(3, 1.020, 20.0, verifiedAt(base, 1), "bad time"),
		nil)
	if err != nil {
		return err
	}
	monotonicPreview, err := decodePreview(nonMonotonic)
	if err != nil {
		return err
	}
	if err := requireConflict(monotonicPreview, string(domain.CodeObservationConflict),
		"preview non-monotonic observation"); err != nil {
		return err
	}

	outOfRange, err := api.callPreview("POST", "/v1/runs/"+runID+"/observations",
		observationPayload(3, 1.5, 20.0, verifiedAt(base, 3), "bad gravity"),
		nil)
	if err != nil {
		return err
	}
	rangePreview, err := decodePreview(outOfRange)
	if err != nil {
		return err
	}
	if err := requireConflict(rangePreview, string(domain.CodeInvalidInput),
		"preview out-of-range gravity"); err != nil {
		return err
	}

	thirdCommit, err := api.call("POST", "/v1/runs/"+runID+"/observations",
		observationPayload(3, 1.012, 19.9, verifiedAt(base, 3), "commit three"),
		ptrInt64(version))
	if err != nil {
		return err
	}
	if err := thirdCommit.decode(&committed); err != nil {
		return err
	}
	version = committed.Version

	conditioningPreviewResp, err := api.callPreview("POST",
		"/v1/runs/"+runID+"/conditioning", nil, ptrInt64(version))
	if err != nil {
		return err
	}
	conditioningPreview, err := decodePreview(conditioningPreviewResp)
	if err != nil {
		return err
	}
	if err := requireNoConflict(conditioningPreview, "preview conditioning"); err != nil {
		return err
	}
	if err := requireRunVersion(api, runID, version, domain.RunFermenting,
		"conditioning preview must not commit"); err != nil {
		return err
	}
	conditioningCommit, err := api.call("POST", "/v1/runs/"+runID+"/conditioning",
		nil, ptrInt64(version))
	if err != nil {
		return err
	}
	if err := conditioningCommit.decode(&committed); err != nil {
		return err
	}
	version = committed.Version

	completePreviewResp, err := api.callPreview("POST", "/v1/runs/"+runID+"/complete",
		nil, ptrInt64(version))
	if err != nil {
		return err
	}
	completePreview, err := decodePreview(completePreviewResp)
	if err != nil {
		return err
	}
	if err := requireNoConflict(completePreview, "preview complete"); err != nil {
		return err
	}
	completedPair, err := completePreview.decodePair()
	if err != nil {
		return err
	}
	if err := requireRunState(completedPair.Run, domain.RunCompleted); err != nil {
		return err
	}
	if err := requireVesselState(completedPair.Vessel, domain.VesselCleaning); err != nil {
		return err
	}
	if completedPair.Run.Metrics == nil ||
		completedPair.Run.Metrics.ObservationCount != 3 {
		return errInvalidWorkflow("completion preview did not calculate metrics")
	}
	if err := requireRunVersion(api, runID, version, domain.RunConditioning,
		"complete preview must not commit"); err != nil {
		return err
	}
	if err := requireVesselVersion(api, vessel.ID, started.Vessel.Version,
		domain.VesselInUse, "complete preview must not clean the vessel"); err != nil {
		return err
	}

	finishEarly, err := api.callPreview("POST",
		"/v1/vessels/"+vessel.ID+"/finish-cleaning", nil,
		ptrInt64(started.Vessel.Version))
	if err != nil {
		return err
	}
	earlyFinish, err := decodePreview(finishEarly)
	if err != nil {
		return err
	}
	if err := requireConflict(earlyFinish, string(domain.CodeRunState),
		"preview finish-cleaning before completion"); err != nil {
		return err
	}

	staleComplete, err := api.callPreview("POST", "/v1/runs/"+runID+"/complete",
		nil, ptrInt64(version-1))
	if err != nil {
		return err
	}
	stale, err := decodePreview(staleComplete)
	if err != nil {
		return err
	}
	if err := requireConflict(stale, string(domain.CodeRevisionConflict),
		"preview stale complete"); err != nil {
		return err
	}

	completeCommit, err := api.call("POST", "/v1/runs/"+runID+"/complete",
		nil, ptrInt64(version))
	if err != nil {
		return err
	}
	if err := requireStatus(completeCommit, 200, "commit complete"); err != nil {
		return err
	}
	if err := completeCommit.decode(&completedPair); err != nil {
		return err
	}

	finishPreviewResp, err := api.callPreview("POST",
		"/v1/vessels/"+vessel.ID+"/finish-cleaning", nil,
		ptrInt64(completedPair.Vessel.Version))
	if err != nil {
		return err
	}
	finishPreview, err := decodePreview(finishPreviewResp)
	if err != nil {
		return err
	}
	if err := requireNoConflict(finishPreview, "preview finish-cleaning"); err != nil {
		return err
	}
	finishVessel, err := finishPreview.decodeVessel()
	if err != nil {
		return err
	}
	if err := requireVesselState(finishVessel, domain.VesselAvailable); err != nil {
		return err
	}
	if err := requireVesselVersion(api, vessel.ID, completedPair.Vessel.Version,
		domain.VesselCleaning, "finish-cleaning preview must not commit"); err != nil {
		return err
	}

	finishCommit, err := api.call("POST", "/v1/vessels/"+vessel.ID+"/finish-cleaning",
		nil, ptrInt64(completedPair.Vessel.Version))
	if err != nil {
		return err
	}
	if err := requireStatus(finishCommit, 200, "commit finish-cleaning"); err != nil {
		return err
	}
	return nil
}

func requireNoConflict(preview previewView, operation string) error {
	if preview.Conflict != nil {
		return fmt.Errorf("%s: unexpected conflict %q (%s)",
			operation, preview.Conflict.Code, preview.Conflict.Field)
	}
	if preview.Change == nil {
		return fmt.Errorf("%s: preview carried no projected change", operation)
	}
	return nil
}

func requireConflict(preview previewView, expectedCode, operation string) error {
	if preview.Conflict == nil {
		return fmt.Errorf("%s: expected conflict %q, but none was reported",
			operation, expectedCode)
	}
	if preview.Conflict.Code != expectedCode {
		return fmt.Errorf("%s: expected conflict %q, got %q",
			operation, expectedCode, preview.Conflict.Code)
	}
	return nil
}

func requireRevision(api *client, preview previewView, operation string) error {
	resp, err := api.call("GET", "/healthz", nil, nil)
	if err != nil {
		return err
	}
	var health struct {
		Revision int64 `json:"revision"`
	}
	if err := resp.decode(&health); err != nil {
		return err
	}
	if preview.SnapshotRevision != health.Revision {
		return fmt.Errorf("%s: preview reported revision %d for a snapshot at %d",
			operation, preview.SnapshotRevision, health.Revision)
	}
	return nil
}

func decodePendingRun(preview previewView) (pendingRunView, error) {
	var value pendingRunView
	if err := json.Unmarshal(preview.Change["run"], &value); err != nil {
		return pendingRunView{}, err
	}
	return value, nil
}

func collectionCount(api *client, path, key string) (int, error) {
	resp, err := api.call("GET", path, nil, nil)
	if err != nil {
		return 0, err
	}
	if err := requireStatus(resp, 200, "count "+path); err != nil {
		return 0, err
	}
	var body map[string][]json.RawMessage
	if err := resp.decode(&body); err != nil {
		return 0, err
	}
	return len(body[key]), nil
}

func formulaCount(api *client) (int, error) {
	return collectionCount(api, "/v1/formulas", "formulas")
}

func vesselCount(api *client) (int, error) {
	return collectionCount(api, "/v1/vessels", "vessels")
}

func requireFormulaVersion(
	api *client, id string, version int64, state domain.FormulaState, operation string,
) error {
	resp, err := api.call("GET", "/v1/formulas/"+id, nil, nil)
	if err != nil {
		return err
	}
	var view formulaView
	if err := resp.decode(&view); err != nil {
		return err
	}
	if view.Version != version || view.State != state {
		return fmt.Errorf("%s: formula is v%d/%s, want v%d/%s",
			operation, view.Version, view.State, version, state)
	}
	return nil
}

func requireVesselVersion(
	api *client, id string, version int64, state domain.VesselState, operation string,
) error {
	resp, err := api.call("GET", "/v1/vessels/"+id, nil, nil)
	if err != nil {
		return err
	}
	var view vesselView
	if err := resp.decode(&view); err != nil {
		return err
	}
	if view.Version != version || view.State != state {
		return fmt.Errorf("%s: vessel is v%d/%s, want v%d/%s",
			operation, view.Version, view.State, version, state)
	}
	return nil
}

func requireRunVersion(
	api *client, id string, version int64, state domain.RunState, operation string,
) error {
	resp, err := api.call("GET", "/v1/runs/"+id, nil, nil)
	if err != nil {
		return err
	}
	var view runView
	if err := resp.decode(&view); err != nil {
		return err
	}
	if view.Version != version || view.State != state {
		return fmt.Errorf("%s: run is v%d/%s, want v%d/%s",
			operation, view.Version, view.State, version, state)
	}
	return nil
}

func requireObservationCount(api *client, id string, count int, operation string) error {
	resp, err := api.call("GET", "/v1/runs/"+id, nil, nil)
	if err != nil {
		return err
	}
	var view runView
	if err := resp.decode(&view); err != nil {
		return err
	}
	if len(view.Observations) != count {
		return fmt.Errorf("%s: run has %d observations, want %d",
			operation, len(view.Observations), count)
	}
	return nil
}

func verifiedAtBase() time.Time {
	return time.Now().UTC().Add(10 * time.Second)
}

func observationPayload(
	sequence int, gravity, temperature float64, observedAt time.Time, note string,
) map[string]any {
	return map[string]any{
		"sequence":      sequence,
		"gravity":       gravity,
		"temperature_c": temperature,
		"observed_at":   observedAt,
		"note":          note,
	}
}
