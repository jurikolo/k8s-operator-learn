# API Contract: CRDs `custom.jurikolo.name/v1`

The operator's public interface is its CRD API. Field semantics and state machine: [data-model.md](../data-model.md). YAML below is the user-facing contract; exact schemas are generated from `api/v1` types (`make manifests`).

All three kinds are **namespaced** and used in namespace `default`.

## CustomNamespace

```yaml
apiVersion: custom.jurikolo.name/v1
kind: CustomNamespace
metadata:
  name: team-apps        # becomes the real namespace name
  namespace: default
```

**Admission-time rejections** (request fails):

- name `default`, name `kube-system`, or any `kube-` prefixed name.

**Observable states** (`status.conditions[type=Ready]`):

| Reason | Meaning |
|--------|---------|
| `Ready` | Real namespace exists (pre-existing non-reserved namespaces are adopted, not modified) |
| `NamespaceDraining` | Delete requested; namespace still contains resources — deletion stays blocked until it is empty |
| `Reconciling` | Namespace missing out-of-band; recreation in progress |

## CustomDeployment

```yaml
apiVersion: custom.jurikolo.name/v1
kind: CustomDeployment
metadata:
  name: web              # custom object name
  namespace: default
spec:
  namespace: team-apps   # required, immutable: must name an existing CustomNamespace
  name: web              # required: the real Deployment's name
  image: nginx:1.27      # required
  replicas: 3            # optional, default 1, minimum 0
```

**Admission-time rejections**:

- missing `namespace` / `image` (`name` is the metadata name); negative `replicas`;
- changing `spec.namespace` after creation.

**Observable states**:

| Reason | Meaning |
|--------|---------|
| `Ready` | Deployment `web` runs `image` with `replicas` in namespace `team-apps` |
| `MissingCustomNamespace` | No CustomNamespace named `spec.namespace` — retried until it exists; no Deployment created |
| `NameConflict` | Unmanaged Deployment `web` already in `team-apps` — left untouched |
| `Reconciling` | Transient; also used while repairing out-of-band drift |

## CustomConfig

```yaml
apiVersion: custom.jurikolo.name/v1
kind: CustomConfig
metadata:
  name: app-settings
  namespace: default
spec:
  namespace: team-apps   # required, immutable
  data:
    log_level: debug
  binaryData: {}         # optional, base64 in YAML
  immutable: false       # optional, mirrors ConfigMap semantics
```

Same admission rules, reasons, and semantics as CustomDeployment, producing a ConfigMap instead. Content parity with a regular ConfigMap: `data`, `binaryData`, `immutable` (+ standard metadata); `namespace` semantics replaced by the CustomNamespace reference.

## Cross-cutting contract

- Failure reasons are readable via `kubectl describe customdeployment` / `-config` / `-namespace` (condition `message`), satisfying FR-012.
- Deleting a CustomDeployment/CustomConfig deletes its real resource before the custom object disappears.
- Deleting a CustomNamespace succeeds only once its namespace is empty; `NamespaceDraining` is the visible blocked state (FR-004).
- The operator stamps real resources with label `custom.jurikolo.name/owner: <Kind>/<cr-namespace>/<cr-name>`; resources without this label are never modified by the operator.
