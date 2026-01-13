#!/usr/bin/env bash
#// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
#// SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o nounset
set -o pipefail


REPO_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
KUBECTL=$REPO_ROOT/bin/kubectl

# desired kind cluster name; default is "metal"
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-metal}"

if [[ "$(kind get clusters)" =~ .*"${KIND_CLUSTER_NAME}".* ]]; then
  echo "cluster already exists, moving on"
  exit 0
fi

reg_name='kind-registry'
reg_port="${KIND_REGISTRY_PORT:-5000}"

# create registry container unless it already exists
running="$(docker inspect -f '{{.State.Running}}' "${reg_name}" 2>/dev/null || true)"
if [ "${running}" != 'true' ]; then
  docker run -d --restart=always -p "127.0.0.1:${reg_port}:5000" --name "${reg_name}" registry:2
fi

# create a cluster with the local registry enabled in containerd
cat <<EOF | kind create cluster --name "${KIND_CLUSTER_NAME}" --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry]
    config_path = "/etc/containerd/certs.d"
EOF

# add the registry config to the nodes
REGISTRY_DIR="/etc/containerd/certs.d/localhost:${reg_port}"
node="metal-control-plane"
docker exec "${node}" mkdir -p "${REGISTRY_DIR}"
cat <<EOF | docker exec -i "${node}" cp /dev/stdin "${REGISTRY_DIR}/hosts.toml"
[host."http://${reg_name}:5000"]
EOF

# connect the registry to the cluster network if not already connected
if [ "$(docker inspect -f='{{json .NetworkSettings.Networks.kind}}' "${reg_name}")" = 'null' ]; then
  docker network connect "kind" "${reg_name}"
fi

# Document the local registry
# https://github.com/kubernetes/enhancements/tree/master/keps/sig-cluster-lifecycle/generic/1755-communicating-a-local-registry
cat <<EOF | "${KUBECTL}" apply --namespace=kube-public -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:${reg_port}"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF

"${KUBECTL}" wait node "${KIND_CLUSTER_NAME}-control-plane" --for=condition=ready --timeout=90s
