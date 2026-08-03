# Sharding

The metal-operator can run as multiple instances against a single API server,
where each instance owns a disjoint shard of the resources. This scales BMC and
reconcile load horizontally instead of vertically scaling one manager, and
isolates failures between groups of servers (e.g. per rack or per site).

Sharding is label based and applies uniformly to every resource type the
operator watches (`Server`, `ServerClaim`, `BMC`, `BMCSecret`, `Endpoint`,
`ServerBootConfiguration`, ...).

## Assigning resources to a shard

A resource belongs to a shard if it carries the label
`metal.ironcore.dev/shard: <name>`.

Shard assignment is deliberate: whoever enrolls a resource decides its shard.
Resources without the label belong to the default (unsharded) slice of the
fleet. There is no automatic assignment or rebalancing built in.

## Running instances

Each instance is started with the `--shard` flag:

| Flag value | Owns |
|---|---|
| unset / empty | exactly the resources *without* the shard label |
| `--shard=<name>` | exactly the resources labeled `metal.ironcore.dev/shard=<name>` |

Ownership is disjoint by construction: an instance's informer cache has a
label selector applied, so objects outside its shard never enter its watch
set. An unsharded instance uses a `DoesNotExist` selector on the shard label
and never sees labeled resources.

Example: one production instance plus an experimental one:

```text
prod instance:          (no --shard flag) -> handles all unlabeled resources
experimental instance:  --shard=experimental -> handles shard: experimental
```

## Cross-resource references

References never cross shard boundaries. When a controller follows a reference
to an object carrying a foreign shard label, it treats that object as if it
does not exist. Concretely:

- Scheduling only considers `Server`s of the claim's own shard; a
  `ServerClaim` can never bind to a server of another shard.
- Controllers stamp the shard label onto child resources they create
  (`ServerClaim` → `ServerBootConfiguration`, `Endpoint` → `BMC` and
  `BMCSecret`, `BMCUser` → `BMCSecret`), so children stay visible to the same
  shard instance as their parent.

## Deploying multiple shards

The default (unsharded) instance deploys as usual from `config/default`. Each
additional shard is a kustomize overlay that prefixes all objects and adds
the `--shard` flag to the manager container. Prefixing keeps webhook
configurations, services and RBAC objects of different shards from colliding;
kustomize rewrites the internal name references automatically.

```yaml
# overlays/shard-experimental/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - ../../config/default

namePrefix: experimental-

patches:
  - path: manager_shard_patch.yaml
```

```yaml
# overlays/shard-experimental/manager_shard_patch.yaml
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
            - --shard=experimental
```

Deploy both against the same API server:

```sh
kubectl apply -k config/default                 # owns all unlabeled resources
kubectl apply -k overlays/shard-experimental    # owns shard: experimental
```

Then label the resources the experimental instance should take over:

```sh
kubectl label server my-server metal.ironcore.dev/shard=experimental
```

Each instance is scraped and labeled separately by your metrics setup, so
per-shard health stays observable.

Only idle servers should be moved between shards (by changing the label);
migrating servers with in-flight operations or finalizers is not supported
yet.

When running more than one replica of the *same* shard, use `--leader-elect`.
The leader election ID is prefixed with the shard name, so replicas of one
shard elect among themselves while different shards act independently.
