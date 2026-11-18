package config

type Config struct {
	// BaseURL is the base URL to the daemon server.
	BaseURL string `json:"baseURL"`

	// Token is the token used to authenticate.
	Token string `json:"token"`

	// ConfigDriveURL is the url pointing at the config drive.
	ConfigDriveURL string `json:"configDriveURL"`

	// ImageRef references the image to write.
	ImageRef string `json:"imageRef"`

	// Device to write disk to (/dev/sdX); empty = auto-select via hints
	Device string `json:"device"`

	// Hints are JSON hints for the disk selection.
	Hints *RootDeviceHints `json:"hints,omitempty"`

	// MinGB is the minimum disk size in GiB for auto-selection.
	MinGB int64 `json:"minGB"`

	// SourceDevice is the block device the config drive was loaded from
	// (e.g. "/dev/sda"). Populated at runtime by LoadConfig, it is never part
	// of the serialized config. Device auto-selection must never install to
	// the config drive itself, so ResolveDevice excludes it.
	SourceDevice string `json:"-"`
}

// RootDeviceHints — subset of Ironic's root_device hints (operator grammar
// like "s>= 100" / "<or>" not implemented; see README note in previous
// version).
type RootDeviceHints struct {
	Name       string `json:"name"`
	Model      string `json:"model"`
	Vendor     string `json:"vendor"`
	Serial     string `json:"serial"`
	WWN        string `json:"wwn"`
	Rotational *bool  `json:"rotational"`
	SizeGB     int64  `json:"size"`
	MinSizeGB  int64  `json:"min_size"`
}

func (h *RootDeviceHints) IsZero() bool {
	return h == nil || *h == RootDeviceHints{}
}
