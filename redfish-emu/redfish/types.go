package redfish

// This file holds the JSON DTOs the server emits and accepts. They cover only
// the subset of Redfish the emulator implements today; OData annotations are
// hand-authored. Fields use pointers or omitempty where Redfish distinguishes
// "absent" from "zero".

// serviceRoot is GET /redfish/v1.
type serviceRoot struct {
	ODataType  string `json:"@odata.type"`
	ODataID    string `json:"@odata.id"`
	ID         string `json:"Id"`
	Name       string `json:"Name"`
	RedfishVer string `json:"RedfishVersion"`
	UUID       string `json:"UUID,omitempty"`
	Systems    ref    `json:"Systems"`
	Managers   ref    `json:"Managers"`
}

// ref is an OData reference ({"@odata.id": "..."}).
type ref struct {
	ODataID string `json:"@odata.id"`
}

// collection is a generic Redfish collection resource.
type collection struct {
	ODataType    string `json:"@odata.type"`
	ODataID      string `json:"@odata.id"`
	Name         string `json:"Name"`
	MembersCount int    `json:"Members@odata.count"`
	Members      []ref  `json:"Members"`
}

// bootDTO is the Boot object embedded in a ComputerSystem.
type bootDTO struct {
	BootSourceOverrideEnabled         string   `json:"BootSourceOverrideEnabled"`
	BootSourceOverrideTarget          string   `json:"BootSourceOverrideTarget"`
	BootSourceOverrideTargetAllowable []string `json:"BootSourceOverrideTarget@Redfish.AllowableValues"`
	BootSourceOverrideMode            string   `json:"BootSourceOverrideMode,omitempty"`
	HTTPBootURI                       string   `json:"HttpBootUri,omitempty"`
}

// computerSystem is GET /redfish/v1/Systems/{id}.
type computerSystem struct {
	ODataType  string     `json:"@odata.type"`
	ODataID    string     `json:"@odata.id"`
	ID         string     `json:"Id"`
	Name       string     `json:"Name"`
	PowerState string     `json:"PowerState"`
	Boot       bootDTO    `json:"Boot"`
	Actions    sysActions `json:"Actions"`
}

type sysActions struct {
	Reset resetAction `json:"#ComputerSystem.Reset"`
}

type resetAction struct {
	Target          string   `json:"target"`
	ResetTypeValues []string `json:"ResetType@Redfish.AllowableValues"`
}

// systemPatch is the accepted body of PATCH /redfish/v1/Systems/{id}. Only the
// Boot object is honored; pointers distinguish "field omitted" from "set to
// empty".
type systemPatch struct {
	Boot *bootPatch `json:"Boot"`
}

type bootPatch struct {
	BootSourceOverrideEnabled *string `json:"BootSourceOverrideEnabled"`
	BootSourceOverrideTarget  *string `json:"BootSourceOverrideTarget"`
	BootSourceOverrideMode    *string `json:"BootSourceOverrideMode"`
	HTTPBootURI               *string `json:"HttpBootUri"`
}

// resetRequest is the body of ComputerSystem.Reset.
type resetRequest struct {
	ResetType string `json:"ResetType"`
}

// manager is GET /redfish/v1/Managers/{id} (the BMC).
type manager struct {
	ODataType    string `json:"@odata.type"`
	ODataID      string `json:"@odata.id"`
	ID           string `json:"Id"`
	Name         string `json:"Name"`
	ManagerType  string `json:"ManagerType"`
	PowerState   string `json:"PowerState"`
	VirtualMedia ref    `json:"VirtualMedia"`
}

// virtualMedia is GET /redfish/v1/Managers/{id}/VirtualMedia/{mid}.
type virtualMedia struct {
	ODataType      string    `json:"@odata.type"`
	ODataID        string    `json:"@odata.id"`
	ID             string    `json:"Id"`
	Name           string    `json:"Name"`
	MediaTypes     []string  `json:"MediaTypes"`
	Image          string    `json:"Image"`
	Inserted       bool      `json:"Inserted"`
	WriteProtected bool      `json:"WriteProtected"`
	ConnectedVia   string    `json:"ConnectedVia"`
	Actions        vmActions `json:"Actions"`
}

type vmActions struct {
	Insert vmAction `json:"#VirtualMedia.InsertMedia"`
	Eject  vmAction `json:"#VirtualMedia.EjectMedia"`
}

type vmAction struct {
	Target string `json:"target"`
}

// insertMediaRequest is the body of VirtualMedia.InsertMedia.
type insertMediaRequest struct {
	Image          string `json:"Image"`
	Inserted       *bool  `json:"Inserted"`
	WriteProtected *bool  `json:"WriteProtected"`
}
