# Quickstart: Custom Namespace-Managed Resources

Validation guide for [spec.md](./spec.md); API details in [contracts/crds.md](./contracts/crds.md). Commands assume repo root.

## Prerequisites

- Go ≥ 1.26, `make`, `kubectl`
- A local Kubernetes development cluster reachable via kubeconfig (project convention: dev-cluster kubeconfig), Kubernetes ≥ 1.25 (CEL `oldSelf`)
- For containerized deploy: podman (Docker-compatible)

## Install & run

```sh
make install                      # install CRDs into the cluster from kubeconfig
make run                          # run the operator in the foreground from your host
```

## Scenario 1 — managed namespace (Story P1)

```sh
kubectl apply -f config/samples/custom_v1_customnamespace.yaml   # sample: name team-apps
kubectl get namespace team-apps                                           # exists within ~30 s
kubectl get customnamespace team-apps -n default -o jsonpath='{.status.conditions}'
```

Expected: `Ready=True`.

Failure paths:

```sh
kubectl create -f - <<'EOF'
apiVersion: custom.jurikolo.name/v1
kind: CustomNamespace
metadata:
  name: kube-system
  namespace: default
EOF
```

Expected: request rejected (reserved name).

Deletion guard: create any resource inside `team-apps` (e.g. `kubectl create configmap keep -n team-apps`), then `kubectl delete customnamespace team-apps -n default` → object hangs in `Terminating` with `NamespaceDraining`; delete the configmap → namespace and custom object then disappear.

## Scenario 2 — deployment in a managed namespace (Story P2)

```sh
kubectl apply -f config/samples/custom_v1_customdeployment.yaml
kubectl get deployment -n team-apps                # 1 replica running the sample image
kubectl patch customdeployment web -n default --type merge -p '{"spec":{"replicas":3}}'
kubectl get deployment web -n team-apps            # 3 replicas
```

Failure paths:

```sh
kubectl create -f - <<'EOF'
apiVersion: custom.jurikolo.name/v1
kind: CustomDeployment
metadata:
  name: ghost-dep
  namespace: default
spec:
  namespace: ghost          # no such CustomNamespace
  name: ghost-dep
  image: nginx:1.27
EOF
kubectl get deployment -A | grep ghost   # empty
kubectl describe customdeployment ghost-dep -n default   # MissingCustomNamespace + message
```

Then `kubectl create ns`-style fix — apply a CustomNamespace named `ghost` → the CustomDeployment recovers to `Ready` on the next pass.

Immutable reference: `kubectl patch` changing `spec.namespace` is rejected.

## Scenario 3 — config in a managed namespace (Story P3)

```sh
kubectl apply -f config/samples/custom_v1_customconfig.yaml
kubectl get configmap app-settings -n team-apps -o yaml    # data matches the sample
kubectl patch customconfig app-settings -n default --type merge -p '{"spec":{"data":{"log_level":"info"}}}'
kubectl get configmap app-settings -n team-apps -o jsonpath='{.data}'   # updated
```

Failure path mirrors Scenario 2 (`MissingCustomNamespace`, `NameConflict` if an unmanaged ConfigMap of that name exists).

## Drift repair (SC-005)

```sh
kubectl delete deployment web -n team-apps
# within ~60 s: kubectl get deployment web -n team-apps   # recreated by the operator
```

## Unmanaged name conflict (FR-013)

```sh
kubectl create deployment rogue --image=nginx -n team-apps
# apply a CustomDeployment named `rogue` → Ready=False, Reason=NameConflict; the kubectl-created Deployment is untouched
```

## Cleanup

```sh
kubectl delete customdeployment --all -n default
kubectl delete customconfig --all -n default
kubectl delete customnamespace --all -n default   # only empty namespaces delete
make uninstall
```

## Test suite

```sh
make test     # unit/controller tests via envtest
```
