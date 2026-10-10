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

- `CustomNamespace` (in `default`): manages a real namespace of the same name; reserved names (`default`, `kube-*`) rejected; deletion drains and is blocked while the namespace holds resources.
- `CustomDeployment` (in `default`): creates a real Deployment (`spec.name`, `spec.image`, `spec.replicas` default 1) inside the namespace named by `spec.namespace`, which must reference an existing CustomNamespace and is immutable.
- `CustomConfig` (in `default`): same as a ConfigMap (`data`, `binaryData`, `immutable`), created in the namespace named by `spec.namespace` under the same reference rules; the ConfigMap is named after the custom object.

Status: each kind reports a `Ready` condition (`Ready`, `MissingCustomNamespace`, `NameConflict`, `NamespaceDraining`, `Reconciling`). Check with `kubectl describe <kind> <name> -n default`.
