# API 参考

最后核对日期：2026-05-07

本文档以 [main.go](file:///f:/chatadd/main.go) 的路由注册和 handler 实现、以及 [.codex/runtime/browser-use-service/src/server.js](file:///f:/chatadd/.codex/runtime/browser-use-service/src/server.js) 为事实来源。

## 通用约定

- Go 服务监听 `127.0.0.1:18473`，浏览器可访问 `http://localhost:18473/`。
- browser-use 服务默认监听 `127.0.0.1:38765`，可通过 `BROWSER_USE_PORT` 覆盖。
- Go API 的通用错误结构为：

```json
{
  "error": "错误信息",
  "status": 400
}
```

- 部分业务接口即使业务失败也会返回 HTTP 200，并在 JSON 中使用 `ok: false`、`stage`、`error` 表达阶段性失败。
- `stream: true` 的监控类接口返回 `application/x-ndjson; charset=utf-8`，每行是一个 JSON envelope。

## Go 本地服务 API

### `GET /api/health`

健康检查。

响应：

```json
{
  "status": "ok",
  "time": "2026-05-07T00:00:00+08:00"
}
```

方法不匹配时返回 `405` 和通用错误结构。

### `POST /api/checkout`

生成可打开的 checkout 长链接。`/api/checkout/start` 与该接口共用同一个 handler。

请求体：

```json
{
  "token": "access token、JWT 或可由前端提取出的 token",
  "entry_point": "all_plans_pricing_modal",
  "plan_name": "chatgptplusplan",
  "billing_details": {
    "country": "ID",
    "currency": "IDR"
  },
  "promo_campaign": {
    "promo_campaign_id": "plus-1-month-free",
    "is_coupon_from_query_param": true
  },
  "checkout_ui_mode": "hosted",
  "proxy": {
    "type": "",
    "url": ""
  },
  "tax_region": {
    "country": "US",
    "line1": "",
    "city": "",
    "postal_code": "",
    "state": ""
  },
  "customer_email": "test@example.com",
  "checkout_session": {
    "cookie": "chatgpt.com Cookie，可选",
    "user_agent": "浏览器 User-Agent，可选"
  },
  "gopay_link": {
    "type": "gopay",
    "country_code": "86",
    "phone_number": "18120322232",
    "otp_channel": "whatsapp",
    "otp": "111111",
    "pin": "123456"
  }
}
```

必填校验：

- `token`
- `plan_name`
- `billing_details.country`
- `billing_details.currency`
- `checkout_ui_mode`
- `proxy.type` 为空或合法代理类型；代理 URL 由后端校验。

成功响应：

```json
{
  "checkout_url": "https://pay.openai.com/c/pay/cs_...",
  "url": "https://pay.openai.com/c/pay/cs_...",
  "checkout_session_id": "cs_..."
}
```

主要失败响应：

- `400`：请求体非法或必填字段缺失。
- `502`：checkout 上游请求失败、响应无法读取或 Stripe 初始化未得到可用链接。
- `409`：Stripe 初始化成功但后端判断当前 session 不符合 0 元试用校验，响应包含 `stage: checkout_trial_validation`、`trial_total`、`stripe_init` 等诊断字段。
- 上游返回非 2xx JSON 时，接口透传上游状态码和响应体。

### `POST /api/checkout/start`

与 `POST /api/checkout` 完全相同，保留为前端兼容入口。

### `POST /api/incognito/open`

启动或复用带 CDP 远程调试端口的系统 Chrome 无痕窗口。

请求体：

```json
{
  "url": "https://chatgpt.com/",
  "new_window": true
}
```

校验：

- `url` 必填。
- `url` scheme 必须是 `http` 或 `https`。

成功响应：

```json
{
  "ok": true,
  "url": "https://chatgpt.com/",
  "new_window": true
}
```

失败响应：

- `400`：JSON 非法、URL 缺失或 URL scheme 非法。
- `500`：Chrome 启动失败。

### `POST /api/session/fetch`

通过 Chrome CDP 在 `chatgpt.com` 页面内请求 `/api/auth/session`，提取 Session JSON 并返回诊断信息。

请求体：

```json
{
  "stream": true
}
```

非流式成功响应：

```json
{
  "ok": true,
  "json": "{\"accessToken\":\"...\"}",
  "diagnostics": {
    "cdp_ready": true,
    "target_found": true,
    "target_url": "https://chatgpt.com/",
    "title": "ChatGPT",
    "ready_state": "complete",
    "login_selector_count": 0,
    "has_access_token": true,
    "session_status": 200,
    "session_length": 1234,
    "session_preview": "...",
    "cookies_enabled": true,
    "body_text": "...",
    "elapsed_ms": 100,
    "conclusion": {
      "status": "session_ready",
      "message": "已检测到 accessToken，可以继续后续 checkout 自动化流程",
      "auto_action": "continue_checkout",
      "next_steps": []
    }
  }
}
```

流式响应 envelope 类型：

```json
{"type":"started","targets":["chatgpt.com:/api/auth/session"]}
{"type":"event","event":{"ts":1770000000000,"domain":"Network","method":"session-fetch","summary":"/api/auth/session status=200 len=1234"}}
{"type":"data","data":{"json":"...","diagnostics":{}}}
{"type":"done","data":{"json":"...","diagnostics":{}}}
```

失败响应：

- `503`：CDP 未就绪或未找到目标页，响应包含 `code: cdp_not_ready` 或诊断信息。
- `500`：CDP 执行过程异常。

### `POST /api/redteam/eligibility-replay-simulate`

本地红黑测试接口。只做“资格是否绑定账号 / 会话 / token 指纹”的防御模拟，不调用 OpenAI、Midtrans 或 GoPay，也不会生成真实 checkout 或试用会话。

访问限制：

- 接口默认关闭。
- 只有在 `ENABLE_REDTEAM_APIS=true` 或配置中 `enable_redteam_apis=true` 时才允许访问。
- 请求必须来自 loopback 地址，否则返回 `403`。

请求体：

```json
{
  "mode": "defensive_simulation",
  "current_account_id": "acct-current",
  "current_session_id": "sess-current",
  "current_token_fingerprint": "sha256-of-current-token",
  "previous_account_id": "acct-previous",
  "previous_trial_reference": "trial-ref-xxxx"
}
```

校验：

- `mode` 必须是 `defensive_simulation`
- `current_account_id`
- `current_session_id`
- `current_token_fingerprint`
- `previous_account_id`
- `previous_trial_reference`

成功响应分两类：

1. 同主体上下文通过本地校验：

```json
{
  "ok": true,
  "stage": "eligibility_context_verified",
  "decision": "matched_subject",
  "reason": "same_account_context"
}
```

2. 跨账号资格复用被拒绝：

```json
{
  "ok": false,
  "stage": "eligibility_replay_rejected",
  "decision": "rejected",
  "reason": "eligibility_bound_to_account_session",
  "controls": [
    "local_simulation_only",
    "account_binding",
    "session_binding",
    "token_fingerprint_binding"
  ],
  "evidence": {
    "account_match": false,
    "session_present": true,
    "token_fingerprint_present": true,
    "previous_trial_reference_present": true,
    "cross_account_replay": true
  }
}
```

响应中的 `current_session_id_summary`、`current_token_fingerprint_summary`、`previous_trial_reference_summary` 只返回长度、尾号和 SHA-256 摘要，不返回原文。

失败响应：

- `400`：请求体非法、字段缺失，或 `mode` 不是 `defensive_simulation`。
- `403`：redteam 接口未启用，或请求不是本机 loopback 访问。

### `GET /api/redteam/payment-replay-last`

从本地 `log/` 审计日志里提取最近一次成功支付的安全摘要。只返回调试所需的支付摘要和派生指纹，不调用真实支付系统。

访问限制与 `POST /api/redteam/eligibility-replay-simulate` 相同：默认关闭，需显式启用，且只允许 loopback 请求。

成功响应示例：

```json
{
  "ok": true,
  "stage": "payment_replay_summary_loaded",
  "source_file": "user@example.com_20260508110000.log",
  "source_operation_time": "2026-05-08T11:00:00+08:00",
  "source_account_email": "user@example.com",
  "previous_account_id": "acct-123",
  "previous_checkout_session_id": "cs_live_123",
  "previous_payment_reference_id": "pay-ref-123",
  "previous_transaction_id": "tx-123",
  "previous_transaction_status": "settlement",
  "previous_amount": "20.00",
  "previous_currency": "IDR",
  "previous_payment_artifact_fingerprint": {
    "present": true,
    "length": 68,
    "sha256": "..."
  }
}
```

失败响应：

- `404`：`log/` 中未找到最近一次成功支付摘要。
- `403`：redteam 接口未启用，或请求不是本机 loopback 访问。
- `500`：本地日志读取或解析失败。

### `POST /api/redteam/payment-replay-simulate`

本地红黑测试接口。用于模拟“把上一次成功支付的摘要信息重放到当前请求里”，验证服务端是否具备账号绑定、金额校验、币种校验、nonce 新鲜度和支付引用唯一性等防护。

访问限制与 `POST /api/redteam/eligibility-replay-simulate` 相同：默认关闭，需显式启用，且只允许 loopback 请求。

请求体：

```json
{
  "mode": "defensive_simulation",
  "current_account_id": "acct-current",
  "current_checkout_session_id": "cs-sim-fresh",
  "current_request_nonce": "nonce-fresh",
  "current_amount": "20.00",
  "current_currency": "IDR",
  "previous_account_id": "acct-previous",
  "previous_checkout_session_id": "cs-old",
  "previous_payment_reference_id": "pay-ref-123",
  "previous_transaction_id": "tx-123",
  "previous_request_nonce": "nonce-old",
  "previous_amount": "20.00",
  "previous_currency": "IDR"
}
```

校验：

- `mode` 必须是 `defensive_simulation`
- `current_account_id`
- `current_checkout_session_id`
- `current_amount`
- `current_currency`
- `previous_account_id`
- `previous_payment_reference_id`
- `previous_transaction_id`
- `previous_amount`
- `previous_currency`

成功响应示例：

```json
{
  "ok": false,
  "stage": "payment_replay_rejected",
  "decision": "rejected",
  "reason": "payment_reference_reuse_detected",
  "controls": [
    "local_simulation_only",
    "account_binding",
    "checkout_session_freshness",
    "request_nonce_freshness",
    "amount_integrity",
    "currency_integrity",
    "payment_reference_uniqueness",
    "transaction_uniqueness"
  ],
  "evidence": {
    "account_match": true,
    "amount_match": true,
    "currency_match": true,
    "checkout_session_reused": false,
    "request_nonce_reused": false,
    "payment_reference_present": true,
    "transaction_present": true
  }
}
```

接口会额外返回：

- `current_request_nonce_summary`
- `previous_request_nonce_summary`
- `current_payment_artifact_fingerprint`
- `previous_payment_artifact_fingerprint`

这些字段只返回摘要和 SHA-256，不返回可直接复用的完整敏感值。

失败响应：

- `400`：请求体非法、字段缺失，或 `mode` 不是 `defensive_simulation`。
- `403`：redteam 接口未启用，或请求不是本机 loopback 访问。

### `POST /api/checkout/resolve-target`

根据最近打开的 checkout URL，在 Chrome CDP 目标列表中解析当前实际支付页。

请求体：

```json
{
  "opened_url": "https://pay.openai.com/c/pay/cs_..."
}
```

成功响应：

```json
{
  "ok": true,
  "opened_url": "https://pay.openai.com/c/pay/cs_...",
  "current_url": "https://pay.openai.com/c/pay/cs_...",
  "target": {
    "id": "target-id",
    "type": "page",
    "url": "https://pay.openai.com/c/pay/cs_..."
  }
}
```

未解析到目标时返回 HTTP 200：

```json
{
  "ok": false,
  "error": "错误信息",
  "opened_url": "https://pay.openai.com/c/pay/cs_...",
  "candidate_urls": ["https://..."]
}
```

### `POST /api/checkout/auto-fill`

对当前 checkout 页面执行目标锁定和随机美国地址自动填写。后端会拒绝与 `expected_url` 不一致的支付页。

请求体：

```json
{
  "expected_url": "https://pay.openai.com/c/pay/cs_..."
}
```

成功响应主要字段：

```json
{
  "ok": true,
  "submitted": false,
  "expected_url": "https://pay.openai.com/c/pay/cs_...",
  "current_url": "https://pay.openai.com/c/pay/cs_...",
  "address": {
    "first_name": "...",
    "last_name": "...",
    "line1": "...",
    "city": "...",
    "state": "CA",
    "zip_code": "...",
    "country": "US"
  },
  "page_target": {
    "id": "target-id",
    "type": "page",
    "url": "https://..."
  },
  "fill_target": {
    "id": "target-id",
    "type": "iframe",
    "url": "https://js.stripe.com/..."
  },
  "probe": {},
  "filled": {}
}
```

业务失败通常返回 HTTP 200 且 `ok: false`，例如未检测到可填写表单、当前支付页与最近打开链接不一致、或目标页尚未展开 GoPay 地址区。

### `POST /api/gopay/force-link`

使用明确给定的 Midtrans account id 创建或复用 GoPay linking。

请求体：

```json
{
  "account_id": "account-id",
  "country_code": "86",
  "phone_number": "18120322232"
}
```

校验：

- `account_id` 必填。
- `phone_number` 必填。
- `country_code` 为空时默认 `86`。

响应主要字段：

```json
{
  "request": {
    "account_id": "account-id",
    "country_code": "86",
    "phone_number": "18120322232"
  },
  "diagnostics": {},
  "linking_diagnostics": {},
  "api": {
    "ok": true,
    "result": {},
    "reused_existing": false,
    "conflict_reason": "",
    "account": {}
  },
  "reused_existing": false,
  "reference_id": "reference-id"
}
```

### `POST /api/gopay/auto-link`

自动执行 GoPay linking、reference 校验、用户授权、OTP 候选验证、PIN token 和 PIN 校验。

请求体：

```json
{
  "account_id": "account-id",
  "country_code": "86",
  "phone_number": "18120322232",
  "otp_channel": "whatsapp",
  "otp": "111111",
  "pin": "123456"
}
```

默认值：

- `country_code` 默认 `86`。
- `otp_channel` 默认 `whatsapp`。

响应主要字段：

```json
{
  "ok": true,
  "stage": "linking_success",
  "request": {},
  "diagnostics": {},
  "linking_diagnostics": {},
  "reference_id": "reference-id",
  "otp_used": "111111",
  "pin_used": "123456",
  "summary": {
    "otp": "111111",
    "pin": "123456",
    "reference_id": "reference-id",
    "gopay_linked": true
  }
}
```

阶段性失败也返回 HTTP 200，`ok: false`，`stage` 可能为 `linking`、`validate_reference`、`user_consent`、`validate_otp`、`pin_token`、`validate_pin`。

### `POST /api/gopay/smart-link`

执行双通道策略的 GoPay linking。请求字段与 `/api/gopay/auto-link` 相同，响应包含：

```json
{
  "ok": true,
  "strategy": "dual-channel",
  "stage": "linking_success",
  "request": {},
  "diagnostics": {},
  "stages": [],
  "summary": {}
}
```

失败时使用 `ok: false`、`stage`、`error` 和 `stages` 描述失败阶段。

### `POST /api/gopay/cdp-otp`

通过 CDP 在 GoPay/Midtrans 页面内检测或填入 OTP。

请求体：

```json
{
  "otp": "111111"
}
```

成功响应：

```json
{
  "ok": true,
  "result": {
    "hasOTPField": true,
    "detected_otp": "",
    "used_otp": "111111",
    "auto_filled": true,
    "auto_submitted": false,
    "url": "https://...",
    "page_text_snippet": "..."
  }
}
```

### `POST /api/gopay/snap-probe`

探测 Midtrans Snap 页面、点击可能的 GoPay/link/continue 入口，并尝试注入手机号与 OTP。

请求体字段由匿名结构解析，实际 JSON 字段按 Go 默认规则匹配：

```json
{
  "SnapToken": "可选",
  "CountryCode": "86",
  "PhoneNumber": "18120322232",
  "OTP": "111111"
}
```

响应主要字段：

```json
{
  "ok": true,
  "probe_steps": [
    {"name": "probe-page"},
    {"name": "after-click"},
    {"name": "iframe-inject"}
  ],
  "otp_page_reached": true,
  "otp_info": {}
}
```

### `POST /api/gopay/monitor`

通过 CDP 监控 OpenAI、Stripe、Midtrans、GoPay 页面网络、导航、控制台和错误事件。

请求体：

```json
{
  "duration_s": 12,
  "stream": false
}
```

`duration_s` 小于等于 0 或大于 30 时重置为 12。

非流式响应中的 `events` 最多返回 120 条，避免响应体过大。

非流式成功响应：

```json
{
  "ok": true,
  "targets": ["OpenAI:chatgpt.com", "Stripe:stripe.com"],
  "events": [
    {"ts": 1770000000000, "domain": "Network", "method": "Stripe-req", "summary": "GET https://..."}
  ],
  "summary": {
    "total_events": 1,
    "network_calls": 1,
    "page_navigations": 0,
    "errors": 0,
    "console_calls": 0,
    "openai_api_calls": 0,
    "stripe_api_calls": 1,
    "snap_api_calls": 0,
    "gopay_api_calls": 0,
    "duration_s": 12
  }
}
```

流式响应 envelope 类型：`started`、`event`、`summary`、`done`、`error`。

### `POST /api/pricing/monitor`

在 ChatGPT 页面内采集 pricing/subscription/checkout 等相关资源、卡片、促销线索和本地存储摘要。

请求体：

```json
{
  "duration_s": 8,
  "url_filters": ["pricing", "checkout", "trial"]
}
```

`duration_s` 小于等于 0 或大于 60 时重置为 8。

默认过滤词：`pricing`、`subscription`、`checkout`、`payments`、`stripe`、`offer`、`promo`、`trial`、`eligible`、`eligibility`、`experiment`、`feature`、`plan`。

响应裁剪规则：

- `resources` 最多 80 条
- `promo_hints` 最多 20 条
- `body_text` 最多 2000 个字符
- `localStorage` / `sessionStorage` 最多各 30 个键
- 单个 storage 值最多 160 个字符

成功响应：

```json
{
  "ok": true,
  "summary": {
    "duration_s": 8,
    "resource_count": 0,
    "card_count": 0,
    "promo_hint_count": 0,
    "filters": ["pricing", "checkout", "trial"]
  },
  "snapshot": {},
  "storage": {}
}
```

### `POST /api/gopay/full-link`

完整 GoPay 链路辅助：使用 access token 生成新 checkout、提取 account id、创建或复用 linking、处理 OTP/PIN、执行 Midtrans 支付确认并返回阶段诊断。

请求体：

```json
{
  "access_token": "ChatGPT access token",
  "country_code": "86",
  "phone_number": "18120322232",
  "otp_channel": "whatsapp",
  "otp": "111111",
  "pin": "123456"
}
```

校验与默认值：

- `access_token` 必填。
- `phone_number` 必填。
- `country_code` 默认 `86`。
- `otp_channel` 默认 `whatsapp`。

响应主要字段：

```json
{
  "ok": true,
  "strategy": "full-link",
  "stage": "payment_complete",
  "diagnostics": {},
  "stages": [
    {"name": "generate-checkout", "ok": true},
    {"name": "extract-account", "ok": true},
    {"name": "linking", "ok": true},
    {"name": "payment", "ok": true}
  ],
  "summary": {},
  "payment_voucher": {},
  "transaction_id": "...",
  "midtrans_status": {}
}
```

失败时通常返回 HTTP 200，使用 `ok: false`、`stage`、`error`、`stages` 表示失败位置。

### `POST /api/gopay/auto-trigger-check`

判断 checkout 页面是否满足 GoPay 自动触发条件。

请求体：

```json
{
  "source": "frontend",
  "checkout_url": "https://pay.openai.com/c/pay/cs_...",
  "page_text": "页面文本",
  "submitted": true
}
```

响应：

```json
{
  "ok": true,
  "stage": "auto_trigger_ready",
  "ready": true,
  "already_triggered": false,
  "checkout_key": "https://pay.openai.com/c/pay/cs_...",
  "reason": "ready",
  "source": "frontend",
  "conditions": {
    "audit_capture_sensitive": true,
    "checkout_url_ok": true,
    "gopay_detected": true,
    "today_due_zero": true,
    "submitted": true,
    "already_triggered": false
  }
}
```

`ready` 只有在敏感捕获开启、URL 是 `pay.openai.com/c/pay/cs_...`、页面检测到 GoPay、今日应付为 0、已提交且未触发过时才为 `true`。

### `POST /api/gopay/midtrans-linking-fill`

在 Midtrans redirection/linking 页面内选择国家码、填写手机号并点击提交。

请求体：

```json
{
  "target_url": "https://app.midtrans.com/snap/v4/redirection/account-id#/gopay-tokenization/linking",
  "checkout_url": "https://pay.openai.com/c/pay/cs_...",
  "country_code": "86",
  "phone_number": "18120322232",
  "debug_network": false
}
```

默认值：

- `country_code` 默认 `86`。
- `phone_number` 默认 `18120322232`。
- `debug_network` 默认 `false`；设为 `true` 时会额外写入一次性网络诊断文件到 `artifacts/network-debug/`。

成功响应主要字段：

```json
{
  "ok": true,
  "stage": "midtrans_linking_submitted",
  "account_id": "account-id",
  "target_url": "https://...",
  "checkout_url": "https://...",
  "country_code": "86",
  "phone_number": "18120322232",
  "trigger_claimed": true,
  "browser_result": {},
  "network_diagnostics": {}
}
```

自动触发快速路径：

- 前端在 checkout 自动填地址完成后启动提交监听器，检测到支付页“订阅”已提交并解析到 Midtrans redirection/linking 页面后，才调用本接口。
- 本接口通过 Chrome CDP 连接已有 Midtrans 页面，在页面上下文内选择国家码、填写手机号并点击 `Link and pay`，不走后端 full-link 管道。
- 自动触发使用 checkout key 去重，同一个 checkout 不会重复触发。
- 点击后会等待真实页面状态：进入 GoPay/OTP/PIN 下一步则返回成功；出现 `technical error` 会返回 `stage: "midtrans_linking_technical_error"` 并保留点击尝试记录。

`network_diagnostics` 用于排查点击 `Link and pay` 后的 Midtrans/GoPay 请求结果。字段包括：

- `entries[].method`、`entries[].url`、`entries[].status`、`entries[].ok`、`entries[].elapsed_ms`。
- `entries[].request_headers`、`entries[].response_headers`：只保存 header 名称和摘要，包括 `present`、`length`、`prefix4`、`suffix4`、`sha256`，不保存 cookie/authorization 原文。
- `entries[].response_text_snippet.fields`：保留 `status_code`、`error_code`、`message`、`transaction_id`、`reference_id`、`payment_reference_id` 等诊断字段。
- `entries[].response_text_snippet.credentials`：对 token、cookie、authorization、client_secret、PIN/OTP 等可复用凭证只保存 `length`、`tail4`、`sha256`。
- URL 保留 origin、path、参数名和 Midtrans hash 路由，查询参数值统一为 `***`。

一次性 debug 文件：

- 在前端控制台执行 `localStorage.setItem("gopay_debug_network_once", "1")` 后，下一次自动 Midtrans linking 填充会携带 `debug_network: true`，前端随后清除该开关。
- 响应中的 `network_debug_artifact.path` 指向本机 `artifacts/network-debug/` 下的 JSON 文件。
- `artifacts/network-debug/` 默认在 `.gitignore` 中，文件内容仍不包含 cookie/authorization 原文或可直接复用的支付凭证。

如果 `target_url` 不是 Midtrans redirection 页面，返回 HTTP 200 且：

```json
{
  "ok": false,
  "stage": "midtrans_redirection_page_not_detected",
  "error": "target_url is not a Midtrans redirection page",
  "target_url": "..."
}
```

## browser-use 服务 API

### `GET /health`

健康检查。

响应：

```json
{
  "ok": true,
  "service": "browser-use",
  "port": 38765
}
```

### `POST /run`

执行 Playwright 自动化任务：导航、等待、表单扫描、可选填写/提交、页面数据提取。

请求体：

```json
{
  "task": "browser-use task",
  "url": "https://example.com",
  "browser": "chromium",
  "headless": true,
  "timeoutMs": 30000,
  "actionTimeoutMs": 15000,
  "submit": false,
  "formData": {
    "email": "test@example.com"
  },
  "fieldHints": {
    "email": "input[type=email]"
  },
  "waitFor": {
    "selector": "form",
    "text": "Submit",
    "urlIncludes": "checkout"
  },
  "extract": "full",
  "logLevel": "info",
  "retryCount": 2,
  "screenshotOnError": true,
  "outputSchema": null,
  "headers": {},
  "storageState": "state.json"
}
```

必填：

- `url` 必填；为空时返回 `INVALID_INPUT`。

默认值：

- `task` 默认 `browser-use task`。
- `browser` 非 `chromium`、`firefox`、`webkit` 时默认 `chromium`。
- `headless` 默认 `true`。
- `timeoutMs` 默认 `30000`。
- `actionTimeoutMs` 默认 `15000`。
- `retryCount` 默认 `2`。
- `screenshotOnError` 默认 `true`。

成功或部分成功响应：

```json
{
  "status": "success",
  "task": "browser-use task",
  "url": "https://example.com",
  "submitted": false,
  "summary": "任务执行成功",
  "data": {
    "navigation": {},
    "forms": [],
    "extraction": {}
  },
  "logs": [],
  "warnings": [],
  "errors": [],
  "durationMs": 1000
}
```

当存在警告时 `status` 为 `partial`，HTTP 状态仍为 200。当任务失败且 `runBrowserUse` 返回 `status: failed` 时，HTTP 状态为 500。

通用 browser-use 业务错误结构：

```json
{
  "error": "INVALID_JSON",
  "message": "请求体不是合法 JSON",
  "stage": "http",
  "details": {}
}
```

未知路径返回：

```json
{
  "error": "NOT_FOUND",
  "message": "未找到请求路径"
}
```
