# Research: Custom Namespace-Managed Resources

Feature: [spec.md](./spec.md) · Branch `001-custom-namespace-crds` · 2026-10-10

All NEEDS CLARIFICATION items: none. Decisions below resolve the design questions raised by the spec's requirements.

## D1. API group, version, scoping

**Decision**: kubebuilder `--group custom --version v1` → API group `custom.jurikolo.name/v1`. All three kinds are **namespaced** resources.

**Rationale**: follows the project domain from `PROJECT` (`jurikolo.name`) with a short descriptive group; namespaced kinds match the spec (objects created in `default`).

**Alternatives considered**: group `learning` (equally fine, no functional difference); cluster-scoped CustomNamespace (rejected — spec states the object lives in `default`).

## D2. Schema-level validation via CRD markers, not webhooks

**Decision**: enforce what a CRD schema can express, using kubebuilder markers on the Go types:

- Required fields: `+kubebuilder:validation:Required` (CustomDeployment: `namespace`, `name`, `image`; CustomConfig fields map from ConfigMap parity).
- `replicas` non-negative: `+kubebuilder:validation:Minimum=0` (default 1 via `+kubebuilder:default=1`).
- Reserved namespace names (FR-002): root-level `+kubebuilder:validation:XValidation:rule="self.metadata.name != 'default' && !self.metadata.name.startsWith('kube-')"` on CustomNamespace → rejected at admission.
- Immutable `namespace` reference (FR-014): field-level CEL rule using `oldSelf` → rejects field change at admission.

**Rationale**: declarative, no webhook deployment/certs (webhooks are production hardening the constitution excludes), and rejection happens at request time exactly as the spec phrases it.

**Alternatives considered**: admission webhooks (rejected: complexity, certs); checking reserved names in the reconciler (rejected: object would be accepted first — weaker than "rejected").

**Prerequisite**: Kubernetes ≥ 1.25 for `oldSelf` in CEL validation rules.

## D3. Cross-resource reference check (FR-006/FR-010) in reconcilers

**Decision**: CustomDeployment/CustomConfig reconcilers `Get` the referenced CustomNamespace **in the referencing object's own namespace**. Missing → status condition `Ready=False, Reason=MissingCustomNamespace`, requeue periodically until it appears (matches the spec's eventual-consistency assumption).

**Rationale**: a CRD schema cannot validate existence of another object; the reconciler is the idiomatic place, and conditions satisfy FR-012 visibility.

**Alternatives considered**: hardcoding lookup in `default` (rejected: the referencing object's namespace is the same thing in practice and keeps code generic).

## D4. Cascade deletion via finalizers (not ownerReferences)

**Decision**: each reconciler sets a finalizer on its custom object; on deletion it deletes the real resource, then removes the finalizer.

**Rationale**: Kubernetes forbids cross-namespace ownerReferences (real Deployment in `team-apps` cannot be owned by a CustomDeployment in `default`), and Namespace is cluster-scoped — so GC cascade cannot do the job. Finalizers are the idiomatic manual pattern.

**Alternatives considered**: ownerReferences (rejected: invalid cross-namespace); orphaning real resources on custom-object deletion (rejected: contradicts documented assumption).

## D5. CustomNamespace deletion guard (FR-004) via explicit emptiness check

**Decision**: the CustomNamespace reconciler holds a finalizer. On deletion it checks namespace contents directly: discovery enumerates every namespaced kind the API server offers and lists each in the target namespace (unstructured lists). Any non-injected object present → deletion blocked, condition `Ready=False, Reason=NamespaceDraining`, namespace left Active; only an empty namespace is deleted, and the finalizer releases once the namespace object is gone. Kubernetes-injected objects (events, `kube-root-ca.crt` ConfigMap, `default` ServiceAccount) don't count as contents. The check needs read-only cluster-wide RBAC (`groups=*,resources=*,verbs=get;list`).

**Rationale**: riding on Kubernetes' own namespace-drain refusal is NOT a guard — the drain destroys the contents, which is exactly what the requirement prevents. The explicit check holds deletion open so users can intervene. Verifying "any resources" needs discovery + dynamic listing; a fixed kind list would miss kinds (data-loss risk).

**Alternatives considered**: deleting the Namespace and relying on drain semantics (rejected during live verification: contents get destroyed, guard evaporates in real clusters while passing envtest, which runs no controllers); listing a fixed set of common kinds (rejected: incomplete — misses CRDs and any kind not on the list).

**Prerequisite**: deployed mode needs the wildcard read RBAC; kinds the RBAC cannot read are invisible to the check (documented `shortcut:` in code).

## D6. Owner identity label + name-conflict detection (FR-013)

**Decision**: real resources created by the operator are stamped with label `custom.jurikolo.name/owner: <Kind>/<CR-namespace>/<CR-name>`. Before creating a real resource, the reconciler `Get`s it: if it exists **without our owner label** → condition `Ready=False, Reason=NameConflict`, no modification. If it exists with **our** label → treat as ours and reconcile it.

**Rationale**: labels work across namespaces and scopes (unlike ownerReferences) and give a one-field check for "does something unmanaged already use this name".

**Alternatives considered**: adopt-any-existing-resource (rejected: contradicts FR-013); uniqueness only by Create-API conflict (insufficient: doesn't distinguish unmanaged resources from our own).

## D7. Drift correction via periodic requeue

**Decision**: every reconciler returns `RequeueAfter: 30s` while its object exists. Out-of-band changes/deletions of real resources (and of a real Namespace, FR-003) are repaired on the next pass; this meets SC-005 (60 s).

**Rationale**: no extra watches/mapping functions — smallest complete mechanism for a learning operator. `shortcut: drift repair latency up to 30 s + conditions may lag; upgrade to watching the real resources (label-based mapping) if immediate reaction is required`.

**Alternatives considered**: `Owns()`/watch real resources (rejected: needs label-mapping predicates since ownerReferences are unusable here — more code for no spec benefit).

## D8. Real Deployment shape (FR-005/FR-007)

**Decision**: single container, container name = deployment `name`, `image` set, nothing else (no ports/env/volumes/probes); default rolling strategy; replicas from spec.

**Rationale**: spec limits parameters explicitly; k8s defaults fill the rest.

## D9. CustomConfig field parity (FR-009)

**Decision**: spec fields `data` (map[string]string), `binaryData` (map[string][]byte), `immutable` (*bool) — the full value-carrying surface of a ConfigMap; standard metadata (labels/annotations) is inherent to any object. `name` comes from the object name (consistent with the other two kinds).

**Rationale**: "all the same parameters" minus namespace semantics; avoiding an embedded `corev1.ConfigMap` keeps CRD generation clean.

**Alternatives considered**: embedding corev1.ConfigMapSpec-style struct (rejected: not directly generatable, adds nothing here).

## D10. Status reporting (FR-012)

**Decision**: `metav1.Conditions` on all three kinds with one condition type `Ready`; reason vocabulary: `Reconciling`, `Ready`, `MissingCustomNamespace`, `NameConflict`, `NamespaceDraining`, plus error messages propagated verbatim in `message`.

**Rationale**: kubebuilder idiom (`meta.SetStatusCondition`); one condition type is enough for three simple kinds.

## D11. Testing strategy

**Decision**: envtest-based controller tests (`internal/controller/suite_test.go` scaffold), one focused test per risky behavior: reserved-name rejection (schema test), missing-reference retry, name-conflict, deletion guard, cascade delete, drift repair. `make test` runs them with `setup-envtest` binaries.

**Rationale**: constitution I requires one small test per non-trivial logic; envtest exercises real API semantics (finalizers, CEL validation) without a full cluster.

## D12. Shared helper policy

**Decision**: no shared "reconcile helper" package across the three controllers initially; each controller reads on its own. If finalizer/label handling is copy-pasted a third time verbatim, extract then.

**Rationale**: constitution I — no abstraction before duplication is real; keeps each controller teachable in one read.

## D13. RBAC

**Decision**: kubebuilder RBAC markers per controller: `namespaces` (cluster-scope verbs: get;list;watch;create;delete) for CustomNamespace; `deployments` (`apps` group) and `configmaps` (core) with standard CRUD verbs for the other two; plus generated CR permissions. Final RBAC manifests generated by `make manifests`.

**Rationale**: markers keep permissions next to the code that needs them; no manual ClusterRole edits.
