# Watch Filter

The metal-operator can run as multiple instances against a single API server,
where each instance owns a disjoint set of resources selected by the
`metal.ironcore.dev/watch-filter` label. The purpose is isolation: a broken or
misbehaving instance can only touch the resources it owns, and an instance
running inside an isolated network (per-AZ traffic, non-routable
console/provisioning networks) never generates BMC/Redfish traffic for
resources owned elsewhere.

The watch filter is label based and applies uniformly to every resource type
the operator watches (`Server`, `ServerClaim`, `BMC`, `BMCSecret`, `Endpoint`,
`ServerBootConfiguration`, ...). There is no automatic assignment, hashing or
rebalancing: the label expresses ownership, nothing more.

## Assigning resources to an instance

A resource belongs to an instance if it carries the label
`metal.ironcore.dev/watch-filter: <value>`.

Assignment is deliberate: whoever enrolls a resource decides its owner.
Resources without the label belong to the default instance. There is no
automatic move or rebalance built in.

## Running instances

Each instance is started with the `--watch-filter` flag:

| Flag value | Owns |
|---|---|
| unset / empty | exactly the resources *without* the watch-filter label |
| `--watch-filter=<value>` | exactly the resources labeled `metal.ironcore.dev/watch-filter=<value>` |

Ownership is disjoint by construction: an instance's informer cache has a
label selector applied, so objects outside its filter never enter its watch
set. An instance without the flag uses a `DoesNotExist` selector on the
watch-filter label and never sees labeled resources.

Example: one production instance plus an experimental one:

```text
prod instance:          (no --watch-filter flag)      -> handles all unlabeled resources
experimental instance:  --watch-filter=experimental   -> handles label value: experimental
```

## Cross-resource references

References never cross ownership boundaries. Because foreign-labeled objects
never enter an instance's cache, following a reference to one surfaces as a
plain `NotFound` and the object is treated as if it does not exist.
Concretely:

- Scheduling only considers `Server`s owned by the same instance as the
  claim; a `ServerClaim` can never bind a server owned by another instance.
- Controllers stamp the watch-filter label onto child resources they create
  (`ServerClaim` → `ServerBootConfiguration`, `Endpoint` → `BMC` and
  `BMCSecret`, `BMCUser` → `BMCSecret`), so children stay visible to the same
  instance as their parent.

## Deploying multiple instances

The default (unfiltered) instance deploys as usual from `config/default`.
Each additional instance is a kustomize overlay that prefixes all objects and
adds the `--watch-filter` flag to the manager container. Prefixing keeps
webhook configurations, services and RBAC objects of different instances from
colliding; kustomize rewrites the internal name references automatically.

```yaml
# overlays/watch-filter-experimental/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - ../../config/default

namePrefix: experimental-

patches:
  - path: manager_watchfilter_patch.yaml
```

```yaml
# overlays/watch-filter-experimental/manager_watchfilter_patch.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: controller-manager
spec:
  template:
    spec:
      containers:
        - name: manager
          args: # replaces the args list, keep flags from the base
            - --leader-elect
            - --watch-filter=experimental
```

Deploy both against the same API server:

```sh
kubectl apply -k config/default                        # owns all unlabeled resources
kubectl apply -k overlays/watch-filter-experimental    # owns label value: experimental
```

Then label the resources the experimental instance should take over:

```sh
kubectl label server my-server metal.ironcore.dev/watch-filter=experimental
```

Each instance is scraped and labeled separately by your metrics setup, so
per-instance health stays observable.

Only idle servers should be moved between instances (by changing the label);
migrating servers with in-flight operations or finalizers is not supported
yet.

When running more than one replica of the *same* instance, use
`--leader-elect`. The leader election ID is prefixed with the watch filter
value, so replicas of one instance elect among themselves while different
instances act independently.
