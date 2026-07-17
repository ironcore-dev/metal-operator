// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package metaldata

import (
	"maps"
	"net/netip"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"k8s.io/client-go/tools/cache"
)

type ServerEntry struct {
	Name     string
	Metadata map[string]string
	ClaimRef *metalv1alpha1.ImmutableObjectReference
}

type Index struct {
	log            logr.Logger
	mu             sync.RWMutex
	hosts          map[netip.Addr]*ServerEntry
	serverIPs      map[string][]netip.Addr
	staticMetadata map[string]string
}

func NewIndex(log logr.Logger, staticMetadata map[string]string) *Index {
	return &Index{
		log:            log,
		hosts:          make(map[netip.Addr]*ServerEntry),
		serverIPs:      make(map[string][]netip.Addr),
		staticMetadata: staticMetadata,
	}
}

func (idx *Index) Lookup(addr netip.Addr) (*ServerEntry, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	h, ok := idx.hosts[addr]
	return h, ok
}

func (idx *Index) EventHandler() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    idx.onAdd,
		UpdateFunc: idx.onUpdate,
		DeleteFunc: idx.onDelete,
	}
}

func (idx *Index) onAdd(obj any) {
	server, ok := obj.(*metalv1alpha1.Server)
	if !ok {
		return
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.syncLocked(server)
}

func (idx *Index) onUpdate(_, newObj any) {
	server, ok := newObj.(*metalv1alpha1.Server)
	if !ok {
		return
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.removeLocked(server.Name)
	idx.syncLocked(server)
}

func (idx *Index) onDelete(obj any) {
	server, ok := obj.(*metalv1alpha1.Server)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			return
		}
		server, ok = tombstone.Obj.(*metalv1alpha1.Server)
		if !ok {
			return
		}
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.removeLocked(server.Name)
}

func (idx *Index) syncLocked(server *metalv1alpha1.Server) {
	entry := &ServerEntry{
		Name:     server.Name,
		Metadata: make(map[string]string, len(idx.staticMetadata)),
	}

	maps.Copy(entry.Metadata, idx.staticMetadata)
	entry.Metadata["server-name"] = server.Name

	for k, v := range server.Annotations {
		key, ok := strings.CutPrefix(k, metalv1alpha1.MetadataKeyPrefix)
		if ok {
			entry.Metadata[key] = v
		}
	}
	for k, v := range server.Labels {
		key, ok := strings.CutPrefix(k, metalv1alpha1.MetadataKeyPrefix)
		if ok {
			entry.Metadata[key] = v
		}
	}

	if server.Spec.ServerClaimRef != nil {
		ref := *server.Spec.ServerClaimRef
		entry.ClaimRef = &ref
	}

	var addrs []netip.Addr
	for _, nic := range server.Status.NetworkInterfaces {
		for _, ip := range nic.IPs {
			addr := ip.Addr
			if !addr.IsValid() {
				continue
			}
			if existing, ok := idx.hosts[addr]; ok && existing.Name != server.Name {
				idx.log.Info("IP claimed by multiple Servers, overwriting index entry",
					"ip", addr, "previous", existing.Name, "current", server.Name)
			}
			idx.hosts[addr] = entry
			addrs = append(addrs, addr)
		}
	}
	idx.serverIPs[server.Name] = addrs

	idx.log.Info("Indexed Server", "name", server.Name, "ips", len(addrs))
}

func (idx *Index) removeLocked(name string) {
	addrs, ok := idx.serverIPs[name]
	if !ok {
		return
	}
	for _, addr := range addrs {
		delete(idx.hosts, addr)
	}
	delete(idx.serverIPs, name)
}
