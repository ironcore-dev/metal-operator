package redfish

import (
	"net/http"

	"github.com/ironcore-dev/metal-operator/redfish-emu/hypervisor"
)

// System couples a Redfish System id with the Hypervisor that backs it and the
// display name shown in resources.
type System struct {
	ID   string
	Name string
	Hyp  hypervisor.Hypervisor
}

// Config configures a Server.
type Config struct {
	// Systems is the set of managed machines. At least one is required.
	Systems []System
	// ServiceUUID is the ServiceRoot UUID (optional).
	ServiceUUID string
}

// Server is the Redfish north-side HTTP handler. It depends only on the
// hypervisor.Hypervisor abstraction via its configured Systems.
type Server struct {
	cfg     Config
	systems map[string]System
	mux     *http.ServeMux
}

// mediaDeviceID is the single virtual media device id the emulator exposes per
// manager. Managers are 1:1 with systems and share the system's id.
const mediaDeviceID = "Cd"

// NewServer builds a Server from cfg. It panics if no systems are configured,
// which is a programming error at wiring time.
func NewServer(cfg Config) *Server {
	if len(cfg.Systems) == 0 {
		panic("redfish: NewServer requires at least one system")
	}
	s := &Server{
		cfg:     cfg,
		systems: make(map[string]System, len(cfg.Systems)),
	}
	for _, sys := range cfg.Systems {
		s.systems[sys.ID] = sys
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /redfish/v1", s.handleServiceRoot)
	mux.HandleFunc("GET /redfish/v1/", s.handleServiceRoot)

	mux.HandleFunc("GET /redfish/v1/Systems", s.handleSystemsCollection)
	mux.HandleFunc("GET /redfish/v1/Systems/{id}", s.handleSystemGet)
	mux.HandleFunc("PATCH /redfish/v1/Systems/{id}", s.handleSystemPatch)
	mux.HandleFunc("POST /redfish/v1/Systems/{id}/Actions/ComputerSystem.Reset", s.handleReset)

	mux.HandleFunc("GET /redfish/v1/Managers", s.handleManagersCollection)
	mux.HandleFunc("GET /redfish/v1/Managers/{id}", s.handleManagerGet)
	mux.HandleFunc("GET /redfish/v1/Managers/{id}/VirtualMedia", s.handleVirtualMediaCollection)
	mux.HandleFunc("GET /redfish/v1/Managers/{id}/VirtualMedia/{mid}", s.handleVirtualMediaGet)
	mux.HandleFunc("POST /redfish/v1/Managers/{id}/VirtualMedia/{mid}/Actions/VirtualMedia.InsertMedia", s.handleInsertMedia)
	mux.HandleFunc("POST /redfish/v1/Managers/{id}/VirtualMedia/{mid}/Actions/VirtualMedia.EjectMedia", s.handleEjectMedia)

	s.mux = mux
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// lookupSystem resolves the {id} path value, writing a 404 envelope and
// returning ok=false if unknown.
func (s *Server) lookupSystem(w http.ResponseWriter, r *http.Request) (System, bool) {
	id := r.PathValue("id")
	sys, ok := s.systems[id]
	if !ok {
		writeError(w, http.StatusNotFound, "Base.1.0.ResourceNotFound", "System "+id+" not found")
		return System{}, false
	}
	return sys, true
}
