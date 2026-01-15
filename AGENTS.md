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

## High-Level Review Principles

* Prefer **declarative**, **idempotent**, and **level-based** reconciliation logic
* Follow Kubernetes API and controller patterns rather than inventing new abstractions
* Optimize for **clarity and correctness over cleverness**
* Ensure behavior is predictable under retries, restarts, and partial failures

---

## Kubernetes API Review Checklist

### API Types (CRDs)

Reviewers should verify:

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
