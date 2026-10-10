# Data Model: Custom Namespace-Managed Resources

Feature: [spec.md](./spec.md) · Decisions: [research.md](./research.md) · 2026-10-10

API group/version: `custom.jurikolo.name/v1` (D1). All kinds namespaced; in practice used in `default`.

## Entities

### CustomNamespace (namespaced, used in `default`)

| Field | Type | Validation | Maps to |
|-------|------|------------|---------|
| (metadata.name) | string | CEL: not `default`, no `kube-` prefix (D2) | real Namespace name (FR-002) |

No spec fields — the object name is the whole interface.

- **Finalizer**: `custom.jurikolo.name/finalizer` — deletion guard (D5): delete Namespace, wait until gone, then release (FR-004).
- **Status**: condition `Ready`; `Reason=NamespaceDraining` while the Namespace still exists during deletion; `Ready=True` once present.

State transitions: `Created → Ready` (namespace exists / adopted idempotently) → on delete request `Terminating (NamespaceDraining)` → finalizer released when Namespace gone.

### CustomDeployment (namespaced, used in `default`)

| Field | Type | Validation | Maps to |
|-------|------|------------|---------|
| `namespace` | string, required | CEL `self == oldSelf` — immutable (D2) | CustomNamespace name → real Deployment's namespace (FR-006/FR-007) |
| `name` | string, required | — | real Deployment name |
| `image` | string, required | — | single container image (D8) |
| `replicas` | *int32, optional | `Minimum=0`, default 1 | Deployment replicas |

- **Finalizer**: same key — deletes real Deployment before release (D4).
- **Owner label** on real Deployment: `custom.jurikolo.name/owner: CustomDeployment/<cr-namespace>/<cr-name>` (D6).
- **Status conditions** (D10): `Ready=True`; `Ready=False, Reason=MissingCustomNamespace`; `Ready=False, Reason=NameConflict`; `Reason=Reconciling` transient.

State transitions: `Created → MissingCustomNamespace (retry)` → `Ready` (Deployment exists & matches) → `NameConflict` if unmanaged resource appears under the same name → back to `Ready` after repair. On delete: finalizer removes Deployment → released.

### CustomConfig (namespaced, used in `default`)

| Field | Type | Validation | Maps to |
|-------|------|------------|---------|
| `namespace` | string, required | CEL `self == oldSelf` — immutable | CustomNamespace name → real ConfigMap's namespace |
| `data` | map[string]string, optional | — | ConfigMap `data` |
| `binaryData` | map[string][]byte, optional | — | ConfigMap `binaryData` |
| `immutable` | *bool, optional | — | ConfigMap `immutable` |

Same finalizer, owner label, conditions, and state transitions as CustomDeployment (ConfigMap instead of Deployment; `name` from object name per D9).

### Managed real resources

- **Namespace** (cluster-scoped): name = CustomNamespace name. Deleted by operator only via CustomNamespace deletion; recreated if removed out-of-band while the CustomNamespace exists (FR-003).
- **Deployment** (`apps/v1`, in managed namespace): single container `name` = deployment name, `image`; replicas; labels: owner label (D6).
- **ConfigMap** (core/v1, in managed namespace): data/binaryData/immutable verbatim; owner label.

## Relationships

```text
CustomNamespace (default)  --name-->  Namespace  (1:1, adoption if pre-existing, non-reserved)
CustomDeployment (default) --namespace field--> CustomNamespace (must exist, immutable ref)
CustomDeployment           --1:1-->    Deployment in that namespace
CustomConfig (default)     --namespace field--> CustomNamespace (must exist, immutable ref)
CustomConfig               --1:1-->    ConfigMap in that namespace
```

Cross-namespace ownerReferences are invalid, so ownership is expressed by finalizers + owner labels (D4/D6), not GC cascade.

## Uniqueness & conflicts

- CustomNamespace name uniqueness = real namespace name uniqueness (cluster-wide); reserved names rejected at admission (D2).
- Real resource names must be free in the target namespace: unmanaged occupants cause `NameConflict` (D6, FR-013).
- Two custom objects cannot claim the same real resource: the second sees the first's owner label and reports `NameConflict` (different owner identity).

## Scale assumptions

Learning scope: single-cluster, low object counts (tens). Reconcile throughput, HA, and metrics are out of scope (constitution II).
