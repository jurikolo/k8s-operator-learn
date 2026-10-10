# Feature Specification: Custom Namespace-Managed Resources

**Feature Branch**: `feature/001-init`

**Created**: 2026-10-10

**Status**: Draft

**Input**: User description: "Three new CRDs: `CustomNamespace` (creates a regular Namespace; the CustomNamespace object itself is created in the `default` namespace), `CustomDeployment` (like a Deployment, limited to: namespace (required, must reference an existing CustomNamespace — otherwise creation fails), name (required), image (required), replicas (optional, default 1); the real Deployment is created in the k8s namespace matching the referenced CustomNamespace), `CustomConfig` (like a ConfigMap with all the same parameters, except the namespace is validated against CustomNamespace; the real ConfigMap is created in the regular k8s namespace if validation passes)."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Create a managed namespace (Priority: P1)

A user wants a dedicated namespace that the operator manages. They create a `CustomNamespace` object named, for example, `team-apps` in the `default` namespace. The cluster then contains a real namespace named `team-apps`, ready to hold workloads.

**Why this priority**: It is the foundation — both other resource types reference a CustomNamespace and cannot work without one.

**Independent Test**: Can be fully tested by creating one CustomNamespace object and observing that a real namespace with the same name appears.

**Acceptance Scenarios**:

1. **Given** a cluster without a namespace named `team-apps`, **When** the user creates a CustomNamespace named `team-apps` in the `default` namespace, **Then** a real namespace `team-apps` exists.
2. **Given** an existing CustomNamespace, **When** its real namespace is deleted out-of-band (directly, not via the custom object), **Then** the operator recreates the real namespace with the same name.
3. **Given** an existing CustomNamespace whose real namespace contains resources, **When** the user deletes the CustomNamespace object, **Then** deletion is rejected with a clear reason and both the object and the namespace remain.
4. **Given** an existing CustomNamespace whose real namespace is empty, **When** the user deletes the CustomNamespace object, **Then** the real namespace is deleted as well.
5. **Given** the cluster's reserved namespaces, **When** the user creates a CustomNamespace named `kube-system`, `default`, or any `kube-` prefixed name, **Then** creation is rejected with a clear reason.

---

### User Story 2 - Deploy an application into a managed namespace (Priority: P2)

A user wants to run an application inside a managed namespace. They create a `CustomDeployment` object in the `default` namespace specifying the target namespace (the name of an existing CustomNamespace), a deployment name, a container image, and optionally a replica count. The cluster then runs a real Deployment with those settings inside that namespace. If the referenced CustomNamespace does not exist, the CustomDeployment fails with a clear reason and nothing is deployed.

**Why this priority**: This is the core workload-delivery value; it depends conceptually on Story 1 (a CustomNamespace must exist first) but is independently testable by creating one CustomNamespace upfront.

**Independent Test**: Can be fully tested by creating a CustomNamespace, then a CustomDeployment referencing it, and observing the real Deployment in that namespace.

**Acceptance Scenarios**:

1. **Given** a CustomNamespace named `team-apps` exists, **When** the user creates a CustomDeployment in the `default` namespace with namespace `team-apps`, name `web`, image `nginx:1.27`, and no replicas value, **Then** a Deployment `web` with 1 replica running image `nginx:1.27` exists in namespace `team-apps`.
2. **Given** the same setup, **When** the user sets replicas to 3, **Then** the Deployment in `team-apps` reports 3 replicas.
3. **Given** no CustomNamespace named `ghost` exists, **When** the user creates a CustomDeployment referencing namespace `ghost`, **Then** the object reports a failed state naming the missing CustomNamespace and no Deployment exists in any namespace.
4. **Given** a CustomDeployment referencing a CustomNamespace that does not exist yet, **When** that CustomNamespace is created later, **Then** the CustomDeployment eventually succeeds and the Deployment appears.

---

### User Story 3 - Store configuration in a managed namespace (Priority: P3)

A user wants configuration data available inside a managed namespace. They create a `CustomConfig` object providing the same content a regular ConfigMap accepts (key/value data, binary data, and the usual object metadata), with its namespace referencing an existing CustomNamespace. The cluster then contains a real ConfigMap with identical content inside that namespace. If the referenced CustomNamespace does not exist, the CustomConfig fails with a clear reason.

**Why this priority**: Completes the set (workload + configuration), but delivers less standalone value than running an application.

**Independent Test**: Can be fully tested by creating a CustomNamespace, then a CustomConfig referencing it, and observing the real ConfigMap and its content.

**Acceptance Scenarios**:

1. **Given** a CustomNamespace named `team-apps` exists, **When** the user creates a CustomConfig in the `default` namespace with namespace `team-apps`, name `app-settings`, and data `{ "log_level": "debug" }`, **Then** a ConfigMap `app-settings` exists in namespace `team-apps` with the same key and value.
2. **Given** an existing CustomConfig, **When** the user changes its data, **Then** the real ConfigMap content is updated to match.
3. **Given** no CustomNamespace named `ghost` exists, **When** the user creates a CustomConfig referencing namespace `ghost`, **Then** the object reports a failed state naming the missing CustomNamespace and no ConfigMap exists in any namespace.

---

### Edge Cases

- What happens when a CustomDeployment or CustomConfig references a CustomNamespace that disappears? Deleting that CustomNamespace is blocked while its real namespace still contains resources (FR-004), so dependents cannot lose workloads through it. If the namespace disappears out-of-band, the operator recreates it and the managed resources are restored; a dependent object reports a failed state naming the missing CustomNamespace only while the CustomNamespace itself does not exist.
- What happens when a CustomNamespace is named after a non-reserved namespace that already exists? Creation succeeds idempotently and the existing namespace is treated as the managed one; it is not recreated or modified.
- What happens when a CustomDeployment is given a negative replica count? The object is rejected by validation. A replica count of 0 is valid and results in zero running replicas.
- What happens when a CustomConfig specifies binary data? It is carried into the real ConfigMap unchanged.
- How does the system treat out-of-band changes to a managed resource (someone edits the real Deployment's image or a real ConfigMap's data)? The operator restores the state defined by the custom object.

## Clarifications

### Session 2026-10-10

- Q: What should happen when a CustomDeployment or CustomConfig uses a name that already belongs to a real Deployment or ConfigMap in the target namespace, one that the operator did not create? → A: The object fails with a name-conflict error; the existing real resource is left untouched.
- Q: Should CustomNamespace refuse to manage critical system namespaces (names like `default`, `kube-system`, or anything starting with `kube-`), given deletion of the custom object deletes the real namespace? → A: Yes — creation of such CustomNamespace objects is rejected with a clear reason.
- Q: After a CustomDeployment or CustomConfig has been created, should its `namespace` field stay locked to the CustomNamespace it pointed at creation, or should users be allowed to repoint it to a different CustomNamespace? → A: Immutable — the reference cannot change after creation; attempts to change it are rejected.
- Q: If a referenced CustomNamespace is deleted and then created again with the same name, should dependents recover on their own and recreate their real resources? → A: Deletion is prevented instead — a CustomNamespace whose real namespace still contains resources cannot be deleted; dependents then follow the existing keep-in-sync rules.

## Requirements *(mandatory)*

### Functional Requirements

**CustomNamespace**

- **FR-001**: System MUST allow users to create CustomNamespace objects in the `default` namespace.
- **FR-002**: Creating a CustomNamespace MUST result in a real namespace whose name equals the CustomNamespace object's name. Creation of a CustomNamespace named `default`, `kube-system`, or any name starting with `kube-` MUST be rejected with a clear reason.
- **FR-003**: While a CustomNamespace exists, its real namespace MUST remain present; if it is deleted out-of-band, the operator recreates it.
- **FR-004**: Deleting a CustomNamespace MUST delete its real namespace, but the deletion MUST be blocked with a clear reason while the real namespace still contains any resources (managed or not); once the namespace is empty, the deletion proceeds.

**CustomDeployment**

- **FR-005**: CustomDeployment MUST require `namespace`, `name`, and `image`; `replicas` MUST be optional and default to 1. No other workload parameters are accepted.
- **FR-006**: A CustomDeployment whose `namespace` does not reference an existing CustomNamespace MUST fail: the failure and its reason (naming the missing CustomNamespace) MUST be visible on the object, and no Deployment MUST be created in any namespace.
- **FR-007**: A valid CustomDeployment MUST produce a real Deployment in the real namespace whose name matches the referenced CustomNamespace, running the requested image under the requested name with the requested replica count.
- **FR-008**: While a CustomDeployment exists, the real Deployment MUST match it: changes to `image` or `replicas` propagate, and out-of-band drift is corrected.

**CustomConfig**

- **FR-009**: CustomConfig MUST accept the same parameters a regular ConfigMap accepts (data, binaryData, and standard object metadata such as labels and annotations).
- **FR-010**: A CustomConfig whose `namespace` does not reference an existing CustomNamespace MUST fail: the failure and its reason MUST be visible on the object, and no ConfigMap MUST be created in any namespace.
- **FR-011**: A valid CustomConfig MUST produce a real ConfigMap with identical content in the real namespace whose name matches the referenced CustomNamespace, kept in sync while the CustomConfig exists.

**Cross-cutting**

- **FR-012**: Every custom object MUST reach a success or failure state within a bounded time, with the reason for any failure readable by the user who created it.
- **FR-013**: A CustomDeployment or CustomConfig whose real resource name is already taken in the target namespace by a resource the operator does not manage MUST fail with a name-conflict error; the existing real resource MUST be left untouched.
- **FR-014**: The `namespace` reference on an existing CustomDeployment or CustomConfig MUST be immutable; attempts to change it MUST be rejected.

### Key Entities *(include if feature involves data)*

- **CustomNamespace**: A custom object living in the `default` namespace; its name is the name of the real namespace it manages.
- **CustomDeployment**: A custom object with `namespace` (reference to a CustomNamespace name), `name`, `image`, and `replicas` (default 1); it owns one real Deployment in the referenced namespace.
- **CustomConfig**: A custom object with `namespace` (reference to a CustomNamespace name) plus ConfigMap-equivalent content; it owns one real ConfigMap in the referenced namespace.
- **Managed real resources**: The real Namespace, Deployment, and ConfigMap objects that mirror the custom objects above and exist only as their counterparts.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A created CustomNamespace has its matching real namespace present within 30 seconds.
- **SC-002**: A valid CustomDeployment has a real Deployment with the requested image and replica count present within 2 minutes.
- **SC-003**: 100% of custom objects referencing a non-existent CustomNamespace report a visible failure, and 0 real resources are created for them.
- **SC-004**: A valid CustomConfig has a real ConfigMap whose content exactly matches the custom object's content within 30 seconds.
- **SC-005**: An out-of-band deletion of any managed real resource is corrected within 60 seconds while its custom object exists.
- **SC-006**: No CustomNamespace deletion ever removes a namespace that contains resources: 100% of such attempts are rejected with a visible reason, while deletions of empty namespaces complete.

## Assumptions

- All three custom object types are created from the `default` namespace. The user stated this for CustomNamespace; it is extended to CustomDeployment and CustomConfig for consistency (the `namespace` field, not the object's location, decides where real resources go).
- Deleting a CustomDeployment or CustomConfig deletes the real resource it manages (ownership cascade). Deleting a CustomNamespace is blocked while its real namespace still contains any resources (FR-004).
- A CustomNamespace whose real namespace already exists succeeds and adopts it (idempotent), rather than erroring; reserved system names are rejected outright instead (FR-002).
- Validation of the CustomNamespace reference is eventual: an object referencing a not-yet-existing CustomNamespace stays in a failed/pending state and succeeds once the CustomNamespace appears.
- `replicas` of 0 is allowed (scale to zero); negative values are rejected.
- CustomDeployment describes a single-container workload defined solely by its image (no ports, env, volumes, or other Deployment features).

