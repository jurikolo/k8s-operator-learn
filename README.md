# k8s-operator-learn

Custom Kubernetes operator (kubebuilder, Go, API group `custom.jurikolo.name/v1`) built for learning operator concepts.

Three CRDs, all created in the `default` namespace. The real resource's name lives in `spec.name`; the custom object's own name is arbitrary and ignored.

- **CustomNamespace** — manages the real namespace named by `spec.name`; deleting it is blocked while the namespace still holds resources.
- **CustomDeployment** — creates a real Deployment (name, image, replicas) in a namespace managed by a CustomNamespace.
- **CustomConfig** — creates a real ConfigMap with identical content under the same CustomNamespace gating.

Failures are visible: a failed create reports `Ready=False, Reason=CreateFailed` on the object, two objects claiming the same name resolve first-claim-wins (`NameConflict` on the loser), and every create/delete/failure is written to the operator log.

**Breaking change vs 001**: manifests must set `spec.name` — the object's own name no longer names the real resource.

How to build, test, run, and deploy: [docs/operating.md](docs/operating.md). Design docs for the current feature: [specs/002-logs-and-feedback/](specs/002-logs-and-feedback/).
