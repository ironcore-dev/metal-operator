package redfish

import (
	"errors"
	"net/http"

	"github.com/ironcore-dev/metal-operator/redfish-emu/hypervisor"
)

var allowableResetTypes = []string{
	string(hypervisor.ResetOn),
	string(hypervisor.ResetForceOn),
	string(hypervisor.ResetForceOff),
	string(hypervisor.ResetGracefulShutdown),
	string(hypervisor.ResetGracefulRestart),
	string(hypervisor.ResetForceRestart),
	string(hypervisor.ResetPowerCycle),
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	sys, ok := s.lookupSystem(w, r)
	if !ok {
		return
	}
	var req resetRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Base.1.0.MalformedJSON", err.Error())
		return
	}
	rt, err := parseResetType(req.ResetType)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Base.1.0.ActionParameterValueNotInList", err.Error())
		return
	}
	if err := sys.Hyp.Reset(r.Context(), rt); err != nil {
		writeError(w, http.StatusInternalServerError, "Base.1.0.InternalError", err.Error())
		return
	}
	// Redfish actions succeed with 204 No Content when there is no task/body.
	w.WriteHeader(http.StatusNoContent)
}

func parseResetType(v string) (hypervisor.ResetType, error) {
	for _, t := range allowableResetTypes {
		if t == v {
			return hypervisor.ResetType(v), nil
		}
	}
	if v == "" {
		return "", errors.New("ResetType is required")
	}
	return "", errors.New("unsupported ResetType: " + v)
}
