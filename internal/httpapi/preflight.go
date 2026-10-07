package httpapi

import (
	"net/http"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/service"
)

type preflightBody struct {
	CurrentVersion   int64      `json:"current_version"`
	SnapshotRevision int64      `json:"snapshot_revision"`
	WouldApply       bool       `json:"would_apply"`
	Conflict         *errorBody `json:"conflict,omitempty"`
}

type preflightResponse struct {
	Preflight preflightBody `json:"preflight"`
	Result    any           `json:"result,omitempty"`
}

// preflightRequested reports whether the write should only be rehearsed.
// The flag is opt-in and parsed strictly so a misspelled value cannot
// silently turn into a committed write.
func preflightRequested(request *http.Request) (bool, error) {
	switch value := request.URL.Query().Get("preflight"); value {
	case "", "false":
		return false, nil
	case "true":
		return true, nil
	default:
		return false, domain.FieldError(domain.CodeInvalidInput, "preflight",
			"must be \"true\" or \"false\" when provided")
	}
}

// writePreflight answers a rehearsed write. The preflight itself is a
// read-only operation, so it always succeeds with 200: the conflict the
// committed write would return is reported as data, and the would-be
// result is included only when the write would apply.
func writePreflight(response http.ResponseWriter, report service.PreflightReport, result any) {
	body := preflightResponse{
		Preflight: preflightBody{
			CurrentVersion:   report.CurrentVersion,
			SnapshotRevision: report.Revision,
			WouldApply:       report.WouldApply,
		},
	}
	if report.Conflict != nil {
		body.Preflight.Conflict = &errorBody{
			Code:   report.Conflict.Code,
			Detail: report.Conflict.Message,
			Field:  report.Conflict.Field,
		}
	}
	if report.WouldApply {
		body.Result = result
	}
	writeJSON(response, http.StatusOK, body)
}
