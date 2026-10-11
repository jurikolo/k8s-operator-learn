# Quickstart: Operator Logging, Visible Failures and Spec-Only Resources

Feature: [spec.md](./spec.md) · Contracts: [contracts/crds.md](./contracts/crds.md) · 2026-10-11

Validation scenarios proving the feature end-to-end on the local development cluster. Run from the repo root; `kubectl` must point at `dev-cluster` (`dev-admin@dev-cluster` context).

## Prerequisites

```sh
make manifests generate install   # CRDs with the new spec.name fields on the cluster
make run                          # operator in the foreground; keep this terminal visible
```

Automated checks first (no cluster needed beyond envtest assets, already in `bin/k8s`):

```sh
make test
```

## Scenario 1 — spec-only manifests work (US3 / SC-004)

The custom object's own name is arbitrary; the real resource follows `spec.name`.

```sh
kubectl apply -f - <<'EOF'
apiVersion: custom.jurikolo.name/v1
kind: CustomNamespace
metadata:
  name: anything-i-like
spec:
  name: team-apps
EOF
```

Expected: a real namespace `team-apps` appears; the CRD rejects an object whose `spec.name` is missing, reserved (`default`, `kube-…`), invalid (uppercase), or changed after creation — each with a clear message (see [contracts/crds.md](./contracts/crds.md)).

```sh
kubectl get ns team-apps
kubectl get customnamespace anything-i-like -o jsonpath='{.status.conditions}'
```

## Scenario 2 — creations and deletions are logged (US2 / SC-001)

With `make run` in view:

1. Create a CustomNamespace (Scenario 1) → the log shows `created namespace namespace=team-apps` with the owning object.
2. Delete it while the namespace is empty → the log shows `deleted namespace` before the object disappears.

```sh
kubectl delete customnamespace anything-i-like
```

Expected: one `created …` line per real resource created, one `deleted …` line per real resource removed, for all three kinds.

## Scenario 3 — a silent failure now reports itself (US1 / SC-005)

The spec's original repro: an object placed where the operator cannot satisfy it.

```sh
kubectl create ns orphan-place
kubectl apply -f - <<'EOF'
apiVersion: custom.jurikolo.name/v1
kind: CustomDeployment
metadata:
  name: misplaced
  namespace: orphan-place
spec:
  namespace: ghost
  name: web
  image: nginx:1.27
EOF
```

Expected: kubectl still says "created" (the custom object exists), but the object itself now tells the truth within one reconcile cycle (under a minute):

```sh
kubectl get customdeployment misplaced -n orphan-place -o jsonpath='{.status.conditions}'
# Ready=False, reason names the missing CustomNamespace "ghost"
```

The operator log shows the matching error line with the cause. After the blocker is removed (create CustomNamespace `ghost` in `orphan-place`), the object returns to `Ready=True` on its own (FR-004).

## Scenario 4 — duplicate claims resolve first-claim-wins (US3 / FR-009)

```sh
kubectl apply -f - <<'EOF'
apiVersion: custom.jurikolo.name/v1
kind: CustomNamespace
metadata:
  name: contender
spec:
  name: team-apps
EOF
```

Expected: `contender` reports `Ready=False, Reason=NameConflict` naming the existing owner; the original object stays `Ready`; the namespace is untouched.

## Scenario 5 — cluster-rejected create is visible (US1 / FR-003)

The `CreateFailed` path needs the cluster to refuse a create the CRD schema allowed — e.g. a CustomConfig whose `spec.namespace` points at a CustomNamespace that vanished between admission and reconcile, or a quota/admission block on the target namespace. Verify the condition appears on the object and clears once the create succeeds:

```sh
kubectl get customconfig <name> -o jsonpath='{.status.conditions}'
```

Expected: `Ready=False, Reason=CreateFailed` with the API error in the message, then `Ready=True` after the cause is fixed — no manual intervention.

## Cleanup

```sh
kubectl delete customnamespace,customdeployment,customconfig --all -n default
kubectl delete ns orphan-place --ignore-not-found
```

Note: deleting a CustomNamespace whose namespace still holds resources blocks until emptied (`NamespaceDraining`) — that is 001 behavior, unchanged.
