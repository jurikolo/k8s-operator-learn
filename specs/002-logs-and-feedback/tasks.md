# Tasks: Operator Logging, Visible Failures and Spec-Only Resources

**Input**: Design documents from `/specs/002-logs-and-feedback/`

**Prerequisites**: [plan.md](./plan.md) · [spec.md](./spec.md) · [research.md](./research.md) · [data-model.md](./data-model.md) · [contracts/crds.md](./contracts/crds.md) · [quickstart.md](./quickstart.md)

**Tests**: Included — the spec's acceptance scenarios are testable in envtest (admission rules, conditions, collision behavior). Log-line formatting is verified live per [quickstart.md](./quickstart.md), not asserted in tests (research D6).

**Organization**: Tasks grouped by user story. The API schema change and identity switch sit in Foundational because every story — including US1's collision test — depends on spec-carried identity.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1 = visible failures, US2 = logging, US3 = spec-only resources)
- Exact file paths included in every task

## Path Conventions

kubebuilder single-project layout (per plan.md): API types in `api/v1/`, reconcilers + tests in `internal/controller/`, generated manifests in `config/`, docs in `docs/`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Confirm the starting point is healthy before touching anything.

- [ ] T001 Run `make test` and confirm the 001 baseline suite is green; record the test count before changes

**Checkpoint**: Baseline known — no changes made yet.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Spec-carried identity must exist before any user story: the new fields are required by validation (old fixtures would fail admission) and US1's collision test needs the reconcilers to key on `spec.name`.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [ ] T002 Add `Name string` to `CustomNamespaceSpec` in api/v1/customnamespace_types.go with markers per contracts/crds.md: `+kubebuilder:validation:Required`, `MaxLength=63`, `Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`, `XValidation:rule="self == oldSelf",message="name is immutable"`; move the root-level reserved-name CEL rule from `self.metadata.name` to `self.spec.name` (`!= 'default' && !startsWith('kube-')`, message "name is reserved")
- [ ] T003 [P] Add `Name string` to `CustomConfigSpec` in api/v1/customconfig_types.go with markers: `Required`, `MaxLength=253`, `Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`, `XValidation:rule="self == oldSelf",message="name is immutable"`
- [ ] T004 [P] Add `XValidation:rule="self == oldSelf",message="name is immutable"` to `CustomDeploymentSpec.Name` in api/v1/customdeployment_types.go
- [ ] T005 Run `make manifests generate` to regenerate config/crd/bases and zz_generated.deepcopy.go
- [ ] T006 Switch CustomNamespace reconciler identity to `cn.Spec.Name` in internal/controller/customnamespace_controller.go: the `Get`/`Create`/`namespaceEmpty` object keys and log arguments stop reading `cn.Name` (owner-label bookkeeping keeps CR identity)
- [ ] T007 [P] Switch CustomConfig reconciler identity to `cc.Spec.Name` in internal/controller/customconfig_controller.go: ConfigMap `Get`/`Create` names come from `cc.Spec.Name`; owner label unchanged
- [ ] T008 Update existing fixtures/assertions in internal/controller/customnamespace_controller_test.go, customdeployment_controller_test.go, customconfig_controller_test.go to set `spec.name` on every custom object and assert real-resource names from `spec.name`; run `make test` — suite must be green with the count from T001 (admission rejections from the new markers are the only expected failure mode if a fixture is missed)

**Checkpoint**: `make test` green; real-resource identity comes from `spec.name`; behavior otherwise unchanged.

---

## Phase 3: User Story 1 — A failed creation is visible to the user (Priority: P1) 🎯 MVP

**Goal**: Every failure that prevents a real resource from existing shows up as a `Ready=False` condition on the custom object — including the previously silent cases: rejected creates and duplicate claims.

**Independent Test**: Create a CustomDeployment referencing a missing CustomNamespace → object reports the failure; create two CustomNamespace objects with the same `spec.name` → second reports `NameConflict`, first stays `Ready`.

### Implementation for User Story 1

- [ ] T009 [US1] Add namespace ownership conflict detection in internal/controller/customnamespace_controller.go: stamp `custom.jurikolo.name/owner: CustomNamespace/<cr-ns>/<cr-name>` (existing `ownerRef()` helper) on created Namespaces; when the target Namespace already exists, adopt only if the label matches, otherwise set `Ready=False, Reason=NameConflict` with a message naming the conflict and the existing owner and leave the namespace untouched (research D3)
- [ ] T010 [P] [US1] On failed real-resource Create in internal/controller/customnamespace_controller.go, set `Ready=False, Reason=CreateFailed, Message=<error>` via `setCondition` before returning the error (research D4)
- [ ] T011 [P] [US1] Same `CreateFailed` condition on failed Create in internal/controller/customdeployment_controller.go
- [ ] T012 [P] [US1] Same `CreateFailed` condition on failed Create in internal/controller/customconfig_controller.go
- [ ] T013 [US1] Add collision test in internal/controller/customnamespace_controller_test.go: two CustomNamespace objects with different object names and the same `spec.name` → first `Ready=True`, second `Ready=False` with `Reason=NameConflict`; plus a pre-existing unmanaged namespace (no owner label) under the claimed name → `NameConflict`, namespace contents intact

**Checkpoint**: US1 independently testable — failed creates and duplicate claims report themselves; `make test` green.

---

## Phase 4: User Story 2 — Operator activity is logged (Priority: P2)

**Goal**: The operator log shows one line per real-resource creation and deletion (kind, name, namespace, owning custom object) and an error line with cause at every failed operation.

**Independent Test**: With `make run` in view, create and delete one custom resource of each kind; the log shows `created …` and `deleted …` lines; forcing a failure produces an error line (quickstart.md Scenarios 2/3).

### Implementation for User Story 2

Tasks T014–T016 touch the same three reconciler files — run sequentially.

- [ ] T014 [US2] Extend the three existing creation `log.Info` calls ("created namespace/deployment/configmap") in internal/controller/customnamespace_controller.go, customdeployment_controller.go, customconfig_controller.go with an owning-object argument: `"<kind>", "<cr-ns>/<cr-name>"` (FR-001)
- [ ] T015 [US2] Add `log.Info("deleted <kind>", identity…)` at the three finalizer-cleanup `Delete` sites (namespace in customnamespace_controller.go, deployment in customdeployment_controller.go, configmap in customconfig_controller.go) with the same identity arguments (FR-010)
- [ ] T016 [US2] Add `log.Error(err, "<operation> failed", identity…)` at each error-return site in the three reconcilers (get/create/update/delete/status-write), keeping the returned error unchanged (FR-002, research D5)

**Checkpoint**: US2 independently testable — full create/delete/error trail visible in `make run` output; `make test` still green (no behavior change).

---

## Phase 5: User Story 3 — Custom resources require only a spec (Priority: P3)

**Goal**: Manifests carry identity in `spec.name` with arbitrary object names; schema validation rejects missing/changed/reserved/invalid names with clear messages.

**Independent Test**: Apply a CustomNamespace with object name `anything` and `spec.name: team-apps` → real namespace `team-apps` appears; submit one missing `spec.name`, one changed after create, one reserved, one invalid → each rejected with the message from contracts/crds.md.

### Implementation for User Story 3

- [ ] T017 [US3] Update config/samples/: CustomNamespace and CustomConfig samples set `spec.name` (object `metadata.name` arbitrary and distinct from it); CustomDeployment sample unchanged except a comment noting `spec.name` is immutable
- [ ] T018 [US3] Add admission tests in internal/controller/customnamespace_controller_test.go and internal/controller/customconfig_controller_test.go: create rejected when `spec.name` missing; `spec.name` update rejected with "name is immutable"; reserved names (`default`, `kube-foo`) rejected with "name is reserved"; pattern-invalid names (uppercase) rejected; `spec.name` change on CustomDeployment rejected with "name is immutable" (FR-007/FR-008)
- [ ] T019 [US3] Add independence test in internal/controller/customnamespace_controller_test.go: two CustomNamespace objects with arbitrary distinct object names and `spec.name` values `team-apps` and `team-b` → both real namespaces exist under their spec names (US3 acceptance 2)

**Checkpoint**: All three stories independently functional; `make test` green.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Documentation (constitution III) and end-to-end verification.

- [ ] T020 [P] Update docs/operating.md and README.md: spec-carried identity with example manifests, the log lines to expect (created/deleted/error), first-claim-wins behavior, and a breaking-change note for 001-style objects (object name no longer names the namespace — set `spec.name`)
- [ ] T021 Run quickstart.md Scenarios 1–4 against dev-cluster (`make install` + `make run`): spec-only apply, creation/deletion logs, the misplaced-object repro reporting `MissingCustomNamespace`, duplicate-claim `NameConflict`; note Scenario 5 (`CreateFailed`) result if forceable
- [ ] T022 Final verification: `make test`, `make build`, and every command in docs/operating.md works as written (constitution Development Workflow)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: none — run first
- **Foundational (Phase 2)**: depends on Setup; **blocks all user stories** (T008's fixture update is what makes the suite green under the new schema)
- **US1 (Phase 3)**: depends on Foundational
- **US2 (Phase 4)**: depends on US1 — T016 writes `log.Error` at the same error-return sites T010–T012 just added conditions to (same files, sequential)
- **US3 (Phase 5)**: depends on Foundational; could run in parallel with US1 by a second implementer (different concerns), but after US1 in priority order
- **Polish (Phase 6)**: depends on all user stories

### User Story Dependencies

- **US1**: Foundational only — collision test relies on T006's identity switch
- **US2**: US1 (same-file sequencing); no functional dependency — logging works regardless of conflict detection
- **US3**: Foundational only — admission tests need T002–T005

### Parallel Opportunities

- Foundational: T003 ∥ T004 (independent API files); T006 ∥ T007
- US1: T010 ∥ T011 ∥ T012 (three different files); T009 before T013 (test needs the branch)
- US3: T017 ∥ T018 ∥ T019 are independent file scopes except T018/T019 share customnamespace_controller_test.go — T019 after T018
- Polish: T020 ∥ everything in Phase 6 start

---

## Parallel Example: User Story 1

```bash
# After T009 lands, the three CreateFailed conditions are independent files:
Task: "CreateFailed condition in internal/controller/customnamespace_controller.go"
Task: "CreateFailed condition in internal/controller/customdeployment_controller.go"
Task: "CreateFailed condition in internal/controller/customconfig_controller.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1 → Phase 2 (identity in spec, suite green)
2. Phase 3: failures report themselves → **STOP and VALIDATE** (`make test` + quickstart Scenario 3/4)
3. The silent-failure complaint — the reason this feature exists — is resolved here

### Incremental Delivery

1. Foundational → spec-carried identity, no behavior change
2. US1 → visible failures (MVP)
3. US2 → full log trail
4. US3 → spec-only ergonomics + admission guarantees
5. Polish → docs + live verification

### Requirements Traceability

| Requirement | Tasks |
|-------------|-------|
| FR-001 creation logs | T014 |
| FR-002 error logs | T016 |
| FR-003/FR-004 failed-state + recovery | T010, T011, T012 |
| FR-005/FR-006 spec-only, metadata ignored | T002–T008, T017 |
| FR-007 identity immutable | T002, T003, T004, T018 |
| FR-008 clear validation messages | T018 |
| FR-009 first claim wins | T009, T013 |
| FR-010 deletion logs | T015 |
| SC-001–SC-005 (end-to-end) | T021, T022 |

## Notes

- Commit after each task or logical group; `make test` after every phase checkpoint
- Log-line formatting is verified live (quickstart), not asserted in tests — do not add log-capture plumbing
- `CreateFailed` has no deterministic envtest test (research D6) — quickstart Scenario 5 covers it
- Avoid: new dependencies, webhooks, events, log-capture test harnesses — all explicitly rejected in research.md
