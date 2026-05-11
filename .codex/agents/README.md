# Checkout Workbench Multi-Agent System

Last verified: 2026-05-11

This directory defines the project-local multi-role agent system used to review, plan, and coordinate automation work for Checkout Workbench.

The files here are intentional project configuration and guides. They must stay separate from `.codex/runtime/`, `log/`, `tmp/`, cache folders, local credentials, browser sessions, and generated artifacts.

## Purpose

The system models a small specialist team:

- Automation Architect
- Backend/CDP Engineer
- Frontend Workflow/UX Expert
- Operations/User Representative

Each role has a bounded responsibility, explicit permissions, input/output expectations, and a collaboration protocol. The goal is to make project changes safer by ensuring automation flow design, CDP behavior, UI workflow impact, and operator usability are reviewed together.

## Directory Layout

```text
.codex/agents/
  README.md
  agents.manifest.json
  protocol.md
  permissions.md
  schemas/
    message.schema.json
    task.schema.json
  roles/
    automation-architect.md
    backend-cdp-engineer.md
    frontend-workflow-ux.md
    operations-user-representative.md
  workflows/
    checkout-automation-review.md
  tasks/
    p0-p4-review.task.json
```

## How To Use

1. Start from `agents.manifest.json` to select the role set and workflow.
2. Load the relevant role documents from `roles/`.
3. Use `protocol.md` for message format, handoff rules, and decision records.
4. Use `permissions.md` before assigning file ownership or runtime actions.
5. Use `workflows/checkout-automation-review.md` for P0-P4 automation work.
6. Use `tasks/p0-p4-review.task.json` as a reusable task template.
7. Use `schemas/` to keep new task and message files compatible with the role interface.

## Shared Project Context

Primary source files:

- `main.go`: Go HTTP service, CDP, GoPay, Checkout, LuckMail, audit logging.
- `web/app.js`: frontend workflow orchestration, state rendering, polling, browser API calls.
- `web/index.html`: UI structure.
- `web/styles.css`: visual system and responsive layout.
- `main_test.go`: tests and regression guards.
- `docs/audit/automation-log-analysis-p0-p4-plan.md`: latest P0-P4 analysis and implementation plan.

Sensitive local runtime state:

- Do not copy or expose `log/`, `gptpls/`, session JSON, tokens, PINs, cookies, or `.codex/runtime/` artifacts unless explicitly required for a local-only diagnostic task.
- If a report must reference sensitive material, summarize fields and preserve only local file paths.

## Role Output Contract

Every role response should include:

- `role`
- `scope`
- `findings`
- `risks`
- `recommendations`
- `required_followups`
- `confidence`

For implementation work, each role must also state:

- owned files or modules
- files that must not be edited by that role
- validation commands
- rollback or safety notes
