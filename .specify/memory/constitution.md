# k8s-operator-learn Constitution

## Core Principles

### I. Minimal Code — Ponytail (NON-NEGOTIABLE)
The best code is the code never written. Every change is the smallest complete change that solves the task: reuse what exists (codebase, standard library, installed dependency) before adding anything; no abstraction, wrapper, option, config or "for later" code nobody asked for; deletion beats addition; the shortest working diff wins. Be lazy about the solution, never about the change itself — finish every caller, test and fixture the change breaks. New non-trivial logic (branch, loop, parser, money or security) ships with one small test. Known shortcuts get a `shortcut: <limit>, <when to upgrade>` comment.
Never cut: validation at trust boundaries, error handling that prevents data loss, security, accessibility, anything the user asked for. Applies to specs and plans too: smallest spec/plan that satisfies the requirement, every task traces to a requirement.
Source: [Ponytail](https://github.com/DietrichGebert/ponytail) (MIT); ruleset in [AGENTS.md](../../AGENTS.md).

### [PRINCIPLE_2_NAME]
<!-- Example: II. CLI Interface -->
[PRINCIPLE_2_DESCRIPTION]
<!-- Example: Every library exposes functionality via CLI; Text in/out protocol: stdin/args → stdout, errors → stderr; Support JSON + human-readable formats -->

### [PRINCIPLE_3_NAME]
<!-- Example: III. Test-First (NON-NEGOTIABLE) -->
[PRINCIPLE_3_DESCRIPTION]
<!-- Example: TDD mandatory: Tests written → User approved → Tests fail → Then implement; Red-Green-Refactor cycle strictly enforced -->

### [PRINCIPLE_4_NAME]
<!-- Example: IV. Integration Testing -->
[PRINCIPLE_4_DESCRIPTION]
<!-- Example: Focus areas requiring integration tests: New library contract tests, Contract changes, Inter-service communication, Shared schemas -->

### [PRINCIPLE_5_NAME]
<!-- Example: V. Observability, VI. Versioning & Breaking Changes, VII. Simplicity -->
[PRINCIPLE_5_DESCRIPTION]
<!-- Example: Text I/O ensures debuggability; Structured logging required; Or: MAJOR.MINOR.BUILD format; Or: Start simple, YAGNI principles -->

## [SECTION_2_NAME]
<!-- Example: Additional Constraints, Security Requirements, Performance Standards, etc. -->

[SECTION_2_CONTENT]
<!-- Example: Technology stack requirements, compliance standards, deployment policies, etc. -->

## [SECTION_3_NAME]
<!-- Example: Development Workflow, Review Process, Quality Gates, etc. -->

[SECTION_3_CONTENT]
<!-- Example: Code review requirements, testing gates, deployment approval process, etc. -->

## Governance
<!-- Example: Constitution supersedes all other practices; Amendments require documentation, approval, migration plan -->

[GOVERNANCE_RULES]
<!-- Example: All PRs/reviews must verify compliance; Complexity must be justified; Use [GUIDANCE_FILE] for runtime development guidance -->

**Version**: 1.0.0 | **Ratified**: 2026-10-10 | **Last Amended**: 2026-10-10
<!-- Principles II+ are still template placeholders; complete them with /speckit-constitution -->
<!-- Example: Version: 2.1.1 | Ratified: 2025-06-13 | Last Amended: 2025-07-16 -->
