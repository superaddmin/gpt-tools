# Multi-Agent Communication Protocol

Last verified: 2026-05-11

## 1. Core Principles

- Use role ownership to reduce accidental cross-module edits.
- Share conclusions, evidence, and risks, not raw sensitive data.
- Prefer one decision owner per topic.
- Keep payment actions conservative: never optimize by repeating final payment clicks.
- Treat `docs/audit/automation-log-analysis-p0-p4-plan.md` as the current automation improvement baseline.

## 2. Message Types

| Type | Purpose | Required Fields |
| --- | --- | --- |
| `brief` | Assign or summarize a task | `task_id`, `from_role`, `to_role`, `scope`, `acceptance_criteria` |
| `finding` | Report evidence from code/logs/UI | `evidence`, `impact`, `confidence` |
| `proposal` | Suggest a design or implementation path | `proposal`, `tradeoffs`, `required_reviews` |
| `decision` | Record accepted or rejected direction | `decision`, `owner_role`, `rationale`, `validation` |
| `handoff` | Transfer work between roles | `completed`, `remaining`, `blocked_by`, `files_touched` |
| `blocker` | Escalate an issue that stops progress | `blocker`, `needed_decision`, `risk_if_ignored` |
| `verification` | Report validation result | `commands`, `result`, `residual_risk` |

## 3. Message Envelope

```json
{
  "task_id": "p0-p4-checkout-automation",
  "from_role": "automation-architect",
  "to_role": "backend-cdp-engineer",
  "message_type": "brief",
  "priority": "P1",
  "context_refs": [
    "docs/audit/automation-log-analysis-p0-p4-plan.md#P3",
    "main.go:/api/gopay/cdp-otp",
    "web/app.js:startGopayOTPAutoCapture"
  ],
  "payload": {
    "scope": "Add target readiness gate before GoPay CDP polling",
    "acceptance_criteria": [
      "Do not call /api/gopay/cdp-otp when no Midtrans or GoPay target exists",
      "Do not introduce repeated Pay now clicks",
      "Expose next_action when target is missing"
    ]
  },
  "requested_action": "Review backend feasibility and implementation boundary",
  "deadline_or_gate": "Before frontend state-machine rollout"
}
```

## 4. Shared Context Packet

Each multi-role task should start with a compact context packet:

```json
{
  "task_id": "string",
  "objective": "string",
  "current_state": "string",
  "known_failures": ["string"],
  "source_files": ["string"],
  "logs_summary": ["string"],
  "constraints": ["string"],
  "acceptance_criteria": ["string"]
}
```

## 5. Decision Record

Every cross-role decision must be captured as:

```json
{
  "decision_id": "P3-CDP-READY-GATE-001",
  "owner_role": "automation-architect",
  "status": "accepted",
  "decision": "Gate GoPay CDP polling on Midtrans/GoPay target presence.",
  "rationale": "Historical logs show cdp-otp 503s from CDP not ready and target missing.",
  "risks": [
    "Gate may delay detection of newly opened PIN iframe if target discovery is too strict."
  ],
  "mitigations": [
    "Use short probe interval with explicit payment_target_waiting state.",
    "Allow gopayapi.com iframe target and app.midtrans.com parent target."
  ],
  "validation": [
    "go test ./...",
    "Manual log check for reduced cdp-otp Service Unavailable"
  ]
}
```

## 6. Review Order

For checkout automation work:

1. Automation Architect defines state transition and acceptance criteria.
2. Backend/CDP Engineer reviews API/CDP feasibility and payment safety.
3. Frontend Workflow/UX Expert reviews operator flow and status visibility.
4. Operations/User Representative reviews manual fallback and runbook clarity.
5. Automation Architect records final decision.

## 7. Conflict Resolution

| Conflict | Tie Breaker |
| --- | --- |
| Speed vs payment safety | Payment safety wins |
| Automation vs manual control | Manual takeover must remain available |
| Logging detail vs sensitive data | Local-only sensitive capture must be explicit |
| Backend simplicity vs frontend clarity | Prefer explicit API fields that reduce UI guesswork |

