# Workflow: Checkout Automation Review

Workflow ID: `checkout-automation-review`

Last verified: 2026-05-11

## 1. Goal

Coordinate the four project roles to design, review, and validate automation changes for the Checkout Workbench P0-P4 roadmap.

## 2. Entry Criteria

Start this workflow when a task touches any of:

- recommended operation sequence
- Plus subscription probing
- session fetch automation
- checkout page target selection
- GoPay linking
- OTP/PIN automation
- Pay now behavior
- status lights or operator handoff
- audit logging or sensitive local diagnostics

## 3. Role Sequence

```text
Automation Architect
  -> Backend/CDP Engineer
  -> Frontend Workflow/UX Expert
  -> Operations/User Representative
  -> Automation Architect final decision
```

## 4. Required Shared Context

```json
{
  "task_id": "",
  "objective": "",
  "priority": "P0|P1|P2|P3|P4",
  "source_files": [],
  "current_log_evidence": [],
  "payment_safety_constraints": [
    "Do not repeat Pay now clicks",
    "Do not block native redirects",
    "Do not click final OpenAI confirmation automatically"
  ],
  "expected_user_visible_result": "",
  "validation_commands": []
}
```

## 5. Stage Gates

### Gate A: Architecture Fit

Owner: Automation Architect

Checks:

- Does this map to an existing or new state?
- Is the next action explicit?
- Does it reduce repeated polling or repeated manual work?

### Gate B: Backend/CDP Feasibility

Owner: Backend/CDP Engineer

Checks:

- API fields are sufficient for frontend state transitions.
- CDP target/context handling is safe.
- Transient errors are classified correctly.
- Sensitive logging respects local policy.

### Gate C: Frontend Workflow Clarity

Owner: Frontend Workflow/UX Expert

Checks:

- UI state text is concise and action-oriented.
- Status lights match backend truth.
- Polling can stop, pause, or back off.
- Manual fallback is visible.

### Gate D: Operator Acceptance

Owner: Operations/User Representative

Checks:

- User can recover without raw log reading.
- No irreversible payment step is hidden.
- Repeated input is reduced.
- Runbook is clear enough for normal use.

## 6. Review Output

Each review should produce:

```json
{
  "role": "",
  "approved": true,
  "blocking_findings": [],
  "non_blocking_findings": [],
  "required_changes": [],
  "validation": [],
  "residual_risk": []
}
```

## 7. Done Definition

A task is done when:

- implementation matches the role decision record
- relevant docs are updated
- validation commands pass or skipped validation is explicitly explained
- operator-visible behavior is described
- sensitive data handling is reviewed

