# k8s-operator-learn

Custom Kubernetes operator (kubebuilder, Go, API group `custom.jurikolo.name/v1`) built for learning operator concepts.

Three CRDs, all created in the `default` namespace:

- **CustomNamespace** — manages a real namespace of the same name; deleting it is blocked while the namespace still holds resources.
- **CustomDeployment** — creates a real Deployment (name, image, replicas) in a namespace managed by a CustomNamespace.
- **CustomConfig** — creates a real ConfigMap with identical content under the same CustomNamespace gating.

How to build, test, run, and deploy: [docs/operating.md](docs/operating.md). Design docs for the current feature: [specs/001-custom-namespace-crds/](specs/001-custom-namespace-crds/).
