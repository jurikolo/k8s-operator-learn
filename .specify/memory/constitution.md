<!--
SYNC IMPACT REPORT
==================
Version change: 1.0.0 → 1.1.0 (MINOR: new principles II–III, new sections, governance filled in)

Modified principles: none renamed; Principle I unchanged
Added:
  - Principle II. Learning Purpose
  - Principle III. Living Documentation
  - Section: Project Scope
  - Section: Development Workflow
  - Governance rules filled in
Removed: template placeholder comments for principles II+

Deferred TODOs: none
-->
# k8s-operator-learn Constitution

## Core Principles

### I. Minimal Code — Ponytail (NON-NEGOTIABLE)
The best code is the code never written. Every change is the smallest complete change that solves the task: reuse what exists (codebase, standard library, installed dependency) before adding anything; no abstraction, wrapper, option, config or "for later" code nobody asked for; deletion beats addition; the shortest working diff wins. Be lazy about the solution, never about the change itself — finish every caller, test and fixture the change breaks. New non-trivial logic (branch, loop, parser, money or security) ships with one small test. Known shortcuts get a `shortcut: <limit>, <when to upgrade>` comment.
Never cut: validation at trust boundaries, error handling that prevents data loss, security, accessibility, anything the user asked for. Applies to specs and plans too: smallest spec/plan that satisfies the requirement, every task traces to a requirement.
Source: [Ponytail](https://github.com/DietrichGebert/ponytail) (MIT); ruleset in [AGENTS.md](../../AGENTS.md).

### II. Learning Purpose
This project exists to learn how a Kubernetes operator works, not to serve production. Every feature MUST demonstrate an operator concept (CRDs, reconciliation, code generation, deployment) and MUST stay readable: explicit kubebuilder idioms over cleverness, code a learner can follow in one read. Production-grade hardening (HA, autoscaling, multi-tenancy) is out of scope unless it is itself the lesson. When minimal code (Principle I) and teaching value conflict, keep the smallest version that still shows the concept clearly.

### III. Living Documentation
The project MUST carry minimalistic technical documentation that lets a consumer operate it: how to run tests, generate manifests, build the code, and deploy the operator to a cluster. This documentation lives in markdown file(s) outside README.md and is updated with every functional change that affects those steps. README.md MUST be updated continuously so a consumer can see at a glance what the project offers and how to benefit from it; it links to the technical docs instead of duplicating them. Documentation stays minimalistic: commands and outcomes, no narrative or theory.

## Project Scope
A kubebuilder-generated Go operator (API domain `jurikolo.name`) built for learning, targeting a local development cluster via kubeconfig. Features are added to demonstrate operator concepts, one concept at a time. The project is not production-hardened and makes no compatibility promises between learning iterations.

## Development Workflow
Features follow the spec-kit flow (`/speckit-specify` → `/speckit-plan` → `/speckit-tasks` → `/speckit-implement`); small fixes may skip straight to implementation. Documentation updates (Principle III) are part of "done" for every functional change, not a follow-up task. Verification before completion: code builds, tests pass, and any documented command in the docs still works as written.

## Governance
This constitution supersedes all other project practices. Amendments require an updated version (MAJOR: principle removal/redefinition, MINOR: new principle or materially expanded guidance, PATCH: wording), a Sync Impact Report, and a refreshed Last Amended date. All reviews and implementations MUST verify compliance with the three principles; unjustified complexity is a blocking finding. Runtime development guidance lives in [AGENTS.md](../../AGENTS.md).

**Version**: 1.1.0 | **Ratified**: 2026-10-10 | **Last Amended**: 2026-10-10
