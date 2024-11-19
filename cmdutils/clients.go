package cmdutils

import "sigs.k8s.io/controller-runtime/pkg/client"

type Clients struct {
	Source client.Client
	Target client.Client
}
