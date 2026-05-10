# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Checkout Workbench — a local Go HTTP service that generates OpenAI/ChatGPT checkout payment links and automates the GoPay/Midtrans payment flow via Chrome CDP. The entire Go backend is a single `main.go` file (~11K lines) with embedded web frontend (`web/`) and a companion browser-use Node/Playwright service.

## Build & Run

```powershell
# Start Go service (listens on http://127.0.0.1:18473)
go run .

# Build binary
go build .

# Run all tests
go test ./...

# Run a single test by name
go test -run TestExtractLuckMailVerificationCode ./...

# Start browser-use service (separate terminal, 127.0.0.1:38765)
.\start-browser-use.cmd

# Start both services
.\start-all-services.cmd
```

## Architecture

### Monolithic Go service (`main.go`)
All backend logic lives in `package main` — no internal packages. HTTP routing uses stdlib `net/http.ServeMux`. Key subsystems:

- **Checkout API** (`/api/checkout`, `/api/checkout/start`): generates Stripe/pay.openai.com payment links from access tokens
- **Chrome CDP** (`/api/incognito/*`, `/api/session/fetch`, `/api/checkout/auto-fill`): launches/manages system Chrome with remote debugging on port 9223, executes JS via WebSocket CDP protocol
- **GoPay automation** (`/api/gopay/*`): linking, OTP, PIN, Midtrans page filling, full payment flow orchestration
- **Midtrans linking** (`/api/gopay/midtrans-linking-fill`): CDP-based page interaction for GoPay tokenization linking pages
- **Monitoring** (`/api/gopay/monitor`, `/api/pricing/monitor`): CDP-based network/console/page event collection (NDJSON streaming support)
- **LuckMail** (`/api/luckmail/*`): temporary email inbox for receiving verification codes
- **Audit logging**: middleware (`withAuditLogging`) writes JSON logs to `log/` directory with sensitive field masking
- **Static files**: `web/` directory embedded via `//go:embed`

### Web Frontend (`web/`)
- SPA with `index.html` + `app.js` + `styles.css` (embedded into Go binary)
- Three-column workbench layout
- `app.js` calls Go API and browser-use service directly from browser

### Browser-Use Service (`.codex/runtime/browser-use-service/`)
- Node.js Express server wrapping Playwright for generic page automation
- Endpoints: `GET /health`, `POST /run`
- Supports navigation, form filling, data extraction, screenshot on error

### Chrome Extension (`extension/`)
- Manifest V3 extension for extracting cookies/LocalStorage/SessionStorage from logged-in sites
- Used as a credential extraction helper

### Configuration
- Embedded `config.json` at build time, overridable via `APP_CONFIG` env var
- All config fields map to uppercase env vars (e.g. `checkout_endpoint` → `CHECKOUT_ENDPOINT`)
- Sensitive defaults: `audit_capture_sensitive` should stay `false` in normal environments

## Key Design Decisions

- **No Go framework**: pure stdlib `net/http`, manual JSON encode/decode, no router library
- **Config via embedded json + env override**: config is embedded at compile time via `//go:embed config.json`, runtime env var `APP_CONFIG` can override
- **Audit-sensitive data handling**: all logs redact tokens/cookies/passwords unless `audit_capture_sensitive: true`
- **Error handling convention**: business failures return HTTP 200 with `{"ok": false, "stage": "...", "error": "..."}`; transport-level failures return 4xx/5xx with `{"error": "...", "status": N}`
- **Testing**: no mocking framework — uses hand-written table-driven tests with `httptest.NewServer` for HTTP integration tests; tests directly call internal functions since everything is in `package main`

## Testing Patterns

- Tests live in `main_test.go` (same package, white-box testing)
- Common pattern: table-driven subtests with `t.Run`
- HTTP handler tests use `httptest.NewServer` + `httptest.NewRequest`/`ResponseRecorder`
- CDP-related tests use string-based CDP command/response serialization verification (no real CDP connection in unit tests)
- No external test dependencies; all tests in a single file (~3300 lines)

## Adding a New API Route

1. Add handler function in `main.go`
2. Register with `mux.HandleFunc` in `main()`
3. Add to `operationDisplayNames` map and `operationTypes` map
4. Add tests in `main_test.go`
5. Update `docs/api/reference.md`

## Docs Maintenance

- Architecture: `docs/architecture/overview.md`
- API reference: `docs/api/reference.md` (must stay in sync with route registrations)
- Development guide: `docs/development/guide.md`
- Consistency audit: `docs/audit/docs-code-consistency.md`
- Design specs: `docs/superpowers/specs/`
- Docs fact sources: `main.go` for backend, `web/app.js` for frontend calls, `server.js` + `browser-use.js` for browser-use service
