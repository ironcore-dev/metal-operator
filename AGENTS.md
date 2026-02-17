## Purpose

This document defines review guidelines for automated and human reviewers of this repository.

The project is a **Kubernetes controller** built using **Kubebuilder** and **controller-runtime**. 
Reviews must prioritize correctness, maintainability, and alignment with Kubernetes and Go community standards.

---

## Primary References

Reviewers **must** be familiar with and evaluate changes against the following:

* Kubernetes API Conventions
  [https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)

* Kubebuilder Book
  [https://book.kubebuilder.io/](https://book.kubebuilder.io/)

* controller-runtime documentation
  [https://pkg.go.dev/sigs.k8s.io/controller-runtime](https://pkg.go.dev/sigs.k8s.io/controller-runtime)

* Go Code Review Comments
  [https://github.com/golang/go/wiki/CodeReviewComments](https://github.com/golang/go/wiki/CodeReviewComments)

---

<<<<<<< HEAD
```go
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=".status.conditions[?(@.type=='Ready')].status"

// On fields:
// +kubebuilder:validation:Required
// +kubebuilder:validation:Minimum=1
// +kubebuilder:validation:MaxLength=100
// +kubebuilder:validation:Pattern="^[a-z]+$"
// +kubebuilder:default="value"
```

- **Use** `metav1.Condition` for status (not custom string fields)
- **Use predefined types**: `metav1.Time` instead of `string` for dates
- **Follow K8s API conventions**: Standard field names (`spec`, `status`, `metadata`)

### Controller Design

**RBAC markers in** `internal/controller/*_controller.go`:

```go
// +kubebuilder:rbac:groups=mygroup.example.com,resources=mykinds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mygroup.example.com,resources=mykinds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=mygroup.example.com,resources=mykinds/finalizers,verbs=update
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
```

**Implementation rules:**
- **Idempotent reconciliation**: Safe to run multiple times
- **Re-fetch before updates**: `r.Get(ctx, req.NamespacedName, obj)` before `r.Update` to avoid conflicts
- **Structured logging**: `log := log.FromContext(ctx); log.Info("msg", "key", val)`
- **Owner references**: Enable automatic garbage collection (`SetControllerReference`)
- **Watch secondary resources**: Use `.Owns()` or `.Watches()`, not just `RequeueAfter`
- **Finalizers**: Clean up external resources (buckets, VMs, DNS entries)

### Logging

**Follow Kubernetes logging message style guidelines:**

- Start from a capital letter
- Do not end the message with a period
- Active voice: subject present (`"Deployment could not create Pod"`) or omitted (`"Could not create Pod"`)
- Past tense: `"Could not delete Pod"` not `"Cannot delete Pod"`
- Specify object type: `"Deleted Pod"` not `"Deleted"`
- Balanced key-value pairs

```go
log.Info("Starting reconciliation")
log.Info("Created Deployment", "name", deploy.Name)
log.Error(err, "Failed to create Pod", "name", name)
```

**Reference:** https://github.com/kubernetes/community/blob/master/contributors/devel/sig-instrumentation/logging.md#message-style-guidelines

### Webhooks
- **Create all types together**: `--defaulting --programmatic-validation --conversion`
- **When`--force`is used**: Backup custom logic first, then restore after scaffolding
- **For multi-version APIs**: Use hub-and-spoke pattern (`--conversion --spoke v2`)
  - Hub version: Usually oldest stable version (v1)
  - Spoke versions: Newer versions that convert to/from hub (v2, v3)
  - Example: `--group crew --version v1 --kind Captain --conversion --spoke v2` (v1 is hub, v2 is spoke)

### Learning from Examples

The **deploy-image plugin** scaffolds a complete controller following good practices. Use it as a reference implementation:

```bash
kubebuilder create api --group example --version v1alpha1 --kind MyApp \
  --image=<your-image> --plugins=deploy-image.go.kubebuilder.io/v1-alpha
```
=======
## High-Level Review Principles
>>>>>>> tmp-original-17-02-26-00-42

* Prefer **declarative**, **idempotent**, and **level-based** reconciliation logic
* Follow Kubernetes API and controller patterns rather than inventing new abstractions
* Optimize for **clarity and correctness over cleverness**
* Ensure behavior is predictable under retries, restarts, and partial failures

---

## Kubernetes API Review Checklist

### API Types (CRDs)

Reviewers should verify:

<<<<<<< HEAD
```bash
kubebuilder edit --plugins=helm/v2-alpha                      # Generates dist/chart/ (default)
kubebuilder edit --plugins=helm/v2-alpha --output-dir=charts  # Generates charts/chart/
```

**For development:**
```bash
make helm-deploy IMG=<registry>/<project>:<tag>          # Deploy manager via Helm
make helm-deploy IMG=$IMG HELM_EXTRA_ARGS="--set ..."    # Deploy with custom values
make helm-status                                         # Show release status
make helm-uninstall                                      # Remove release
make helm-history                                        # View release history
make helm-rollback                                       # Rollback to previous version
```

**For end users/production:**
```bash
helm install my-release ./<output-dir>/chart/ --namespace <ns> --create-namespace
```

**Important:** If you add webhooks or modify manifests after initial chart generation:
1. Backup any customizations in `<output-dir>/chart/values.yaml` and `<output-dir>/chart/manager/manager.yaml`
2. Re-run: `kubebuilder edit --plugins=helm/v2-alpha --force` (use same `--output-dir` if customized)
3. Manually restore your custom values from the backup

### Publish Container Image

```bash
export IMG=<registry>/<project>:<version>
make docker-build docker-push IMG=$IMG
```

## References

### Essential Reading
- **Kubebuilder Book**: https://book.kubebuilder.io (comprehensive guide)
- **controller-runtime FAQ**: https://github.com/kubernetes-sigs/controller-runtime/blob/main/FAQ.md (common patterns and questions)
- **Good Practices**: https://book.kubebuilder.io/reference/good-practices.html (why reconciliation is idempotent, status conditions, etc.)
- **Logging Conventions**: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-instrumentation/logging.md#message-style-guidelines (message style, verbosity levels)

### API Design & Implementation
- **API Conventions**: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md
- **Operator Pattern**: https://kubernetes.io/docs/concepts/extend-kubernetes/operator/
- **Markers Reference**: https://book.kubebuilder.io/reference/markers.html

### Tools & Libraries
- **controller-runtime**: https://github.com/kubernetes-sigs/controller-runtime
- **controller-tools**: https://github.com/kubernetes-sigs/controller-tools
- **Kubebuilder Repo**: https://github.com/kubernetes-sigs/kubebuilder
=======
* Fields follow Kubernetes naming conventions (camelCase JSON, PascalCase Go)
* `Spec` is declarative and user-facing; `Status` is controller-owned
* No mutable fields in `Spec` that belong in `Status`
* Clear separation of:
  * Desired state (`Spec`)
  * Observed state (`Status`)
* Use of standard types where applicable:
  * `metav1.Time`
  * `metav1.Condition`
  * `resource.Quantity`
* Conditions follow Kubernetes conventions:
  * Stable, well-defined `Type`
  * Correct `Status`, `Reason`, and `Message`
* Defaulting and validation are implemented via:
  * Webhooks (preferred)
  * OpenAPI schema where applicable

### Backward Compatibility

* No breaking changes to existing fields without versioning
* Additive-only changes for existing API versions
* Deprecated fields are clearly marked and documented

---

## Controller / Reconciler Review Checklist

### Reconcile Logic

Ensure the controller:

* Is **idempotent**
* Can be safely re-run at any time
* Reconciles based on **current cluster state**, not assumptions
* Handles:
  * NotFound errors correctly
  * Partial failures gracefully
  * Retries via requeue or error return, not loops

### Error Handling

* Errors are propagated correctly to trigger retries
* Transient vs permanent errors are distinguished where possible
* No silent failures
* Logging includes enough context (namespaced name, resource identifiers)

### Controller Runtime Usage

* Proper use of:
  * `client.Client`
  * `controllerutil.CreateOrUpdate`
  * OwnerReferences
* Avoid direct API calls when controller-runtime helpers exist
* Watches are minimal and intentional
* Predicates are used to reduce unnecessary reconciliations

---

## Status Management

Reviewers should ensure:

* Status updates are:
  * Performed via `Status().Update()` or `Status().Patch()`
  * Separated from spec mutations
* Status reflects **observed state**, not desired state
* Conditions are updated consistently and deterministically
* No hot loops caused by status-only changes triggering reconciliation

---

## Go Style and Project Structure

### Go Idioms

* Code follows standard Go formatting and idioms
* Clear, explicit error handling
* Minimal use of global state
* Small, focused functions
* Interfaces are introduced only when justified

### Project Layout

* Standard Kubebuilder layout is preserved
* API types, controllers, and internal logic are clearly separated
* Helpers and utilities are reusable and well-scoped

---

## Testing Expectations

Reviewers should check for:

* Unit tests for:
  * Reconcile logic
  * Pure functions
* Envtest-based tests for:
  * Controller behavior
  * API interactions
* Tests are deterministic and do not rely on timing assumptions
* Fake clients are used appropriately

---

## Logging and Observability

* Structured logging using controller-runtime logger
* No excessive log noise in hot paths
* Logs include relevant identifiers
* Events are emitted for meaningful user-facing state changes

---

## What to Flag Explicitly

Reviewers **should flag**:

* Non-idempotent reconcile logic
* Spec mutations during reconciliation
* Custom patterns that duplicate standard Kubernetes behavior
* Hidden coupling between controllers
* Over-engineered abstractions
* Ignoring API conventions or Go idioms

---

## Final Guideline

When in doubt, prefer the approach that:

* Matches upstream Kubernetes patterns
* Is easiest to reason about during failure scenarios
* Would be familiar to an experienced Kubernetes contributor

Consistency with the Kubernetes ecosystem is more important than local preference.
>>>>>>> tmp-original-17-02-26-00-42
