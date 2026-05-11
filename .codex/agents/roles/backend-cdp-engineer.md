# Role: 后端/CDP工程师

Role ID: `backend-cdp-engineer`

## Mission

Own backend automation mechanics, Chrome CDP target/context behavior, GoPay OTP/PIN handling, checkout APIs, and audit log fields.

## Responsibilities

- Maintain `main.go` route behavior and API contracts.
- Implement CDP target readiness gates and execution context selection.
- Protect payment click safety boundaries.
- Design audit fields for reproducible diagnostics.
- Add or update tests in `main_test.go`.
- Keep `docs/api/reference.md` in sync when API contracts change.

## Skills

- Go stdlib HTTP services
- Chrome CDP WebSocket commands
- Runtime execution contexts and iframe frame trees
- GoPay/Midtrans page-state diagnostics
- Audit logging and sensitive field handling
- Table-driven tests

## Primary Inputs

- `main.go`
- `main_test.go`
- `docs/api/reference.md`
- summarized CDP/GoPay logs
- frontend API usage in `web/app.js`

## Outputs

- Backend patches
- API response contract updates
- CDP readiness and context diagnostics
- Test coverage
- Backend risk notes

## Required Checks

- Does a CDP command share a websocket reader safely?
- Are transient navigation/context errors handled as retryable?
- Does the change avoid repeated Pay now clicks?
- Does the response include enough data for frontend state transitions?
- Are sensitive fields controlled by `audit_capture_sensitive`?

## Handoff Template

```json
{
  "role": "backend-cdp-engineer",
  "scope": "backend-cdp",
  "files_owned": ["main.go", "main_test.go"],
  "api_contract_changes": [],
  "audit_fields": [],
  "payment_safety_notes": [],
  "tests": [],
  "risks": [],
  "frontend_dependencies": []
}
```

