# CRD and Controller Docs Checklist

This page is a practical checklist for documenting a Kubernetes CRD and its controller end-to-end.

Use it when introducing a new API, or when improving stale docs for existing APIs.

## 1. API Contract Documentation

- Purpose and scope of the CRD.
- Clear ownership boundaries (what this controller manages vs what it does not).
- Full `spec` reference with required/optional fields and defaults.
- Full `status` reference with all state enums and conditions.
- Validation rules and immutability constraints.
- Backward compatibility notes for field additions/removals/renames.

## 2. Reconciliation Behavior

- Reconciliation inputs (watched resources, selectors, owner references).
- Reconciliation outputs (child resources, side effects on external systems).
- Idempotency model and conflict handling.
- Finalizer behavior and deletion semantics.
- Retry semantics and backoff or requeue strategy.
- Explicit list of terminal and non-terminal states.

## 3. State Machine and Workflow

- One state diagram for single-resource CRDs.
- One workflow diagram for set/fleet controllers.
- Transition triggers between all states.
- Failure paths and recovery paths.
- Preconditions and gates (maintenance approval, version checks, connectivity checks).

## 4. Operational Runbook

- Common failure signatures and diagnosis steps.
- How to safely retry failed operations.
- How to pause/ignore reconciliation and resume.
- How to recover from orphan references or stuck finalizers.
- Rollback strategy and supported downgrade behavior.

## 5. Security and Compliance

- Secret handling model (which fields use references).
- Required RBAC for controller/service account.
- External endpoint trust requirements (TLS and insecure modes).
- Auditability expectations (events, conditions, logs, metrics).

## 6. Observability and SLOs

- Metrics exposed per controller and key labels.
- Log markers for each major reconcile phase.
- Events and condition reasons expected during normal operations.
- Suggested SLOs: success rate, mean completion time, failure rate by reason.

## 7. Usage Documentation

- Minimal valid example.
- Production-grade example.
- Fleet rollout example (`*Set` CRDs).
- Multi-step real workflow examples (version first, then settings).
- Upgrade sequencing and dependency ordering guidance.

## 8. Testing Documentation

- Unit-test coverage expectations (spec validation helpers, state transitions).
- Integration-test coverage expectations (controller + fake API interactions).
- E2E expectations (real vendor behavior, task lifecycle, maintenance approvals).
- Known environment assumptions and test data requirements.

## 9. Release Notes Inputs

For any CRD/controller change, include:

- User-visible API changes.
- Behavior changes in reconciliation.
- Migration notes.
- New metrics/conditions/events.
- Deprecated fields and removal timelines.

## Recommended Page Structure Per CRD

1. What it does.
2. Spec reference.
3. Status reference.
4. State machine diagram.
5. Workflow.
6. Examples.
7. Failure handling notes.
8. Related CRDs and ordering.