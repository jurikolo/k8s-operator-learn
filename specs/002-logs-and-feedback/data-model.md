# Data Model: Operator Logging, Visible Failures and Spec-Only Resources

Feature: [spec.md](./spec.md) · Decisions: [research.md](./research.md) · 2026-10-11

API group/version unchanged: `custom.jurikolo.name/v1`. All kinds namespaced, used in `default`. Only deltas from 001 are listed.

## Entities

### CustomNamespace (namespaced, used in `default`)

| Field | Type | Validation | Maps to |
|-------|------|------------|---------|
| `spec.name` *(new)* | string, required | immutable (`self == oldSelf`); `MaxLength=63`; RFC 1123 label pattern; CEL: not `default`, no `kube-` prefix (D2) | real Namespace name (was: object's `metadata.name`) |

- The object's own `metadata.name` is no longer interpreted; any DNS-1123-valid value works.
- **Owner label** *(new on Namespace)*: `custom.jurikolo.name/owner: CustomNamespace/<cr-ns>/<cr-name>` — stamped at create, checked before adopt (D3).
- **Status conditions**: `Ready=True`; `Ready=False, Reason=NameConflict` *(new)* — target exists under a different/absent owner; `Reason=NamespaceDraining`, `Reason=Reconciling` unchanged; `Reason=CreateFailed` *(new)* on failed Namespace create (D4).

State transitions: `Created → Ready` (namespace created or adopted when owner label matches) or `Created → NameConflict` (label mismatch, no takeover); on create failure `CreateFailed` → `Ready` after the error clears; delete path unchanged (`NamespaceDraining` → namespace deleted → finalizer released).

### CustomDeployment (namespaced, used in `default`)

| Field | Type | Validation | Maps to |
|-------|------|------------|---------|
| `spec.name` | string, required | CEL `self == oldSelf` — immutable *(new marker)* (D2) | real Deployment name (unchanged mapping) |

Behavior deltas only:

- **Status conditions**: `Reason=CreateFailed` *(new)* on failed Deployment create (D4); `MissingCustomNamespace`, `NameConflict`, `Reconciling`, `Ready` unchanged.

### CustomConfig (namespaced, used in `default`)

| Field | Type | Validation | Maps to |
|-------|------|------------|---------|
| `spec.name` *(new)* | string, required | immutable (`self == oldSelf`); `MaxLength=253`; RFC 1123 subdomain pattern (D2) | real ConfigMap name (was: object's `metadata.name`) |

- Conflict detection is unchanged in code but now keys on `spec.name`, so two CustomConfig objects claiming the same ConfigMap name in one namespace resolve first-claim-wins (D3).
- **Status conditions**: `Reason=CreateFailed` *(new)* (D4); existing reasons unchanged.

## Cross-entity rules

- **Ownership**: real resources carry `custom.jurikolo.name/owner` = `<Kind>/<cr-ns>/<cr-name>`. The CR identity in the label is bookkeeping only — it never appears in the real resource's name and is not user-facing (D1/D3).
- **Uniqueness**: one real resource per (kind, target namespace, spec.name). Same-kind duplicates: first claim wins, challenger reports `NameConflict` (FR-009). Cross-kind name sharing (Deployment and ConfigMap both named `web`) is fine — different kinds.
- **Identity immutability**: `spec.name` is immutable on all three kinds, rejected at admission with a clear message (CustomDeployment gains the marker this feature; previously a name change would have orphaned the old Deployment). See [contracts/crds.md](./contracts/crds.md).

## Observability contract (FR-001/002/003/010)

Every real-resource create/delete emits `log.Info` with kind, name, namespace and owning custom object; every failed operation emits `log.Error(err, operation, identity...)`; every failed create additionally sets `Ready=False` with a reason naming the cause. No metrics, traces or events.
