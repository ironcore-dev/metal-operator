package redfish

import (
	"net/http"

	"github.com/ironcore-dev/metal-operator/redfish-emu/hypervisor"
)

// vmBasePath returns the collection path for a manager's virtual media.
func vmBasePath(managerID string) string {
	return "/redfish/v1/Managers/" + managerID + "/VirtualMedia"
}

func (s *Server) handleVirtualMediaCollection(w http.ResponseWriter, r *http.Request) {
	sys, ok := s.lookupSystem(w, r)
	if !ok {
		return
	}
	base := vmBasePath(sys.ID)
	writeJSON(w, http.StatusOK, collection{
		ODataType:    "#VirtualMediaCollection.VirtualMediaCollection",
		ODataID:      base,
		Name:         "Virtual Media Collection",
		MembersCount: 1,
		Members:      []ref{{ODataID: base + "/" + mediaDeviceID}},
	})
}

// lookupMedia resolves both the manager {id} and the media {mid}. The emulator
// exposes exactly one media device (mediaDeviceID) per manager.
func (s *Server) lookupMedia(w http.ResponseWriter, r *http.Request) (System, bool) {
	sys, ok := s.lookupSystem(w, r)
	if !ok {
		return System{}, false
	}
	if r.PathValue("mid") != mediaDeviceID {
		writeError(w, http.StatusNotFound, "Base.1.0.ResourceNotFound",
			"VirtualMedia "+r.PathValue("mid")+" not found")
		return System{}, false
	}
	return sys, true
}

func (s *Server) renderMedia(w http.ResponseWriter, r *http.Request, sys System) {
	list, err := sys.Hyp.ListMedia(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Base.1.0.InternalError", err.Error())
		return
	}
	var cur hypervisor.MediaState
	cur.ConnectedVia = "NotConnected"
	for _, m := range list {
		if m.DeviceID == mediaDeviceID {
			cur = m
			break
		}
	}
	base := vmBasePath(sys.ID) + "/" + mediaDeviceID
	writeJSON(w, http.StatusOK, virtualMedia{
		ODataType:      "#VirtualMedia.v1_5_0.VirtualMedia",
		ODataID:        base,
		ID:             mediaDeviceID,
		Name:           "Virtual CD",
		MediaTypes:     []string{"CD", "DVD"},
		Image:          cur.Image,
		Inserted:       cur.Inserted,
		WriteProtected: cur.WriteProt,
		ConnectedVia:   cur.ConnectedVia,
		Actions: vmActions{
			Insert: vmAction{Target: base + "/Actions/VirtualMedia.InsertMedia"},
			Eject:  vmAction{Target: base + "/Actions/VirtualMedia.EjectMedia"},
		},
	})
}

func (s *Server) handleVirtualMediaGet(w http.ResponseWriter, r *http.Request) {
	sys, ok := s.lookupMedia(w, r)
	if !ok {
		return
	}
	s.renderMedia(w, r, sys)
}

func (s *Server) handleInsertMedia(w http.ResponseWriter, r *http.Request) {
	sys, ok := s.lookupMedia(w, r)
	if !ok {
		return
	}
	var req insertMediaRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Base.1.0.MalformedJSON", err.Error())
		return
	}
	if req.Image == "" {
		writeError(w, http.StatusBadRequest, "Base.1.0.ActionParameterMissing",
			"Image is required for InsertMedia")
		return
	}
	spec := hypervisor.MediaSpec{
		DeviceID:  mediaDeviceID,
		Image:     req.Image,
		Inserted:  true,
		WriteProt: true,
	}
	if req.WriteProtected != nil {
		spec.WriteProt = *req.WriteProtected
	}
	if err := sys.Hyp.InsertMedia(r.Context(), spec); err != nil {
		writeError(w, http.StatusInternalServerError, "Base.1.0.InternalError", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleEjectMedia(w http.ResponseWriter, r *http.Request) {
	sys, ok := s.lookupMedia(w, r)
	if !ok {
		return
	}
	if err := sys.Hyp.EjectMedia(r.Context(), mediaDeviceID); err != nil {
		writeError(w, http.StatusInternalServerError, "Base.1.0.InternalError", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
