package boot

import (
	"context"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/task"
)

type PowerState string

const (
	PowerStateUnknown     PowerState = "Unknown"
	PowerStateOn          PowerState = "On"
	PowerStateOff         PowerState = "Off"
	PowerStatePoweringOn  PowerState = "PoweringOn"
	PowerStatePoweringOff PowerState = "PoweringOff"
)

type PowerControl interface {
	Rebooter
	// ApplyPower applies the given power state, calling mutate if transitioning and if mutate is non-nil.
	ApplyPower(ctx context.Context, server *metalv1alpha1.Server, powerState bool, mutate func() error) (task.Result, error)
}

type Rebooter interface {
	// Reboot issues a reboot on the target server. It does not wait for the reboot to actually have happened
	// or be completed. It only ensures that the reboot will be done.
	Reboot(ctx context.Context, server *metalv1alpha1.Server) error
}
