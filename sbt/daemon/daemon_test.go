package daemon_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ironcore-dev/ironcore-image/v2/xio"
	"github.com/ironcore-dev/metal-operator/redfish-emu/redfishemutest"
	"github.com/ironcore-dev/metal-operator/sbt/daemon"
	"github.com/ironcore-dev/metal-operator/sbt/inventory"
	"github.com/ironcore-dev/metal-operator/sbt/server"
	"github.com/ironcore-dev/metal-operator/sbt/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"oras.land/oras-go/v2/registry/remote"
)

type bootCallbackServer struct {
	bootedOnce sync.Once
	booted     chan struct{}
	srv        *httptest.Server
}

func (s *bootCallbackServer) Booted() <-chan struct{} {
	return s.booted
}

func (s *bootCallbackServer) URL() string {
	_, port, _ := net.SplitHostPort(s.srv.Listener.Addr().String())
	return fmt.Sprintf("http://10.0.2.2:%s", port) // qemu magic address
}

func startBootCallbackServer() *bootCallbackServer {
	GinkgoHelper()
	s := &bootCallbackServer{
		booted: make(chan struct{}),
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		s.bootedOnce.Do(func() { close(s.booted) })
	})

	ln, err := net.Listen("tcp", "0.0.0.0:0")
	Expect(err).NotTo(HaveOccurred())

	s.srv = &httptest.Server{
		Listener: ln,
		Config:   &http.Server{Handler: handler},
	}
	s.srv.Start()
	DeferCleanup(s.srv.Close)
	return s
}

var _ = Describe("Daemon", func() {
	var (
		redfishSrv *redfishemutest.Server
		d          *daemon.Daemon
	)
	BeforeEach(func(ctx SpecContext) {
		redfishSrv = redfishemutest.Start(ctx, redfishemutest.Options{
			Driver: redfishemutest.DriverQEMU,
		})
		DeferCleanup(redfishSrv.Stop)

		inv := inventory.NewMemory()
		inv.AddHost("foo", &inventory.Host{
			Address:  strings.TrimSuffix(redfishSrv.BaseURL, "redfish/v1"),
			SystemID: "1",
			User:     "foo",
			Password: "bar",
		})

		d = daemon.New(
			"127.0.0.1:0",
			store.NewMemory(),
			inv,
			func(repository string) (*remote.Repository, error) {
				repo, err := remote.NewRepository(repository)
				if err != nil {
					return nil, err
				}

				repo.PlainHTTP = true
				return repo, nil
			},
			map[string]xio.Source{
				"arm64": xio.FileSource(filepath.Join("..", "..", "hack", "linuxaa64.efi.stub")),
			},
			daemon.Options{
				Log: GinkgoLogr.WithName("daemon"),
			},
		)

		daemonCtx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)

		go func() {
			defer GinkgoRecover()
			Expect(d.Start(daemonCtx)).To(Succeed())
		}()

		Eventually(d.Started()).Should(BeClosed())
	})

	It("should boot a server", func(ctx SpecContext) {
		bootCallbackSrv := startBootCallbackServer()

		By("creating a server")
		id, err := d.ServerCreate(ctx, &daemon.ServerConfig{
			Name:    "my-server",
			HostID:  "foo",
			Image:   "localhost:50000/booter:latest",
			Cmdline: "reportURL=" + bootCallbackSrv.URL(),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(id).NotTo(BeEmpty())

		By("booting it")
		Expect(d.ServerBoot(ctx, id)).To(Succeed())

		By("waiting for the server to be booted")
		Eventually(func() (*server.Server, error) {
			return d.ServerGet(ctx, id)
		}).WithTimeout(30*time.Second).Should(HaveField("Status.State", server.StateBooted), func() string {
			serialLog, _ := redfishSrv.SerialLog(ctx)
			return fmt.Sprintf("Server did not boot in time\nSerial Logs:\n%s", serialLog)
		})

		By("waiting for the VM to have booted and to report to the boot server")
		Eventually(bootCallbackSrv.Booted()).WithTimeout(30*time.Second).Should(BeClosed(), func() string {
			serialLog, _ := redfishSrv.SerialLog(ctx)
			return fmt.Sprintf("Server did not call back in time\nSerial Logs:\n%s", serialLog)
		})
	})
})
