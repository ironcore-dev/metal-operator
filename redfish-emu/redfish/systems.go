package redfish

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/ironcore-dev/metal-operator/redfish-emu/hypervisor"
)

// allowableBootTargets are the BootSourceOverrideTarget values the emulator
// advertises and accepts.
var allowableBootTargets = []string{
	string(hypervisor.BootNone),
	string(hypervisor.BootPxe),
	string(hypervisor.BootHdd),
	string(hypervisor.BootCd),
	string(hypervisor.BootUsb),
	string(hypervisor.BootUefiHttp),
}

// allowableBootModes are the BootSourceOverrideMode values the emulator accepts.
var allowableBootModes = []string{
	string(hypervisor.BootModeLegacy),
	string(hypervisor.BootModeUEFI),
}

func (s *Server) renderSystem(w http.ResponseWriter, r *http.Request, sys System) {
	st, err := sys.Hyp.Status(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Base.1.0.InternalError", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, computerSystem{
		ODataType:  "#ComputerSystem.v1_13_0.ComputerSystem",
		ODataID:    "/redfish/v1/Systems/" + sys.ID,
		ID:         sys.ID,
		Name:       sys.Name,
		PowerState: string(st.Power),
		Boot: bootDTO{
			BootSourceOverrideEnabled:         string(st.Boot.Enabled),
			BootSourceOverrideTarget:          string(st.Boot.Target),
			BootSourceOverrideTargetAllowable: allowableBootTargets,
			BootSourceOverrideMode:            string(st.Boot.Mode),
			HTTPBootURI:                       st.Boot.HTTPBootURI,
		},
		Actions: sysActions{Reset: resetAction{
			Target:          "/redfish/v1/Systems/" + sys.ID + "/Actions/ComputerSystem.Reset",
			ResetTypeValues: allowableResetTypes,
		}},
	})
}

func (s *Server) handleSystemGet(w http.ResponseWriter, r *http.Request) {
	sys, ok := s.lookupSystem(w, r)
	if !ok {
		return
	}
	s.renderSystem(w, r, sys)
}

func (s *Server) handleSystemPatch(w http.ResponseWriter, r *http.Request) {
	sys, ok := s.lookupSystem(w, r)
	if !ok {
		return
	}
	var patch systemPatch
	if err := decodeJSON(r.Body, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "Base.1.0.MalformedJSON", err.Error())
		return
	}
	if patch.Boot == nil {
		// Nothing we honor changed; return the current representation.
		s.renderSystem(w, r, sys)
		return
	}

	// Start from the currently recorded override and apply only present fields.
	cur, err := sys.Hyp.GetBootOverride(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Base.1.0.InternalError", err.Error())
		return
	}
	if v := patch.Boot.BootSourceOverrideEnabled; v != nil {
		mode, err := parseOverrideMode(*v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Base.1.0.PropertyValueNotInList", err.Error())
			return
		}
		cur.Enabled = mode
	}
	if v := patch.Boot.BootSourceOverrideTarget; v != nil {
		target, err := parseBootTarget(*v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Base.1.0.PropertyValueNotInList", err.Error())
			return
		}
		cur.Target = target
	}
	if v := patch.Boot.BootSourceOverrideMode; v != nil {
		mode, err := parseBootMode(*v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Base.1.0.PropertyValueNotInList", err.Error())
			return
		}
		cur.Mode = mode
	}
	if v := patch.Boot.HTTPBootURI; v != nil {
		cur.HTTPBootURI = *v
	}
	if cur.Target == hypervisor.BootUefiHttp && cur.HTTPBootURI == "" {
		writeError(w, http.StatusBadRequest, "Base.1.0.PropertyMissing",
			"HttpBootUri is required when BootSourceOverrideTarget is UefiHttp")
		return
	}

	if err := sys.Hyp.SetBootOverride(r.Context(), cur); err != nil {
		writeError(w, http.StatusInternalServerError, "Base.1.0.InternalError", err.Error())
		return
	}
	s.renderSystem(w, r, sys)
}

func parseOverrideMode(v string) (hypervisor.OverrideMode, error) {
	switch hypervisor.OverrideMode(v) {
	case hypervisor.OverrideDisabled:
		return hypervisor.OverrideDisabled, nil
	case hypervisor.OverrideOnce:
		return hypervisor.OverrideOnce, nil
	case hypervisor.OverrideContinuous:
		return hypervisor.OverrideContinuous, nil
	default:
		return "", errors.New("invalid BootSourceOverrideEnabled: " + v)
	}
}

func parseBootTarget(v string) (hypervisor.BootTarget, error) {
	for _, t := range allowableBootTargets {
		if t == v {
			return hypervisor.BootTarget(v), nil
		}
	}
	return "", errors.New("invalid BootSourceOverrideTarget: " + v)
}

func parseBootMode(v string) (hypervisor.BootMode, error) {
	for _, m := range allowableBootModes {
		if m == v {
			return hypervisor.BootMode(v), nil
		}
	}
	return "", errors.New("invalid BootSourceOverrideMode: " + v)
}

// decodeJSON strictly decodes a single JSON object from r into v.
func decodeJSON(r io.Reader, v any) error {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	return nil
}
