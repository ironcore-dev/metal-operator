package daemon_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	pollingInterval      = 50 * time.Millisecond
	eventuallyTimeout    = 5 * time.Second
	consistentlyDuration = 1 * time.Second
)

func TestDaemon(t *testing.T) {
	SetDefaultConsistentlyPollingInterval(pollingInterval)
	SetDefaultEventuallyPollingInterval(pollingInterval)
	SetDefaultEventuallyTimeout(eventuallyTimeout)
	SetDefaultConsistentlyDuration(consistentlyDuration)
	RegisterFailHandler(Fail)

	RunSpecs(t, "Daemon Suite")
}

var _ = BeforeSuite(func() {
})
