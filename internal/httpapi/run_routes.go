package httpapi

import (
	"net/http"
	"time"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/service"
)

type createRunRequest struct {
	Code           string  `json:"code"`
	FormulaID      string  `json:"formula_id"`
	PlannedVolumeL float64 `json:"planned_volume_liters"`
	Notes          string  `json:"notes"`
}

type reserveRunRequest struct {
	VesselID string `json:"vessel_id"`
}

type observationRequest struct {
	Sequence     int       `json:"sequence"`
	Gravity      float64   `json:"gravity"`
	TemperatureC float64   `json:"temperature_c"`
	ObservedAt   time.Time `json:"observed_at"`
	Note         string    `json:"note"`
}

func registerRunRoutes(mux *http.ServeMux, app *service.Service) {
	mux.HandleFunc("POST /v1/runs", func(response http.ResponseWriter, request *http.Request) {
		body, err := decodeJSON[createRunRequest](response, request)
		if !handleDecode(response, err) {
			return
		}
		input := service.CreateRunInput{
			Code:           body.Code,
			FormulaID:      body.FormulaID,
			PlannedVolumeL: body.PlannedVolumeL,
			Notes:          body.Notes,
		}
		preview, err := previewRequested(request)
		if !handleDecode(response, err) {
			return
		}
		if preview {
			writePreview(response, app.PreviewCreateRun(input))
			return
		}
		run, err := app.CreateRun(input)
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusCreated, run)
	})

	mux.HandleFunc("GET /v1/runs", func(response http.ResponseWriter, _ *http.Request) {
		runs, err := app.ListRuns()
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{"runs": runs})
	})

	mux.HandleFunc("GET /v1/runs/{id}", func(response http.ResponseWriter, request *http.Request) {
		run, err := app.GetRun(request.PathValue("id"))
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, run)
	})

	mux.HandleFunc("POST /v1/runs/{id}/reserve", func(response http.ResponseWriter, request *http.Request) {
		body, err := decodeJSON[reserveRunRequest](response, request)
		if !handleDecode(response, err) {
			return
		}
		expected, err := expectedVersion(request)
		if !handleDecode(response, err) {
			return
		}
		preview, err := previewRequested(request)
		if !handleDecode(response, err) {
			return
		}
		input := service.ReserveRunInput{
			ExpectedVersion: expected,
			VesselID:        body.VesselID,
		}
		if preview {
			writePreview(response, app.PreviewReserveRun(request.PathValue("id"), input))
			return
		}
		run, vessel, err := app.ReserveRun(request.PathValue("id"), input)
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{"run": run, "vessel": vessel})
	})

	mux.HandleFunc("POST /v1/runs/{id}/start", func(response http.ResponseWriter, request *http.Request) {
		expected, err := expectedVersion(request)
		if !handleDecode(response, err) {
			return
		}
		preview, err := previewRequested(request)
		if !handleDecode(response, err) {
			return
		}
		input := service.VersionInput{ExpectedVersion: expected}
		if preview {
			writePreview(response, app.PreviewStartRun(request.PathValue("id"), input))
			return
		}
		run, vessel, err := app.StartRun(request.PathValue("id"), input)
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{"run": run, "vessel": vessel})
	})

	mux.HandleFunc("POST /v1/runs/{id}/observations", func(response http.ResponseWriter, request *http.Request) {
		body, err := decodeJSON[observationRequest](response, request)
		if !handleDecode(response, err) {
			return
		}
		preview, err := previewRequested(request)
		if !handleDecode(response, err) {
			return
		}
		input := service.ObservationInput{
			Sequence:     body.Sequence,
			Gravity:      body.Gravity,
			TemperatureC: body.TemperatureC,
			ObservedAt:   body.ObservedAt,
			Note:         body.Note,
		}
		if preview {
			writePreview(response, app.PreviewAppendObservation(request.PathValue("id"), input))
			return
		}
		run, err := app.AppendObservation(request.PathValue("id"), input)
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, run)
	})

	mux.HandleFunc("POST /v1/runs/{id}/conditioning", func(response http.ResponseWriter, request *http.Request) {
		expected, err := expectedVersion(request)
		if !handleDecode(response, err) {
			return
		}
		preview, err := previewRequested(request)
		if !handleDecode(response, err) {
			return
		}
		input := service.VersionInput{ExpectedVersion: expected}
		if preview {
			writePreview(response, app.PreviewBeginConditioning(request.PathValue("id"), input))
			return
		}
		run, err := app.BeginConditioning(request.PathValue("id"), input)
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, run)
	})

	mux.HandleFunc("POST /v1/runs/{id}/complete", func(response http.ResponseWriter, request *http.Request) {
		expected, err := expectedVersion(request)
		if !handleDecode(response, err) {
			return
		}
		preview, err := previewRequested(request)
		if !handleDecode(response, err) {
			return
		}
		input := service.VersionInput{ExpectedVersion: expected}
		if preview {
			writePreview(response, app.PreviewCompleteRun(request.PathValue("id"), input))
			return
		}
		run, vessel, err := app.CompleteRun(request.PathValue("id"), input)
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{"run": run, "vessel": vessel})
	})

	mux.HandleFunc("POST /v1/runs/{id}/abort", func(response http.ResponseWriter, request *http.Request) {
		expected, err := expectedVersion(request)
		if !handleDecode(response, err) {
			return
		}
		preview, err := previewRequested(request)
		if !handleDecode(response, err) {
			return
		}
		input := service.VersionInput{ExpectedVersion: expected}
		if preview {
			writePreview(response, app.PreviewAbortRun(request.PathValue("id"), input))
			return
		}
		run, vessel, err := app.AbortRun(request.PathValue("id"), input)
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{"run": run, "vessel": vessel})
	})
}
