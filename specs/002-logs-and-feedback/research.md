# Research: Operator Logging, Visible Failures and Spec-Only Resources

Feature: [spec.md](./spec.md) · Branch `002-logs-and-feedback` · 2026-10-11

All NEEDS CLARIFICATION items: none. The two spec clarifications (first-claim-wins; deletion logging) were answered by the user on 2026-10-11 and are folded into the decisions below.

## D1. Identity moves to `spec.name` (FR-006/FR-007/FR-008)

**Decision**: `CustomNamespaceSpec` and `CustomConfigSpec` each gain a required `Name string` field; both reconcilers switch real-resource lookups/creates from `cn.Name`/`cc.Name` to `cn.Spec.Name`/`cc.Spec.Name`. CustomDeployment already carries `spec.name` and is untouched. The custom object's own `metadata.name` is no longer read by any reconciler.

**Rationale**: the user chose "spec carries the name" (spec Clarifications, 2026-10-11): the smallest design that makes manifests spec-only while keeping `kubectl apply` working; the platform-mandated object name stays as ignored plumbing.

**Alternatives considered**: `generateName`-only objects (rejected: breaks `apply`, generated names awkward to reference); keeping CR-name-as-identity (rejected: does not deliver spec-only manifests).

## D2. Validation markers on the new fields (FR-007/FR-008)

**Decision**, following 001's marker idiom (D2 in 001 research):

- Both fields: `+kubebuilder:validation:Required`, immutable via `+kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"`.
- `CustomNamespace.spec.name` (DNS label): `MaxLength=63`, `Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`; the root-level reserved-name CEL rule moves from `self.metadata.name` to `self.spec.name` (not `default`, no `kube-` prefix).
- `CustomConfig.spec.name` (DNS subdomain, ConfigMap rules): `MaxLength=253`, `Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`.
- `CustomDeployment.spec.name` gains the same immutable marker (`self == oldSelf`) — FR-007 applies to all three kinds, and today a name change would orphan the old Deployment.

**Rationale**: a bad name is rejected at admission with a clear message instead of surfacing later as a real-resource create failure; declarative markers, no webhooks.

**Alternatives considered**: relying on the real resource's own API validation (rejected: weaker failure mode — object accepted, then stuck in `CreateFailed`, exactly the confusion this feature removes).

**Prerequisite**: Kubernetes ≥ 1.25 for `oldSelf` in CEL (already satisfied; 001 uses it).

## D3. First-claim-wins for namespaces (FR-009)

**Decision**: created Namespaces get the same owner label the other kinds use (`custom.jurikolo.name/owner: CustomNamespace/<cr-ns>/<cr-name>` via `ownerRef()`). When the target Namespace already exists, its label is checked: label equals this object's owner → adopt (idempotent reconcile); label differs or absent → `Ready=False, Reason=NameConflict`, message naming the conflict and the existing owner, resource untouched. CustomConfig gains the same behavior automatically: its conflict path exists but was keyed on `cc.Name`; it now keys on `cc.Spec.Name`.

**Rationale**: today an existing namespace is silently adopted by any CustomNamespace — acceptable when name == object name (API uniqueness prevented two claims), but with spec-carried identity two objects can claim one name and silence would violate FR-005/FR-009. Deployments already demonstrate the exact pattern; namespaces join it.

**Alternatives considered**: rejecting duplicates at admission (rejected: needs a webhook, out of scope per constitution); last-write-wins (rejected: user explicitly chose first-claim-wins).

## D4. Failure surfacing via `CreateFailed` conditions (FR-003/FR-004)

**Decision**: in all three reconcilers, a failed real-resource `Create` sets `Ready=False, Reason=CreateFailed, Message=<error>` before returning the error (existing `setCondition` helper). Returning the error keeps controller-runtime's backoff retry, so the object returns to `Ready` once the create succeeds — FR-004 needs no extra machinery. The silent-failure repro from the spec (object placed where the referenced CustomNamespace is absent) already reports `MissingCustomNamespace`; no change needed there.

**Rationale**: reuses the established condition pattern; conditions are the spec's chosen visibility surface (standard cluster tooling shows them).

**Alternatives considered**: events (`recorder.Eventf`) as a second channel (rejected: conditions already satisfy FR-003; events would be a parallel mechanism nobody asked for); terminal failure without retry (rejected: contradicts FR-004).

## D5. Logging (FR-001/FR-002/FR-010)

**Decision**: keep `logf.FromContext(ctx)` structured logging; no new log infrastructure.

- Creations: the three existing `log.Info("created ...")` calls gain an argument naming the owning custom object (`"<kind>", "namespace/name"`).
- Deletions: `log.Info("deleted ...")` added at the three finalizer cleanup `Delete` call sites (namespace, deployment, configmap) with the same identity arguments.
- Errors: explicit `log.Error(err, "<operation> failed", identity...)` at each error-return site in the three reconcilers (get/create/update/delete/status writes). Controller-runtime already logs returned errors, but its generic "Reconciler error" line is rate-limited and lacks the operation context; explicit site-level logging is the learning-friendly idiom (constitution II).

**Rationale**: FR-001's identity detail and FR-010 are one-argument additions; error logs at the failure site say *what* failed, not just that reconcile failed.

**Alternatives considered**: a wrapping helper that logs every returned error centrally (rejected: hides the flow from a learner, saves almost nothing — the reconcilers are short).

## D6. Test strategy

**Decision**: extend the existing per-controller envtest suites (`make test`):

- Update existing fixtures/assertions: CustomNamespace/CustomConfig objects now set `spec.name`; real-resource identity assertions follow spec.name.
- New admission tests: missing `spec.name` rejected; immutable `name` change rejected; reserved/invalid names (`default`, `kube-x`, uppercase) rejected — CEL runs in envtest (CRDs are applied by the suite).
- New collision test: two CustomNamespace objects with different object names and the same `spec.name` → first `Ready`, second `Ready=False/NameConflict`.
- Failure-condition test: an unmanaged real resource under the claimed name → `NameConflict` condition (covers the failed-state reporting path deterministically).
- Create-failure (`CreateFailed`) is not deterministically forceable in envtest (would need to race the reconciler on an out-of-band object); covered by the live `quickstart.md` scenario instead.
- Controller log lines are verified manually via quickstart; asserting zap output in ginkgo adds test-only plumbing for little value.

**Rationale**: conditions and admission rejections are the spec-visible behaviors and are deterministic in envtest; log lines are format, not logic.

**Alternatives considered**: log-capture test harness (rejected: plumbing outweighs value); forcing `CreateFailed` in envtest (rejected: racy).
