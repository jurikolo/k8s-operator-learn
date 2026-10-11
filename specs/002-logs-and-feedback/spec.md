# Feature Specification: Operator Logging, Visible Failures and Spec-Only Resources

**Feature Branch**: `feature/002-logs-and-feedback`

**Created**: 2026-10-11

**Status**: Draft

**Input**: User description: "1. There must be logs when the operator creates a resource — right now it is silently created without any logs; errors should be logged as well. 2. Custom resources currently require a metadata section; the user wants to completely get rid of it — a custom resource should only require the spec section, and all metadata must be handled by the operator automatically from the user's input. 3. When invalid metadata is provided to CustomDeployment, kubectl says the object was created although in fact nothing was created — add error handling so a failed creation is shown to the customer."

## Clarifications

### Session 2026-10-11

- Q: What should happen when two custom objects of the same kind claim the same real resource name (for example, two CustomNamespaces whose spec both say `team-apps`)? → A: First claim wins — the object whose operator created the real resource first keeps it; the second object reports a failed state naming the conflict, mirroring the existing unmanaged-conflict behavior.
- Q: Should the operator also log when it deletes a real resource during cleanup of a custom object? → A: Yes — one log entry per deletion, matching the creation-log detail (kind, name, namespace, owning custom object).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A failed creation is visible to the user (Priority: P1)

A user creates a custom resource (for example a CustomDeployment) and something prevents the real resource from being created — for example the custom object was created in a namespace where the operator cannot find the referenced CustomNamespace, or the cluster rejects the real resource. Today the user sees only kubectl's "created" and nothing else: no Deployment appears and nothing explains why. After this feature, the custom resource itself carries the failure: anyone inspecting the object with standard cluster tooling sees that it failed and why, without access to operator logs.

**Why this priority**: Today the operator misleads the user — kubectl says created, nothing is created, and nothing reports the problem. Fixing the silent failure is the highest-value change; logging (Story 2) only helps people who can read operator logs.

**Independent Test**: Can be fully tested by creating a CustomDeployment that cannot succeed (for example one referencing a CustomNamespace that does not exist in the object's namespace) and observing that the object reports the failure with the reason while no Deployment exists.

**Acceptance Scenarios**:

1. **Given** no CustomNamespace named `ghost` exists in the custom object's namespace, **When** the user creates a CustomDeployment referencing `ghost`, **Then** the object reports a failed state naming `ghost` as the missing dependency and no Deployment exists in any namespace.
2. **Given** a condition that makes the real resource's creation fail (for example the cluster rejects the object), **When** the operator's create attempt fails, **Then** the custom object reports a failed state whose message contains the failure cause.
3. **Given** a custom object reporting a failed state, **When** the blocking problem is resolved, **Then** the object reports success again without user intervention.
4. **Given** the user provides metadata on the custom object that the operator cannot act on as given (for example the object is created in a namespace where its referenced CustomNamespace does not exist), **When** the operator reconciles, **Then** the object's state makes the problem visible rather than leaving it silently inert.

---

### User Story 2 - Operator activity is logged (Priority: P2)

Every time the operator creates a real resource on behalf of a custom resource, it writes a log entry recording what it created (kind, name, target namespace) and for which custom resource. Every error the operator hits while fulfilling a custom resource is logged with the failure cause. A user watching the operator's log output can follow what the operator did and what failed without inspecting the cluster.

**Why this priority**: Logging is the operator-maintainer's view; it complements Story 1 (the user's view) and is the prerequisite for diagnosing any future misbehavior.

**Independent Test**: Can be fully tested by creating one custom resource of each kind and reading the operator's log output: each real-resource creation appears as a log entry; then forcing a failure produces an error entry; then deleting a custom resource produces a deletion entry.

**Acceptance Scenarios**:

1. **Given** a running operator, **When** a CustomNamespace, CustomDeployment or CustomConfig causes a real resource to be created, **Then** the operator log contains one entry per creation naming the real resource's kind, name and namespace.
2. **Given** a running operator, **When** an operation the operator performs for a custom resource fails, **Then** the operator log contains an error entry with the resource identity and the failure cause.
3. **Given** a successful creation, **When** the user watches the operator log, **Then** the create entry appears within the operator's regular reconcile cycle after the custom object is created.
4. **Given** a custom object being deleted whose real resource is removed by cleanup, **When** the operator deletes the real resource, **Then** the operator log contains one entry per deletion naming the real resource's kind, name and namespace.

---

### User Story 3 - Custom resources require only a spec (Priority: P3)

A user writing a custom-resource manifest should not have to construct a metadata section: the real resource's name and placement come from the spec alone. CustomNamespace and CustomConfig gain a name field in their spec (CustomDeployment already has one), and the operator derives every real resource's metadata from that spec input. The custom object's own manifest keeps exactly one metadata line — its name — which the platform requires but the operator no longer interprets: any value works, and extra metadata (labels, annotations) changes nothing.

**Why this priority**: Ergonomics and API cleanliness; nothing is broken today beyond the friction and the misleading silent failure already covered by Story 1.

**Independent Test**: Can be fully tested by applying a manifest for each custom-resource kind that contains only the fields the clarified design requires, and observing the expected real resource with the expected name.

**Acceptance Scenarios**:

1. **Given** a CustomNamespace named `team-apps` exists, **When** the user creates a CustomDeployment with a manifest containing only the fields the clarified design requires, **Then** a Deployment with the expected name appears in `team-apps`.
2. **Given** the clarified design, **When** the user creates two CustomNamespace objects whose own names are arbitrary and whose spec names are `team-apps` and `team-b`, **Then** both real namespaces appear under their spec names — the custom objects' own names are irrelevant.
3. **Given** the clarified design, **When** the user supplies extra metadata (labels or annotations) on a custom object, **Then** the operator's behavior does not change and nothing fails silently.
4. **Given** the clarified design, **When** the user submits a manifest missing a required spec field, **Then** the object is rejected with a clear validation message.

---

### Edge Cases

- What happens when the custom object is created in a namespace other than the one the operator expects (the user's silent-failure repro)? The object must report the problem; it must not sit "created" and inert.
- What happens when the real resource's creation is rejected by the cluster (admission, quota, invalid field)? It is logged and reported on the custom object.
- What happens when the user supplies metadata the operator does not need (labels, annotations, a different namespace)? It is either ignored predictably or rejected with a clear message — never a silent behavior change.
- What happens when a failure is later resolved? The object returns to a ready state without user intervention.
- What happens when the identity input for the real resource changes after creation? It is treated as immutable and rejected with a clear message, matching the existing immutability rule for the namespace field.
- What happens when two custom objects of the same kind claim the same real resource name? The first claim wins; the second reports a failed state naming the conflict and the existing owner.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The operator MUST log an entry when it creates a real resource on behalf of a custom resource, naming the real resource's kind, name and target namespace and the owning custom resource.
- **FR-002**: The operator MUST log an error entry, including the failure cause and the resource identity, whenever an operation it performs for a custom resource fails.
- **FR-003**: When a real resource cannot be created, the custom resource MUST report a failed state whose message names the cause, visible by inspecting the custom object with standard cluster tooling.
- **FR-004**: A custom resource reporting a failed state MUST return to a success state once the blocking problem is resolved, without user intervention.
- **FR-005**: Metadata the user provides that the operator does not require MUST NOT silently change or suppress the outcome; it is either ignored predictably or rejected with a clear message.
- **FR-006**: A custom-resource manifest MUST require only the spec section as operator input: CustomNamespace and CustomConfig spec MUST carry the real resource's name, and all metadata of the created real resources MUST be derived from that spec input. The custom object's own name is platform plumbing whose value the operator ignores.
- **FR-007**: The identity input that names a created real resource MUST be immutable after creation; changes are rejected with a clear message.
- **FR-008**: A manifest missing a required spec field MUST be rejected with a clear validation message.
- **FR-009**: When two custom objects of the same kind claim the same real resource name, the first claim MUST win and the second MUST report a failed state naming the conflict and the existing owner.
- **FR-010**: The operator MUST log an entry, with the same detail as creation logging, when it deletes a real resource during cleanup of a custom object.

### Key Entities *(include if feature involves data)*

- **Custom resource (CustomNamespace / CustomDeployment / CustomConfig)**: the user-facing object; carries the spec input and the status that reports success or failure; its own metadata shrinks to the minimum the platform mandates.
- **Real resource (Namespace / Deployment / ConfigMap)**: created and managed by the operator; all of its metadata (name, namespace, ownership labels) is derived by the operator from the custom resource's spec input.
- **Status condition**: the per-object reporting surface the user inspects; carries a reason and a human-readable message.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of real-resource creations and deletions appear in the operator log with kind, name and namespace.
- **SC-002**: 100% of failed operations produce both an operator log entry and a failed state on the custom object, within one reconcile cycle (under a minute in the local development cluster).
- **SC-003**: A user can determine why a custom object produced nothing by inspecting the object alone (no operator log access) in under 2 minutes.
- **SC-004**: A user can create all three custom-resource kinds with manifests containing only the fields the clarified design requires, and the expected real resources appear with the expected names.
- **SC-005**: Zero cases remain where kubectl reports a custom object as created while the object ends up permanently inert without any reported reason.

## Assumptions

- Kubernetes itself rejects any object without a name, so "no metadata at all" is impossible; the accepted design keeps exactly one metadata line (the custom object's own name) and moves all real-resource identity into the spec.
- This design supersedes feature 001's rule that the CustomNamespace object's own name names the real namespace; existing objects following the 001 convention will need their spec to carry the name after this change.
- Custom objects continue to live in the `default` namespace, as in feature 001.
- "Logs" means the operator's standard log output in the local development cluster at the default log level; no new log infrastructure is introduced.
- kubectl will keep reporting "created" for the custom object itself — the object is created; the fix is that the object then reports the outcome of the real-resource creation, not that kubectl's immediate output changes.
- Failure reporting continues the feature-001 status-condition pattern; no new reporting mechanism is introduced beyond what is needed to close the silent paths.
