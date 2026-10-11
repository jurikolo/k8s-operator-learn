# Operating the k8s-operator-learn operator

Commands assume repo root, Go ≥ 1.26, and a Kubernetes cluster reached via kubeconfig (Kubernetes ≥ 1.25 for CEL validation rules).

## Build and test

```sh
make build                # generate manifests + deepcopy, build bin/manager
make test                 # envtest-based controller tests (binaries: make setup-envtest)
make lint                 # golangci-lint
```

## Run against a cluster

```sh
make install              # install CRDs (custom.jurikolo.name/v1)
make run                  # run the operator in the foreground
kubectl apply -f config/samples/custom_v1_customnamespace.yaml
kubectl apply -f config/samples/custom_v1_customdeployment.yaml   # needs the CustomNamespace first
kubectl apply -f config/samples/custom_v1_customconfig.yaml
make uninstall            # remove CRDs
```

## Deploy as a container (podman)

```sh
make docker-build IMG=controller:latest
# load into your cluster, then:
make deploy IMG=controller:latest
make undeploy
```

## What the CRDs do

Identity lives in `spec.name` on all three kinds — the custom object's own `metadata.name` is arbitrary and ignored:

```yaml
apiVersion: custom.jurikolo.name/v1
kind: CustomNamespace
metadata:
  name: anything-i-like        # ignored by the operator
spec:
  name: team-apps              # the real namespace; immutable, reserved names rejected
```

**Breaking change vs 001**: the object's name no longer names the real resource. 001-style objects (no `spec.name`) are rejected at admission — add `spec.name` when re-applying old manifests.

- `CustomNamespace` (in `default`): manages the real namespace named by `spec.name`; reserved names (`default`, `kube-*`) rejected; deletion drains and is blocked while the namespace holds resources.
- `CustomDeployment` (in `default`): creates a real Deployment (`spec.name`, `spec.image`, `spec.replicas` default 1) inside the real namespace managed by the referenced CustomNamespace (`spec.namespace` names that custom object; its `spec.name` is the real namespace) and is immutable.
- `CustomConfig` (in `default`): same as a ConfigMap (`data`, `binaryData`, `immutable`), named by `spec.name`, created inside the real namespace managed by the referenced CustomNamespace under the same reference rules.

**First claim wins**: the first custom object to create a real resource owns it (owner label `custom.jurikolo.name/owner`). A second object claiming the same name reports `Ready=False, Reason=NameConflict` and never takes the resource over; deleting a loser leaves the winner's resource untouched.

Status: each kind reports a `Ready` condition (`Ready`, `MissingCustomNamespace`, `NameConflict`, `CreateFailed`, `NamespaceDraining`, `Reconciling`). Check with `kubectl describe <kind> <name> -n default`.

## Log lines

With `make run` in the foreground, expect one line per real-resource change:

```text
created namespace namespace=team-apps CustomNamespace=default/anything-i-like
deleted deployment deployment=web namespace=team-apps CustomDeployment=default/web
create failed deployment=web namespace=ghost error="..."   # Error level, cause included
```

`created …` / `deleted …` at Info level carry the real resource's kind, name, namespace and the owning custom object; every failed API operation logs `<operation> failed` at Error level with the cause.
