# Implementation Plan: Operator Logging, Visible Failures and Spec-Only Resources

**Branch**: `002-logs-and-feedback` | **Date**: 2026-10-11 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/002-logs-and-feedback/spec.md`

## Summary

Evolve the existing three CRDs so that (1) the real resource's identity lives in `spec.name` — CustomNamespace and CustomConfig gain a required immutable `name` field, and the custom object's own `metadata.name` becomes ignored platform plumbing (CustomDeployment already works this way); (2) every real-resource creation, deletion and failure is written to the operator log with resource identity and cause; (3) every failed operation is reported on the custom object as a `Ready=False` status condition, and two objects claiming the same real resource resolve as first-claim-wins (second object reports `NameConflict`). Namespace creation gains the same owner-label conflict detection Deployments already have. Schema-level rules (required/immutable `name`, reserved names, RFC 1123 naming) stay in CRD validation — no webhooks.

## Technical Context

**Language/Version**: Go 1.26 (go.mod `go 1.26.0`; toolchain 1.27 installed)

**Primary Dependencies**: sigs.k8s.io/controller-runtime v0.25, k8s.io/api + apimachinery + client-go v0.37, kubebuilder v4 (`go.kubebuilder.io/v4` layout), ginkgo/gomega (existing test scaffolding)

**Storage**: Kubernetes API server (CRs stored as CRDs; no external storage)

**Testing**: `make test` (Go test + envtest 1.37.0 assets in `bin/k8s`); ginkgo e2e suite under `test/e2e`

**Target Platform**: Linux manager binary/container; local development cluster via kubeconfig (`dev-admin@dev-cluster`)

**Project Type**: Kubernetes operator (kubebuilder)

**Performance Goals**: unchanged from 001 — reconcile effects visible within the 30 s requeue interval (spec SC-002 bound: under a minute)

**Constraints**: minimal code and kubebuilder-idiomatic readability (constitution I + II); no production hardening; no new dependencies

**Scale/Scope**: 2 API types edited (CustomNamespace, CustomConfig), 3 reconcilers edited, regenerated CRD/deepcopy manifests, samples + docs updated, one test suite per controller extended

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate | Status |
|-----------|------|--------|
| I. Minimal Code (Ponytail) | Plan adds only what FR-001..FR-010 need: two spec fields + markers, spec-name lookups in reconcilers, one owner-label check for namespaces, explicit log calls at existing error/return sites, `CreateFailed` conditions on existing create paths. No new deps, components, watches or abstractions | PASS |
| II. Learning Purpose | Demonstrates: CEL field validation (immutable + reserved + pattern), ownership labels as cross-object conflict detection, structured controller logging (`logf`), condition-based error reporting — all kubebuilder idioms, readable in one pass | PASS |
| III. Living Documentation | quickstart.md produced in Phase 1; `docs/operating.md` + README updates are implement-phase tasks per Development Workflow | PASS (deferred to implement) |
| Development Workflow | Following spec-kit flow; verification = build + tests + documented commands still work | PASS |

Post-Phase-1 re-check: design introduces no additional mechanisms (see research.md D1–D6) — **PASS**.

## Project Structure

### Documentation (this feature)

```text
specs/002-logs-and-feedback/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── crds.md          # Phase 1 output
└── tasks.md             # Phase 2 output ($speckit-tasks - NOT created here)
```

### Source Code (repository root)

```text
api/v1/
├── customnamespace_types.go   # spec.Name field + validation markers; reserved-name CEL moves to spec.name
├── customconfig_types.go      # spec.Name field + validation markers
├── customdeployment_types.go  # spec.name gains immutable CEL marker (FR-007)
└── zz_generated.deepcopy.go   # regenerated (make generate)

internal/controller/
├── customnamespace_controller.go   # spec.name identity, owner label + NameConflict, create/delete/error logs, CreateFailed condition
├── customdeployment_controller.go  # create/delete/error logs, CreateFailed condition
├── customconfig_controller.go      # spec.name identity, create/delete/error logs, CreateFailed condition
├── common.go                       # unchanged
└── *_test.go                       # fixtures move to spec.name; new collision/admission tests

config/
├── crd/bases/               # regenerated (make manifests)
└── samples/                 # updated: arbitrary metadata.name, identity in spec

docs/operating.md + README.md # updated manifests, expected log lines, conflict behavior
```

**Structure Decision**: kubebuilder single-project layout, unchanged from 001; all changes land in existing files.

## Complexity Tracking

> No constitution violations — table left empty.
