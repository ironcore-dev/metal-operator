package redfish

import k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

type Driver struct {
	dialer Dialer

	Boot        *BootControl
	Power       *PowerControl
	ConfigDrive *ConfigDriveControl
}

func New(client k8sclient.Client, dialer Dialer) *Driver {
	return &Driver{
		dialer:      dialer,
		Boot:        NewBootControl(client, dialer),
		Power:       NewPowerControl(client, dialer),
		ConfigDrive: NewConfigDriveControl(client, dialer),
	}
}
