package qemu

import (
	"errors"
	"os/exec"
)

// isExpectedExit reports whether a cmd.Wait error corresponds to a shutdown the
// driver initiated (a kill, or a non-zero exit from -no-reboot after the guest
// halted), as opposed to an abnormal crash worth surfacing as EvCrashed.
//
// QEMU with -no-reboot exits 0 on clean guest shutdown. A forced quit/kill
// yields an *exec.ExitError; we treat those as expected because the driver is
// the one that issued them. Anything that isn't an ExitError (e.g. the binary
// failed to exec) is unexpected.
func isExpectedExit(err error) bool {
	if err == nil {
		return true
	}
	var ee *exec.ExitError
	return errors.As(err, &ee)
}
