# Contracts: CRD Schemas (deltas from 001)

Feature: [spec.md](./spec.md) · Decisions: [research.md](./research.md) · 2026-10-11

Group `custom.jurikolo.name/v1`, all kinds namespaced. Only changed parts of the schemas are listed; everything else is as shipped in 001 (`config/crd/bases` after `make manifests` is the source of truth).

## customnamespaces.custom.jurikolo.name

```yaml
spec:
  properties:
    name:                          # NEW, required
      type: string
      maxLength: 63
      pattern: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
      x-kubernetes-validations:
        - rule: "self == oldSelf"
          message: "name is immutable"
  required: [name]
  x-kubernetes-validations:        # rule moved from metadata.name to spec.name
    - rule: "self.spec.name != 'default' && !self.spec.name.startsWith('kube-')"
      message: "name is reserved"
```

Behavioral contract:

- Real Namespace is created with name `spec.name` and owner label `custom.jurikolo.name/owner: CustomNamespace/<cr-ns>/<cr-name>`.
- Existing Namespace with a matching owner label is adopted (idempotent); with a different or absent label the object reports `Ready=False, Reason=NameConflict` and the Namespace is untouched.

## customdeployments.custom.jurikolo.name

```yaml
spec:
  properties:
    name:                          # existing field, NEW validation
      x-kubernetes-validations:
        - rule: "self == oldSelf"
          message: "name is immutable"
```

Behavioral contract: unchanged identity mapping; adds `Ready=False, Reason=CreateFailed` when the real Deployment create is rejected.

## customconfigs.custom.jurikolo.name

```yaml
spec:
  properties:
    name:                          # NEW, required
      type: string
      maxLength: 253
      pattern: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$"
      x-kubernetes-validations:
        - rule: "self == oldSelf"
          message: "name is immutable"
  required: [name]
```

Behavioral contract:

- Real ConfigMap is created with name `spec.name` (was: object's `metadata.name`) in namespace `spec.namespace`.
- Conflict detection unchanged, now keyed on `spec.name`.

## Rejection messages (FR-008)

| Violation | Rejected at | Message |
|-----------|-------------|---------|
| missing `spec.name` (CustomNamespace/CustomConfig) | admission | `spec.name: Required value` |
| `spec.name` change after create (all kinds) | admission | `name is immutable` |
| reserved name `default` / `kube-` prefix | admission | `name is reserved` |
| name not matching RFC 1123 | admission | pattern failure |
| missing other required spec fields | admission | unchanged 001 behavior |

## Log contract (FR-001/002/010)

- Creation: `created <kind>` with `<kind>`, `namespace`, `<custom-kind>=<cr-ns>/<cr-name>` arguments — one line per real resource, at Info level.
- Deletion: `deleted <kind>` with the same identity arguments, emitted when the finalizer cleanup removes the real resource.
- Errors: `<operation> failed` at Error level with `err` plus resource identity, emitted at each failed API operation in the reconcilers.
