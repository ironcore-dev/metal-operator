package server

type Server struct {
	ID       string
	Name     string
	HostID   string
	Image    string
	ImageRef string
	Cmdline  string
	Status   Status
}

type Status struct {
	State State
}

type State string

const (
	StateCreated  State = "Created"
	StateStarting State = "Starting"
	StateBooted   State = "Booted"
)
