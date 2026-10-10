---
description: "Task list for Custom Namespace-Managed Resources"
---

# Tasks: Custom Namespace-Managed Resources

**Input**: Design documents from `/specs/001-custom-namespace-crds/`

**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Included per research.md D11 and constitution Principle I (one small test per non-trivial logic); envtest-based, written before the implementation they verify.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

Kubebuilder go/v4 layout (per plan.md): API types in `api/v1/`, reconcilers in `internal/controller/`, generated manifests in `config/`, entrypoint in `cmd/main.go`, docs in `docs/`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Scaffold the three APIs and controllers with kubebuilder so every later task edits real files.

- [x] T001 Scaffold three namespaced APIs + controllers: run `kubebuilder create api --group custom --version v1 --kind CustomNamespace --resource --controller`, then the same for `CustomDeployment` and `CustomConfig`. This generates `api/v1/groupversion_info.go` plus the three `api/v1/*_types.go`, controller stubs and `internal/controller/suite_test.go`, sample manifests in `config/samples/`, CRD bases in `config/crd/bases/`, and wires scheme registration + `SetupWithManager` into `cmd/main.go`.

**Checkpoint**: scaffold complete; all three kinds exist.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Verify the toolchain end-to-end before any story work.

- [x] T002 Verify green baseline: run `make manifests generate fmt vet build` and `make test` (envtest binaries via `make setup-envtest`); fix nothing feature-specific — the scaffolded suite must pass and CRDs must generate. If envtest cannot run, resolve before Phase 3.

**Checkpoint**: Foundation ready — user story implementation can now begin.

---

## Phase 3: User Story 1 — Create a managed namespace (Priority: P1) 🎯 MVP

**Goal**: A CustomNamespace in `default` manages a real namespace of the same name; reserved names rejected; deletion guarded.

**Independent Test**: `make install && make run`, apply a CustomNamespace → real namespace appears with `Ready=True`; `kube-*`/`default` names rejected; deleting with contents blocks (`NamespaceDraining`), deleting empty succeeds (quickstart.md Scenario 1).

### Tests for User Story 1

- [x] T003 [P] [US1] Write admission-validation tests in `internal/controller/customnamespace_controller_test.go`: creating a CustomNamespace named `kube-system`, `default`, and `kube-foo` in envtest fails at request time (CEL rule, spec FR-002).
- [x] T004 [US1] Write reconciler tests in `internal/controller/customnamespace_controller_test.go`: (a) CustomNamespace `team-a` → Namespace `team-a` exists + `Ready=True`; (b) Namespace deleted out-of-band → recreated on requeue; (c) delete CR while namespace holds a ConfigMap → CR stuck in Terminating with `Ready=False, Reason=NamespaceDraining`; (d) after namespace is emptied → namespace and CR both gone (FR-002/FR-003/FR-004).

### Implementation for User Story 1

- [x] T005 [US1] Define the CustomNamespace type in `api/v1/customnamespace_types.go`: no spec fields (object name is the interface); root-level CEL marker exactly `self.metadata.name != 'default' && !self.metadata.name.startsWith('kube-')` with message "name is reserved"; `status.conditions` as `[]metav1.Condition` with `Ready` type; add shared constants `FinalizerName = "custom.jurikolo.name/finalizer"` and `OwnerLabelKey = "custom.jurikolo.name/owner"` in `api/v1/groupversion_info.go` (data-model.md, D4/D6).
- [x] T006 [US1] Implement the reconciler in `internal/controller/customnamespace_controller.go`: RBAC marker `namespaces` with verbs `get;list;watch;create;delete`; ensure finalizer; create the real Namespace named after the object (adopt idempotently if it exists, D5); set `Ready=True` or `Reconciling`; on deletion: `Delete()` the Namespace, keep finalizer and set `Ready=False, Reason=NamespaceDraining` until the Namespace object is gone, then release (FR-001..FR-004, D5); return `RequeueAfter: 30s` (D7); make T003/T004 pass.
- [x] T007 [US1] Set sample `config/samples/customnamespace_v1_customnamespace.yaml` to `name: team-apps` in namespace `default` (contracts/crds.md).

**Checkpoint**: User Story 1 fully functional and independently testable (`make run` + quickstart Scenario 1).

---

## Phase 4: User Story 2 — Deploy an application into a managed namespace (Priority: P2)

**Goal**: CustomDeployment maps to a real Deployment in the referenced namespace; missing reference and name conflicts reported; cascade delete and drift repair work.

**Independent Test**: with a CustomNamespace present, apply a CustomDeployment → Deployment runs in that namespace with requested image/replicas; `MissingCustomNamespace`/`NameConflict` visible and harmless (quickstart.md Scenarios 2 and "Unmanaged name conflict").

### Tests for User Story 2

- [x] T008 [P] [US2] Write tests in `internal/controller/customdeployment_controller_test.go`: (a) reference to missing CustomNamespace → `Ready=False, Reason=MissingCustomNamespace`, no Deployment anywhere; creating the CustomNamespace afterwards → recovers to `Ready` and Deployment appears (FR-006, eventual consistency); (b) `replicas` omitted → 1; explicit `3` → 3; `0` → 0; (c) unmanaged Deployment of the same name in target namespace → `Reason=NameConflict`, existing Deployment byte-identical before/after (FR-013); (d) deleting the CR removes the Deployment (finalizer cascade); (e) out-of-band image change → repaired on requeue (FR-008); (f) patching `spec.namespace` after creation is rejected (CEL, FR-014).

### Implementation for User Story 2

- [x] T009 [US2] Define the CustomDeployment type in `api/v1/customdeployment_types.go` with validation markers quoting the constraints verbatim: `namespace` string `+kubebuilder:validation:Required` + field CEL `self == oldSelf` ("namespace is immutable"); `name` string `+kubebuilder:validation:Required`; `image` string `+kubebuilder:validation:Required`; `replicas` *int32 `+kubebuilder:default=1` `+kubebuilder:validation:Minimum=0`; `status.conditions` `Ready` as in T005 (FR-005, D2, D8).
- [x] T010 [US2] Implement the reconciler in `internal/controller/customdeployment_controller.go`: RBAC marker `apps/deployments` with `get;list;watch;create;update;patch;delete`; resolve `spec.namespace` against a CustomNamespace in the CR's own namespace → else `MissingCustomNamespace` + requeue (D3); before create, `Get` the target Deployment — exists without owner label value `CustomDeployment/<cr-namespace>/<cr-name>` → `NameConflict`, no writes; exists with our label → reconcile; create single-container Deployment (container name = deployment `name`, image only, default strategy, D8); update image/replicas on change; finalizer `FinalizerName` deletes the Deployment before release (D4); `RequeueAfter: 30s` (D7); make T008 pass.
- [x] T011 [P] [US2] Set sample `config/samples/customdeployment_v1_customdeployment.yaml` to: name `web`, namespace `default`, `spec.namespace: team-apps`, `image: nginx:1.27`, replicas omitted (contracts/crds.md).

**Checkpoint**: User Stories 1 AND 2 work independently.

---

## Phase 5: User Story 3 — Store configuration in a managed namespace (Priority: P3)

**Goal**: CustomConfig maps to a real ConfigMap with full content parity, same gating semantics as US2.

**Independent Test**: apply a CustomConfig referencing a managed namespace → ConfigMap appears with identical `data`/`binaryData`; missing reference and conflicts behave as in US2 (quickstart.md Scenario 3).

### Tests for User Story 3

- [x] T012 [P] [US3] Write tests in `internal/controller/customconfig_controller_test.go`: (a) missing reference → `MissingCustomNamespace`, no ConfigMap; recovers when CustomNamespace appears; (b) `data` and `binaryData` carried verbatim into the ConfigMap (FR-009/FR-011); (c) data change → ConfigMap updated on reconcile (FR-011); (d) unmanaged ConfigMap of the same name → `NameConflict`, untouched (FR-013); (e) deleting the CR removes the ConfigMap; (f) patching `spec.namespace` rejected (FR-014).

### Implementation for User Story 3

- [x] T013 [US3] Define the CustomConfig type in `api/v1/customconfig_types.go`: `namespace` string Required + immutable CEL `self == oldSelf`; `data` map[string]string; `binaryData` map[string][]byte; `immutable` *bool; `status.conditions` as before (FR-009, D9).
- [x] T014 [US3] Implement the reconciler in `internal/controller/customconfig_controller.go`: RBAC marker core `configmaps` CRUD verbs; same pattern as T010 — reference resolution, owner-label conflict check with value `CustomConfig/<cr-namespace>/<cr-name>`, create/update ConfigMap with verbatim content, finalizer cascade, `RequeueAfter: 30s`; make T012 pass.
- [x] T015 [P] [US3] Set sample `config/samples/customconfig_v1_customconfig.yaml` to: name `app-settings`, namespace `default`, `spec.namespace: team-apps`, `data: {log_level: debug}` (contracts/crds.md).

**Checkpoint**: All user stories independently functional.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Documentation (constitution III) and end-to-end verification.

- [x] T016 Write operating documentation per constitution Principle III: create `docs/operating.md` (commands and outcomes only: `make install/uninstall`, `make run`, `make test`, build/deploy via `IMG=` with podman) and link it from `README.md` with one line per CRD describing what it does.
- [x] T017 Run the full `specs/001-custom-namespace-crds/quickstart.md` against the dev cluster (all scenarios including failure paths) and `make test lint`; record any fix-ups needed as follow-up edits.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies — start immediately
- **Foundational (Phase 2)**: depends on T001 — blocks all stories
- **US1 (Phase 3)**: depends on Phase 2; defines the shared constants (T005) that US2/US3 use
- **US2 (Phase 4)**: depends on Phase 3 (constants, established reconciler pattern)
- **US3 (Phase 5)**: depends on Phase 3; mirrors US2 but only needs the constants — can run in parallel with Phase 4
- **Polish (Phase 6)**: depends on all stories

### User Story Dependencies

- **US1**: independent — the foundation story
- **US2**: consumes CustomNamespace objects created via US1; independently testable once one CustomNamespace exists
- **US3**: same shape as US2; independent of it

### Within Each User Story

- Tests (T003/T004, T008, T012) are written first and must fail (or be skipped-pending-CRD) before the corresponding type/reconciler tasks
- Type definitions before reconcilers (reconcilers use the generated types)
- Sample manifests last (they exercise the finished schema)

### Parallel Opportunities

- T008 alone; T012 alone (different test files); T003 → T004 share one test file and run sequentially
- Samples T007/T011/T015 are independent file edits
- After Phase 3 completes: Phase 4 and Phase 5 can proceed in parallel (different controllers, different files)

---

## Parallel Example: User Story 1

```bash
# US1 tests share one file and run in sequence (T003 then T004);
# parallelism starts within US2/US3 (T008, T011) and (T012, T015):
Task: "Tests in internal/controller/customdeployment_controller_test.go"
Task: "Sample config/samples/customdeployment_v1_customdeployment.yaml"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001)
2. Complete Phase 2: Foundational (T002)
3. Complete Phase 3: User Story 1 (T003–T007)
4. **STOP and VALIDATE**: quickstart.md Scenario 1 independently
5. Deploy/demo if ready

### Incremental Delivery

1. Setup + Foundational → toolchain proven
2. Add US1 → validate (MVP: managed namespaces)
3. Add US2 → validate (workloads)
4. Add US3 → validate (configuration)
5. Polish: docs + full quickstart pass

### Parallel Team Strategy

With multiple developers after Phase 3: Developer A takes Phase 4, Developer B takes Phase 5; merge order irrelevant (disjoint files except `cmd/main.go` wiring done at T001).

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Every task traces to a numbered FR in spec.md or a decision in research.md — no speculative tasks
- Commit after each task or logical group
- Stop at any checkpoint to validate a story independently

---

## Phase 7: Convergence

- [x] T018 Fix the broken sample-apply command in docs/operating.md (replace directory apply `kubectl apply -f config/samples/` with the three explicit sample files; the scaffolded samples/kustomization.yaml fails validation) per Constitution III / Development Workflow (partial)
- [x] T019 Correct the three sample manifest paths in specs/001-custom-namespace-crds/quickstart.md from `customnamespace_v1_…`/`customdeployment_v1_…`/`customconfig_v1_…` to the actual `custom_v1_…` filenames per T017 (partial)
