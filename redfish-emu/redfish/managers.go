package redfish

import "net/http"

func (s *Server) handleManagersCollection(w http.ResponseWriter, _ *http.Request) {
	members := make([]ref, 0, len(s.cfg.Systems))
	for _, sys := range s.cfg.Systems {
		members = append(members, ref{ODataID: "/redfish/v1/Managers/" + sys.ID})
	}
	writeJSON(w, http.StatusOK, collection{
		ODataType:    "#ManagerCollection.ManagerCollection",
		ODataID:      "/redfish/v1/Managers",
		Name:         "Manager Collection",
		MembersCount: len(members),
		Members:      members,
	})
}

func (s *Server) handleManagerGet(w http.ResponseWriter, r *http.Request) {
	sys, ok := s.lookupSystem(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, manager{
		ODataType:    "#Manager.v1_10_0.Manager",
		ODataID:      "/redfish/v1/Managers/" + sys.ID,
		ID:           sys.ID,
		Name:         "BMC for " + sys.Name,
		ManagerType:  "BMC",
		PowerState:   "On",
		VirtualMedia: ref{ODataID: "/redfish/v1/Managers/" + sys.ID + "/VirtualMedia"},
	})
}
