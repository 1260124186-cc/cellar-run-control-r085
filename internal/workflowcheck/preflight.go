package workflowcheck

import (
	"encoding/json"
	"fmt"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

// preflight verifies that rehearsed writes report the current version, the
// conflict the committed write would return, and the would-be state change
// without creating entities, advancing versions, or writing the snapshot.
func preflight(api *client) error {
	baseline, err := currentHealth(api)
	if err != nil {
		return err
	}
	realWrites := int64(0)

	createPreview, err := runPreflight(api, "POST", "/v1/formulas",
		createFormulaPayload("Preflight Formula"), nil)
	if err != nil {
		return err
	}
	if err := requireWouldApply(createPreview, "preflight create formula"); err != nil {
		return err
	}
	if createPreview.Preflight.CurrentVersion != 0 {
		return fmt.Errorf("create preflight reported a current version for a missing aggregate")
	}
	if createPreview.Preflight.SnapshotRevision != baseline.Revision {
		return fmt.Errorf("create preflight reported a stale snapshot revision")
	}
	var createdPreview formulaView
	if err := createPreview.decodeResult(&createdPreview); err != nil {
		return err
	}
	if createdPreview.State != domain.FormulaDraft || createdPreview.Version != 1 {
		return fmt.Errorf("create preflight did not preview the would-be formula")
	}
	if err := requireFormulaCount(api, 0); err != nil {
		return err
	}
	if err := requireRevisionUnchanged(api, baseline.Revision+realWrites); err != nil {
		return err
	}

	createdResponse, err := api.call("POST", "/v1/formulas",
		createFormulaPayload("Preflight Formula"), nil)
	if err != nil {
		return err
	}
	if err := requireStatus(createdResponse, 201, "create formula"); err != nil {
		return err
	}
	realWrites++
	var formula formulaView
	if err := createdResponse.decode(&formula); err != nil {
		return err
	}

	duplicate, err := runPreflight(api, "POST", "/v1/formulas",
		createFormulaPayload(" preflight   formula "), nil)
	if err != nil {
		return err
	}
	if err := requirePreflightConflict(duplicate,
		string(domain.CodeFormulaNameConflict), "preflight duplicate formula"); err != nil {
		return err
	}
	if err := requireFormulaCount(api, 1); err != nil {
		return err
	}

	missing, err := runPreflight(api, "POST", "/v1/formulas/formula_missing/approve",
		nil, ptrInt64(1))
	if err != nil {
		return err
	}
	if err := requirePreflightConflict(missing,
		string(domain.CodeFormulaNotFound), "preflight missing formula"); err != nil {
		return err
	}
	if missing.Preflight.CurrentVersion != 0 {
		return fmt.Errorf("missing formula preflight reported a current version")
	}

	premature, err := runPreflight(api, "POST", "/v1/formulas/"+formula.ID+"/approve",
		nil, ptrInt64(1))
	if err != nil {
		return err
	}
	if err := requirePreflightConflict(premature,
		string(domain.CodeInvalidInput), "preflight approve without steps"); err != nil {
		return err
	}
	if premature.Preflight.CurrentVersion != 1 {
		return fmt.Errorf("approve preflight reported version %d, want 1",
			premature.Preflight.CurrentVersion)
	}

	stale, err := runPreflight(api, "POST", "/v1/formulas/"+formula.ID+"/approve",
		nil, ptrInt64(7))
	if err != nil {
		return err
	}
	if err := requirePreflightConflict(stale,
		string(domain.CodeRevisionConflict), "preflight approve with stale version"); err != nil {
		return err
	}

	stepsPreview, err := runPreflight(api, "PUT", "/v1/formulas/"+formula.ID+"/steps",
		formulaStepPayload(), ptrInt64(1))
	if err != nil {
		return err
	}
	if err := requireWouldApply(stepsPreview, "preflight set steps"); err != nil {
		return err
	}
	var stepsResult formulaView
	if err := stepsPreview.decodeResult(&stepsResult); err != nil {
		return err
	}
	if stepsResult.Version != 2 || len(stepsResult.Steps) != 3 {
		return fmt.Errorf("set steps preflight did not preview the would-be revision")
	}
	current, err := readFormula(api, formula.ID)
	if err != nil {
		return err
	}
	if current.Version != 1 || len(current.Steps) != 0 {
		return fmt.Errorf("set steps preflight advanced the persisted formula")
	}
	if err := requireRevisionUnchanged(api, baseline.Revision+realWrites); err != nil {
		return err
	}

	stepsResponse, err := api.call("PUT", "/v1/formulas/"+formula.ID+"/steps",
		formulaStepPayload(), ptrInt64(1))
	if err != nil {
		return err
	}
	if err := requireStatus(stepsResponse, 200, "set formula steps"); err != nil {
		return err
	}
	realWrites++

	approvePreview, err := runPreflight(api, "POST", "/v1/formulas/"+formula.ID+"/approve",
		nil, ptrInt64(2))
	if err != nil {
		return err
	}
	if err := requireWouldApply(approvePreview, "preflight approve"); err != nil {
		return err
	}
	var approveResult formulaView
	if err := approvePreview.decodeResult(&approveResult); err != nil {
		return err
	}
	if approveResult.State != domain.FormulaApproved || approveResult.Version != 3 {
		return fmt.Errorf("approve preflight did not preview the approved formula")
	}

	approvedResponse, err := api.call("POST", "/v1/formulas/"+formula.ID+"/approve",
		nil, ptrInt64(2))
	if err != nil {
		return err
	}
	if err := requireStatus(approvedResponse, 200, "approve formula"); err != nil {
		return err
	}
	realWrites++
	if err := approvedResponse.decode(&formula); err != nil {
		return err
	}
	if formula.State != domain.FormulaApproved || formula.Version != 3 {
		return fmt.Errorf("approve after preflight has unexpected state or version")
	}

	vessel, err := createCheckVessel(api, "VSL-909", 240)
	if err != nil {
		return err
	}
	realWrites++
	first, err := createCheckRun(api, "RUN-PREFLIGHT-A", formula.ID)
	if err != nil {
		return err
	}
	realWrites++
	second, err := createCheckRun(api, "RUN-PREFLIGHT-B", formula.ID)
	if err != nil {
		return err
	}
	realWrites++

	reservePreview, err := runPreflight(api, "POST", "/v1/runs/"+first.ID+"/reserve",
		map[string]any{"vessel_id": vessel.ID}, ptrInt64(1))
	if err != nil {
		return err
	}
	if err := requireWouldApply(reservePreview, "preflight reserve"); err != nil {
		return err
	}
	if reservePreview.Preflight.CurrentVersion != 1 {
		return fmt.Errorf("reserve preflight reported version %d, want 1",
			reservePreview.Preflight.CurrentVersion)
	}
	var reserveResult pairView
	if err := reservePreview.decodeResult(&reserveResult); err != nil {
		return err
	}
	if err := requireRunState(reserveResult.Run, domain.RunReserved); err != nil {
		return err
	}
	if err := requireVesselState(reserveResult.Vessel, domain.VesselReserved); err != nil {
		return err
	}
	persistedVessel, err := readVessel(api, vessel.ID)
	if err != nil {
		return err
	}
	if err := requireVesselState(persistedVessel, domain.VesselAvailable); err != nil {
		return fmt.Errorf("reserve preflight changed the persisted vessel: %w", err)
	}
	if err := requireRevisionUnchanged(api, baseline.Revision+realWrites); err != nil {
		return err
	}

	reservedResponse, err := api.call("POST", "/v1/runs/"+first.ID+"/reserve",
		map[string]any{"vessel_id": vessel.ID}, ptrInt64(1))
	if err != nil {
		return err
	}
	if err := requireStatus(reservedResponse, 200, "reserve vessel"); err != nil {
		return err
	}
	realWrites++

	occupied, err := runPreflight(api, "POST", "/v1/runs/"+second.ID+"/reserve",
		map[string]any{"vessel_id": vessel.ID}, ptrInt64(1))
	if err != nil {
		return err
	}
	if err := requirePreflightConflict(occupied,
		string(domain.CodeVesselUnavailable), "preflight reserve occupied vessel"); err != nil {
		return err
	}

	startPreview, err := runPreflight(api, "POST", "/v1/runs/"+first.ID+"/start",
		nil, ptrInt64(2))
	if err != nil {
		return err
	}
	if err := requireWouldApply(startPreview, "preflight start"); err != nil {
		return err
	}

	// A concurrent commit lands between the preflight and the real write.
	// The committed write, not the preflight report, decides the outcome.
	abortedResponse, err := api.call("POST", "/v1/runs/"+first.ID+"/abort",
		nil, ptrInt64(2))
	if err != nil {
		return err
	}
	if err := requireStatus(abortedResponse, 200, "abort run"); err != nil {
		return err
	}
	realWrites++

	staleStart, err := api.call("POST", "/v1/runs/"+first.ID+"/start", nil, ptrInt64(2))
	if err != nil {
		return err
	}
	if err := requireCode(staleStart, 412, string(domain.CodeRevisionConflict),
		"start run after concurrent abort"); err != nil {
		return err
	}

	invalidFlag, err := api.call("POST", "/v1/runs/"+second.ID+"/start?preflight=yes",
		nil, ptrInt64(1))
	if err != nil {
		return err
	}
	if err := requireCode(invalidFlag, 400, string(domain.CodeInvalidInput),
		"reject invalid preflight flag"); err != nil {
		return err
	}

	if err := requireRevisionUnchanged(api, baseline.Revision+realWrites); err != nil {
		return err
	}
	return nil
}

func runPreflight(
	api *client,
	method string,
	path string,
	payload any,
	expectedVersion *int64,
) (preflightView, error) {
	raw, err := api.call(method, path+"?preflight=true", payload, expectedVersion)
	if err != nil {
		return preflightView{}, err
	}
	if err := requireStatus(raw, 200, "preflight "+method+" "+path); err != nil {
		return preflightView{}, err
	}
	var view preflightView
	if err := raw.decode(&view); err != nil {
		return preflightView{}, err
	}
	return view, nil
}

func (v preflightView) decodeResult(target any) error {
	if !v.Preflight.WouldApply {
		return fmt.Errorf("preflight has no result to decode")
	}
	if err := json.Unmarshal(v.Result, target); err != nil {
		return fmt.Errorf("decode preflight result: %w", err)
	}
	return nil
}

func requireWouldApply(view preflightView, operation string) error {
	if !view.Preflight.WouldApply {
		return fmt.Errorf("%s: expected the write to apply, got conflict %+v",
			operation, view.Preflight.Conflict)
	}
	if view.Preflight.Conflict != nil {
		return fmt.Errorf("%s: applicable write reported a conflict", operation)
	}
	if len(view.Result) == 0 {
		return fmt.Errorf("%s: applicable write reported no result", operation)
	}
	return nil
}

func requirePreflightConflict(view preflightView, code string, operation string) error {
	if view.Preflight.WouldApply {
		return fmt.Errorf("%s: expected conflict %q, but the write would apply",
			operation, code)
	}
	if view.Preflight.Conflict == nil {
		return fmt.Errorf("%s: expected conflict %q, got none", operation, code)
	}
	if view.Preflight.Conflict.Code != code {
		return fmt.Errorf("%s: expected conflict %q, got %q",
			operation, code, view.Preflight.Conflict.Code)
	}
	if len(view.Result) != 0 {
		return fmt.Errorf("%s: conflicting write reported a result", operation)
	}
	return nil
}

func currentHealth(api *client) (healthView, error) {
	raw, err := api.call("GET", "/healthz", nil, nil)
	if err != nil {
		return healthView{}, err
	}
	if err := requireStatus(raw, 200, "read health"); err != nil {
		return healthView{}, err
	}
	var health healthView
	if err := raw.decode(&health); err != nil {
		return healthView{}, err
	}
	return health, nil
}

func requireRevisionUnchanged(api *client, expected int64) error {
	health, err := currentHealth(api)
	if err != nil {
		return err
	}
	if health.Revision != expected {
		return fmt.Errorf("preflight moved the snapshot revision to %d, want %d",
			health.Revision, expected)
	}
	return nil
}

func requireFormulaCount(api *client, expected int) error {
	raw, err := api.call("GET", "/v1/formulas", nil, nil)
	if err != nil {
		return err
	}
	if err := requireStatus(raw, 200, "list formulas"); err != nil {
		return err
	}
	var listed struct {
		Formulas []formulaView `json:"formulas"`
	}
	if err := raw.decode(&listed); err != nil {
		return err
	}
	if len(listed.Formulas) != expected {
		return fmt.Errorf("preflight created entities: found %d formulas, want %d",
			len(listed.Formulas), expected)
	}
	return nil
}

func readFormula(api *client, id string) (formulaView, error) {
	raw, err := api.call("GET", "/v1/formulas/"+id, nil, nil)
	if err != nil {
		return formulaView{}, err
	}
	if err := requireStatus(raw, 200, "read formula"); err != nil {
		return formulaView{}, err
	}
	var formula formulaView
	if err := raw.decode(&formula); err != nil {
		return formulaView{}, err
	}
	return formula, nil
}

func readVessel(api *client, id string) (vesselView, error) {
	raw, err := api.call("GET", "/v1/vessels/"+id, nil, nil)
	if err != nil {
		return vesselView{}, err
	}
	if err := requireStatus(raw, 200, "read vessel"); err != nil {
		return vesselView{}, err
	}
	var vessel vesselView
	if err := raw.decode(&vessel); err != nil {
		return vesselView{}, err
	}
	return vessel, nil
}

func createCheckRun(api *client, code string, formulaID string) (runView, error) {
	raw, err := api.call("POST", "/v1/runs", createRunPayload(code, formulaID), nil)
	if err != nil {
		return runView{}, err
	}
	if err := requireStatus(raw, 201, "create check run"); err != nil {
		return runView{}, err
	}
	var run runView
	if err := raw.decode(&run); err != nil {
		return runView{}, err
	}
	return run, nil
}
