// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package metaldata_test

import (
	"net/netip"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

var _ = Describe("Index", func() {
	var server *metalv1alpha1.Server

	addServer := func(s *metalv1alpha1.Server) {
		idx.EventHandler().AddFunc(s)
	}
	deleteServer := func(s *metalv1alpha1.Server) {
		idx.EventHandler().DeleteFunc(s)
	}
	updateServer := func(oldS, newS *metalv1alpha1.Server) {
		idx.EventHandler().UpdateFunc(oldS, newS)
	}

	BeforeEach(func() {
		server = newServer("server-a", "192.0.2.10")
		server.Annotations = map[string]string{
			"unrelated": "ignored",
			metalv1alpha1.MetadataKeyPrefix + "owner": "alice",
			metalv1alpha1.MetadataKeyPrefix + "rack":  "from-annotation",
		}
		server.Labels = map[string]string{
			"unrelated":                              "ignored",
			metalv1alpha1.MetadataKeyPrefix + "rack": "from-label",
		}
		addServer(server)
	})

	AfterEach(func() {
		deleteServer(server)
	})

	It("returns the entry for an indexed IP", func() {
		entry, ok := idx.Lookup(netip.MustParseAddr("192.0.2.10"))
		Expect(ok).To(BeTrue())
		Expect(entry.Name).To(Equal("server-a"))
	})

	It("returns nothing for an unindexed IP", func() {
		_, ok := idx.Lookup(netip.MustParseAddr("198.51.100.1"))
		Expect(ok).To(BeFalse())
	})

	It("merges static metadata, annotations and labels into the entry", func() {
		entry, ok := idx.Lookup(netip.MustParseAddr("192.0.2.10"))
		Expect(ok).To(BeTrue())
		Expect(entry.Metadata).To(HaveKeyWithValue("owner", "alice"))
		Expect(entry.Metadata).To(HaveKeyWithValue(staticKey, staticVal))
	})

	It("strips the metadata.metal.ironcore.dev/ prefix", func() {
		entry, _ := idx.Lookup(netip.MustParseAddr("192.0.2.10"))
		Expect(entry.Metadata).NotTo(HaveKey(metalv1alpha1.MetadataKeyPrefix + "owner"))
		Expect(entry.Metadata).To(HaveKey("owner"))
	})

	It("excludes annotations and labels without the prefix", func() {
		entry, _ := idx.Lookup(netip.MustParseAddr("192.0.2.10"))
		Expect(entry.Metadata).NotTo(HaveKey("unrelated"))
	})

	It("lets labels override annotations on the same key", func() {
		entry, _ := idx.Lookup(netip.MustParseAddr("192.0.2.10"))
		Expect(entry.Metadata).To(HaveKeyWithValue("rack", "from-label"))
	})

	It("lets annotations override static metadata on the same key", func() {
		deleteServer(server)
		server = newServer("server-a", "192.0.2.10")
		server.Annotations = map[string]string{
			metalv1alpha1.MetadataKeyPrefix + staticKey: "from-annotation",
		}
		addServer(server)

		entry, _ := idx.Lookup(netip.MustParseAddr("192.0.2.10"))
		Expect(entry.Metadata).To(HaveKeyWithValue(staticKey, "from-annotation"))
	})

	It("includes a server-name key matching the Server's name, overriding any static value", func() {
		entry, _ := idx.Lookup(netip.MustParseAddr("192.0.2.10"))
		Expect(entry.Metadata).To(HaveKeyWithValue("server-name", "server-a"))
	})

	It("forwards the ServerClaimRef onto the entry", func() {
		deleteServer(server)
		server = newServer("server-a", "192.0.2.10")
		server.Spec.ServerClaimRef = &metalv1alpha1.ImmutableObjectReference{
			Namespace: "default",
			Name:      "claim-a",
		}
		addServer(server)

		entry, _ := idx.Lookup(netip.MustParseAddr("192.0.2.10"))
		Expect(entry.ClaimRef).NotTo(BeNil())
		Expect(*entry.ClaimRef).To(Equal(metalv1alpha1.ImmutableObjectReference{
			Namespace: "default",
			Name:      "claim-a",
		}))
	})

	When("a Server is updated with a new IP set", func() {
		It("removes the old IPs and indexes the new ones", func() {
			updated := newServer("server-a", "192.0.2.20")
			updateServer(server, updated)
			server = updated

			_, ok := idx.Lookup(netip.MustParseAddr("192.0.2.10"))
			Expect(ok).To(BeFalse())

			entry, ok := idx.Lookup(netip.MustParseAddr("192.0.2.20"))
			Expect(ok).To(BeTrue())
			Expect(entry.Name).To(Equal("server-a"))
		})
	})

	When("a Server is deleted", func() {
		It("removes all of its IPs from the index", func() {
			deleteServer(server)
			server = newServer("placeholder", "192.0.2.10")

			_, ok := idx.Lookup(netip.MustParseAddr("192.0.2.10"))
			Expect(ok).To(BeFalse())

			addServer(server)
		})
	})

	When("a Server is deleted via a tombstone", func() {
		It("removes all of its IPs from the index", func() {
			tombstone := cache.DeletedFinalStateUnknown{
				Key: server.Namespace + "/" + server.Name,
				Obj: server,
			}
			idx.EventHandler().DeleteFunc(tombstone)

			_, ok := idx.Lookup(netip.MustParseAddr("192.0.2.10"))
			Expect(ok).To(BeFalse())

			server = newServer("server-a", "192.0.2.10")
			addServer(server)
		})
	})

	When("a Server has IPs that fail to parse", func() {
		It("skips them without indexing", func() {
			deleteServer(server)
			server = &metalv1alpha1.Server{
				ObjectMeta: metav1.ObjectMeta{Name: "server-a"},
				Status: metalv1alpha1.ServerStatus{
					NetworkInterfaces: []metalv1alpha1.NetworkInterface{
						{IPs: []metalv1alpha1.IP{{}}},
					},
				},
			}
			addServer(server)

			_, ok := idx.Lookup(netip.Addr{})
			Expect(ok).To(BeFalse())
		})
	})
})

func newServer(name string, ips ...string) *metalv1alpha1.Server {
	parsed := make([]metalv1alpha1.IP, 0, len(ips))
	for _, ip := range ips {
		parsed = append(parsed, metalv1alpha1.MustParseIP(ip))
	}
	return &metalv1alpha1.Server{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: metalv1alpha1.ServerStatus{
			NetworkInterfaces: []metalv1alpha1.NetworkInterface{
				{Name: "eth0", IPs: parsed},
			},
		},
	}
}
