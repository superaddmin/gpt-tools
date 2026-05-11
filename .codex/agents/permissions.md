# Multi-Agent Permission Matrix

Last verified: 2026-05-11

## 1. Global Rules

- Do not modify `.codex/runtime/`, `log/`, `gptpls/`, `.tmp/`, `tmp/`, generated browser artifacts, or session files unless the task explicitly requires local diagnostics.
- Do not commit secrets, cookies, PINs, OAuth tokens, session JSON, or local runtime logs.
- Preserve payment safety boundaries:
  - one trusted Pay now click maximum per payment surface
  - no repeated final payment clicks
  - no automation that blocks native redirects
  - no automatic final OpenAI confirmation outside explicitly safe workflow steps
- Use least-privilege file ownership: each role owns only the files needed for its scope.

## 2. Role Permissions

| Role | Read Scope | Write Scope | Restricted Actions |
| --- | --- | --- | --- |
| Automation Architect | all source, docs, summarized logs | `docs/`, `.codex/agents/`, state-machine specs | direct payment automation code without backend/CDP review |
| Backend/CDP Engineer | `main.go`, `main_test.go`, `docs/api/`, summarized logs | `main.go`, `main_test.go`, backend docs | frontend UI rewrites, unsafe payment click loops |
| Frontend Workflow/UX Expert | `web/`, docs, API summaries | `web/app.js`, `web/index.html`, `web/styles.css`, UI docs | backend API contract changes without backend review |
| Operations/User Representative | docs, UI screenshots, summarized logs | runbooks, docs, checklist files | code edits unless explicitly assigned |

## 3. Sensitive Data Policy

Allowed in local-only diagnostics when explicitly requested:

- PIN values
- account emails
- local log filenames
- local JSON session filenames

Not allowed in shared project artifacts by default:

- access tokens
- refresh tokens
- cookies
- complete session JSON
- raw email inbox bodies
- raw `log/` file copies

If sensitive capture is required, the role must state:

```text
sensitive_capture_required: true
reason: local reproduction/debugging only
storage: existing local log files only
redaction_for_reports: required
```

## 4. Implementation Gates

| Gate | Required Reviewers |
| --- | --- |
| Payment click behavior change | Automation Architect + Backend/CDP Engineer + Operations/User Representative |
| CDP target/context selection change | Backend/CDP Engineer + Automation Architect |
| Frontend state-machine transition | Automation Architect + Frontend Workflow/UX Expert |
| UI status wording/manual fallback | Frontend Workflow/UX Expert + Operations/User Representative |
| Sensitive logging field | Backend/CDP Engineer + Operations/User Representative |
| P0-P4 priority change | All roles |

## 5. Validation Requirements

Backend/CDP changes:

```powershell
go test ./...
```

Frontend JS changes:

```powershell
node --check web/app.js
```

Docs/config-only changes:

```powershell
Test-Path .codex/agents/agents.manifest.json
```

JSON manifest changes:

```powershell
Get-Content .codex/agents/agents.manifest.json -Raw | ConvertFrom-Json | Out-Null
```

