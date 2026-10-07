package httpapi

import (
	"net/http"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/service"
)

type createVesselRequest struct {
	Code      string  `json:"code"`
	CapacityL float64 `json:"capacity_liters"`
}

func registerVesselRoutes(mux *http.ServeMux, app *service.Service) {
	mux.HandleFunc("POST /v1/vessels", func(response http.ResponseWriter, request *http.Request) {
		body, err := decodeJSON[createVesselRequest](response, request)
		if !handleDecode(response, err) {
			return
		}
		preflight, err := preflightRequested(request)
		if !handleDecode(response, err) {
			return
		}
		input := service.CreateVesselInput{
			Code:      body.Code,
			CapacityL: body.CapacityL,
		}
		if preflight {
			report, created := app.PreflightCreateVessel(input)
			writePreflight(response, report, created)
			return
		}
		vessel, err := app.CreateVessel(input)
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusCreated, vessel)
	})

	mux.HandleFunc("GET /v1/vessels", func(response http.ResponseWriter, _ *http.Request) {
		vessels, err := app.ListVessels()
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{"vessels": vessels})
	})

	mux.HandleFunc("GET /v1/vessels/{id}", func(response http.ResponseWriter, request *http.Request) {
		vessel, err := app.GetVessel(request.PathValue("id"))
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, vessel)
	})

	mux.HandleFunc("POST /v1/vessels/{id}/finish-cleaning", func(response http.ResponseWriter, request *http.Request) {
		expected, err := expectedVersion(request)
		if !handleDecode(response, err) {
			return
		}
		preflight, err := preflightRequested(request)
		if !handleDecode(response, err) {
			return
		}
		input := service.FinishCleaningInput{ExpectedVersion: expected}
		if preflight {
			report, updated := app.PreflightFinishCleaning(request.PathValue("id"), input)
			writePreflight(response, report, updated)
			return
		}
		vessel, err := app.FinishCleaning(request.PathValue("id"), input)
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, vessel)
	})

	mux.HandleFunc("POST /v1/vessels/{id}/retire", func(response http.ResponseWriter, request *http.Request) {
		expected, err := expectedVersion(request)
		if !handleDecode(response, err) {
			return
		}
		preflight, err := preflightRequested(request)
		if !handleDecode(response, err) {
			return
		}
		input := service.VersionInput{ExpectedVersion: expected}
		if preflight {
			report, updated := app.PreflightRetireVessel(request.PathValue("id"), input)
			writePreflight(response, report, updated)
			return
		}
		vessel, err := app.RetireVessel(request.PathValue("id"), input)
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, vessel)
	})
}
