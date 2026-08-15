package redfish

import "net/http"

func (s *Server) handleServiceRoot(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, serviceRoot{
		ODataType:  "#ServiceRoot.v1_5_0.ServiceRoot",
		ODataID:    "/redfish/v1",
		ID:         "RootService",
		Name:       "Redfish Emulator Service Root",
		RedfishVer: "1.6.0",
		UUID:       s.cfg.ServiceUUID,
		Systems:    ref{ODataID: "/redfish/v1/Systems"},
		Managers:   ref{ODataID: "/redfish/v1/Managers"},
	})
}

func (s *Server) handleSystemsCollection(w http.ResponseWriter, _ *http.Request) {
	members := make([]ref, 0, len(s.cfg.Systems))
	for _, sys := range s.cfg.Systems {
		members = append(members, ref{ODataID: "/redfish/v1/Systems/" + sys.ID})
	}
	writeJSON(w, http.StatusOK, collection{
		ODataType:    "#ComputerSystemCollection.ComputerSystemCollection",
		ODataID:      "/redfish/v1/Systems",
		Name:         "Computer System Collection",
		MembersCount: len(members),
		Members:      members,
	})
}
