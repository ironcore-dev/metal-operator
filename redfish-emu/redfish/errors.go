package redfish

import (
	"encoding/json"
	"net/http"
)

// errorBody is the DMTF Redfish error envelope returned on failures.
type errorBody struct {
	Error struct {
		Code         string         `json:"code"`
		Message      string         `json:"message"`
		ExtendedInfo []extendedInfo `json:"@Message.ExtendedInfo,omitempty"`
	} `json:"error"`
}

type extendedInfo struct {
	MessageID  string `json:"MessageId"`
	Message    string `json:"Message"`
	Severity   string `json:"Severity"`
	Resolution string `json:"Resolution"`
}

// writeError renders a DMTF error envelope with the given HTTP status. code is
// the top-level Redfish message id (e.g. "Base.1.0.GeneralError"); msg is a
// human-readable description.
func writeError(w http.ResponseWriter, status int, code, msg string) {
	var b errorBody
	b.Error.Code = code
	b.Error.Message = msg
	b.Error.ExtendedInfo = []extendedInfo{{
		MessageID: code,
		Message:   msg,
		Severity:  severityFor(status),
	}}
	writeJSON(w, status, b)
}

func severityFor(status int) string {
	switch {
	case status >= 500:
		return "Critical"
	case status >= 400:
		return "Warning"
	default:
		return "OK"
	}
}

// writeJSON marshals v as the JSON response body with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("OData-Version", "4.0")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
