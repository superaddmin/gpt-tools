# API 参考

最后核对日期：2026-05-17

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

### `POST /api/sandbox/payment-authorization/assess`

本地 mock/sandbox 支付授权篡改评估。该接口只允许本机访问，用于验证伪造支付授权、旧凭证 replay、篡改 localStorage/window evidence、跨账号 voucher 注入等场景的拒绝合同。

安全约束：

- 不打开或操作真实支付页。
- 不使用真实凭证。
- 不提交真实 checkout 授权。
- 响应固定标记 `payment_action_executed: false`、`real_checkout_touched: false`、`real_credential_used: false`。

默认请求会执行完整 mock flow：

```json
{}
```

也可以指定单个场景：

```json
{
  "scenario": "cross_account_voucher_injection"
}
```

或提交自定义 mock attempt：

```json
{
  "source": "legacy_voucher",
  "voucher": {
    "payment_reference_id": "pay-ref-old",
    "account_id": "snap-old",
    "checkout_session_id": "cs_mock_old"
  },
  "context": {
    "current_account_id": "snap-new",
    "current_checkout_session_id": "cs_mock_new"
  }
}
```

成功响应：

```json
{
  "ok": true,
  "stage": "payment_authorization_sandbox_assessed",
  "sandbox": true,
  "mode": "mock",
  "scenario": "full_flow",
  "scenario_count": 4,
  "accepted_count": 0,
  "rejected_count": 4,
  "all_rejected": true,
  "authorization_accepted": false,
  "accepted_authority": false,
  "payment_action_executed": false,
  "real_checkout_touched": false,
  "real_credential_used": false,
  "trusted_authority_needed": "current_checkout_server_state",
  "scenarios": [
    {
      "name": "forged_payment_authorization",
      "source": "manual_payload",
      "authorization_accepted": false,
      "risk_reasons": [
        "untrusted_authorization_source",
        "client_claimed_payment_success",
        "server_payment_voucher_missing"
      ]
    }
  ]
}
```

未知场景或非法 JSON 返回 HTTP 400，远程来源返回 HTTP 403。

### `POST /api/voice/speak`

调用本机系统语音播报流程提示，是浏览器 `speechSynthesis` 无声或失败时的本地兜底。当前实现优先支持 Windows，通过 `System.Speech.Synthesis.SpeechSynthesizer` 发声。

请求体：

```json
{
  "message": "语音播报已开启"
}
```

成功响应：

```json
{
  "ok": true,
  "stage": "local_voice_spoken",
  "engine": "windows_system_speech",
  "message": "语音播报已开启"
}
```

业务失败时返回 HTTP 200，`ok: false`，并在 `stage` / `error` 中说明原因。

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

### `POST /api/incognito/close`

关闭本工具管理的 Chrome 无痕窗口或相关 CDP 页面，用于完整流程结束后的收尾清理。

请求体：无。

成功响应：

```json
{
  "ok": true,
  "stage": "incognito_close",
  "closed_count": 1
}
```

说明：

- 如果 CDP 端口未运行，返回 `stage: incognito_not_running`，并清理本地窗口状态。
- 如果未发现可关闭目标，返回 `stage: incognito_close_no_targets`。

### `POST /api/incognito/diagnostics`

诊断当前 Chrome 无痕窗口和 CDP 连接状态，返回 CDP 版本、浏览器版本、User-Agent、当前打开的标签页列表等信息。

请求体：无。

成功响应：

```json
{
  "ok": true,
  "stage": "incognito_diagnostics",
  "cdp_ready": true,
  "cdp_version": {
    "Browser": "Chrome/...",
    "Protocol-Version": "1.3",
    "User-Agent": "Mozilla/5.0 ...",
    "V8-Version": "...",
    "WebKit-Version": "..."
  },
  "targets": [
    {
      "id": "target-id",
      "type": "page",
      "url": "https://chatgpt.com/",
      "title": "ChatGPT",
      "attached": false
    }
  ],
  "target_count": 1,
  "page_count": 1,
  "incognito_targets": 1
}
```

CDP 未就绪时返回 HTTP 503：

```json
{
  "ok": false,
  "stage": "incognito_diagnostics",
  "cdp_ready": false,
  "error": "CDP 未就绪"
}
```

### `POST /api/login/click`

通过 CDP 在 chatgpt.com 页面查找并点击登录按钮，引导用户进入登录流程。

请求体：无。

成功响应：

```json
{
  "ok": true,
  "stage": "login_button_clicked",
  "clicked": true,
  "target_url": "https://chatgpt.com/",
  "login_page_opened": true
}
```

失败响应：

- `503`：CDP 未就绪或未找到 chatgpt.com 页面。
- `500`：CDP 执行异常或未找到登录按钮。

### `POST /api/login/email-fill`

在 ChatGPT 登录页面填写邮箱地址并提交，进入验证码等待阶段。

请求体：

```json
{
  "email": "user@example.com"
}
```

校验：

- `email` 必填。

成功响应：

```json
{
  "ok": true,
  "stage": "login_email_filled",
  "email_filled": true,
  "email_submitted": true,
  "code_page_detected": true,
  "target_url": "https://auth.openai.com/..."
}
```

失败响应：

- `400`：JSON 非法或 email 缺失。
- `503`：CDP 未就绪或未找到登录页面。
- `500`：CDP 执行异常或未找到邮箱输入框。

### `POST /api/login/code-fill`

在 ChatGPT 验证码页面填入验证码（通常来自 LuckMail 接码），完成登录。

请求体：

```json
{
  "code": "123456"
}
```

校验：

- `code` 必填。

成功响应：

```json
{
  "ok": true,
  "stage": "login_code_filled",
  "code_filled": true,
  "code_submitted": true,
  "login_completed": true,
  "target_url": "https://chatgpt.com/..."
}
```

失败响应：

- `400`：JSON 非法或 code 缺失。
- `503`：CDP 未就绪或未找到验证码页面。
- `500`：CDP 执行异常或未找到验证码输入框。

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

### `POST /api/checkout/payment-method-select`

在当前 checkout 页面只选择支付方式，不填写地址、不勾选条款、不点击最终订阅按钮。选择顺序固定为：优先 GoPay；页面没有 GoPay 时选择 PayPal。

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
  "stage": "checkout_payment_method_selected",
  "selected_payment_method": "gopay",
  "gopay_selected": true,
  "paypal_selected": false,
  "manual_confirmation_required": true,
  "subscription_submit_clicked": false,
  "payment_method_selection_order": ["gopay", "paypal"]
}
```

### `POST /api/checkout/authorized-subscribe-click`

仅在本地前端收到使用者明确确认后，对当前 checkout/订阅确认页执行一次 CDP 真实鼠标点击。该接口不会由后台 watcher 自动触发，同一个 checkout key 只允许成功点击一次。

请求体：

```json
{
  "expected_url": "https://pay.openai.com/c/pay/cs_...",
  "target_id": "可选 CDP target id",
  "authorized": true
}
```

成功响应主要字段：

```json
{
  "ok": true,
  "stage": "subscription_submit_authorized_clicked",
  "subscription_submit_authorized": true,
  "subscription_submit_clicked": true,
  "subscription_submit_click_count": 1,
  "click_source": "local_user_authorized_cdp",
  "current_url": "https://..."
}
```

安全边界：

- 缺少 `authorized: true` 时拒绝执行。
- 只定位按钮矩形并通过 CDP `Input.dispatchMouseEvent` 点击，不在页面脚本中调用 `.click()` 或 `submit()`。
- 如果按钮禁用、目标页不匹配或同一 checkout 已点击过，返回 `ok: false` 并保持 `subscription_submit_clicked: false`。

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

通过 CDP 在 GoPay/Midtrans 页面内观测 OTP/PIN 页面，并在 PIN 页面使用已知 PIN 自动填入。

请求体：

```json
{
  "otp": "111111",
  "pin": "123456",
  "target_id": "CDP target id，可选",
  "target_url": "https://app.midtrans.com/snap/v4/redirection/account-id#/gopay-tokenization/linking",
  "account_id": "account-id",
  "checkout_url": "https://pay.openai.com/c/pay/cs_..."
}
```

`target_id`、`target_url`、`account_id`、`checkout_url` 均为可选上下文。自动触发链路会把 `/api/gopay/midtrans-linking-fill` 返回的 `cdp_target_id` 带回来，使 PIN 自动输入优先绑定到同一浏览器标签页。

成功响应：

```json
{
  "ok": true,
  "stage": "pin_entry_payment",
  "selected_target_id": "CDP target id",
  "selected_target_url": "https://...",
  "pin_stage": "payment",
  "pin_auto_filled": true,
  "pin_auto_submitted": true,
  "pin_input_strategy": "single_input",
  "result": {
    "has_otp_field": false,
    "has_pin_field": true,
    "detected_otp": "",
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
  "duration_s": 30,
  "stream": false
}
```

`duration_s` 小于等于 0 或大于 120 时重置为 30。

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
    "duration_s": 30
  }
}
```

流式响应 envelope 类型：`started`、`event`、`summary`、`done`、`error`。

### `POST /api/pricing/monitor`

在 ChatGPT 页面内采集 pricing/subscription/checkout 等相关资源、卡片、促销线索和本地存储摘要。

请求体：

```json
{
  "duration_s": 12,
  "url_filters": ["pricing", "checkout", "trial"]
}
```

默认过滤词：`pricing`、`subscription`、`checkout`、`payments`、`stripe`、`offer`、`promo`、`trial`、`eligible`、`eligibility`、`experiment`、`feature`、`plan`。

成功响应：

```json
{
  "ok": true,
  "summary": {
    "duration_s": 12,
    "resource_count": 0,
    "card_count": 0,
    "promo_hint_count": 0,
    "filters": ["pricing", "checkout", "trial"]
  },
  "snapshot": {},
  "storage": {}
}
```

### `POST /api/pricing/plus-subscribe-probe`

探测当前 CDP 管理的 ChatGPT 页面是否同时出现 Plus 套餐特征和“订阅并付款 / Subscribe and pay”按钮。该接口只读取页面状态，用于前端触发“获取 Session JSON → 生成并打开支付页”流程，不会点击最终订阅或付款按钮。

成功响应：

```json
{
  "ok": true,
  "stage": "plus_subscribe_probe",
  "plus_plan_detected": true,
  "subscribe_payment_detected": true,
  "safe_trigger_fetch_session": true,
  "subscription_submit_clicked": false,
  "manual_payment_confirmation_required": true,
  "button_text": "订阅并付款"
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
  "cdp_target_id": "CDP target id",
  "cdp_target_url": "https://...",
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

## LuckMail API

### `GET /api/luckmail/config`

读取本机保存的 LuckMail API Key 配置档案。响应只返回密钥摘要，不返回 API Key 明文。

```json
{
  "ok": true,
  "stage": "luckmail_api_config_loaded",
  "active_id": "main",
  "active_source": "profile",
  "profiles": [
    {
      "id": "main",
      "name": "主力 API",
      "active": true,
      "api_key": {
        "present": true,
        "prefix": "luck_9",
        "suffix": "abcd",
        "length": 37
      },
      "base_url": "https://mails.luckyous.com",
      "project_code": "openai",
      "email_type": "ms_graph",
      "timeout_s": 300,
      "interval_s": 3
    }
  ],
  "fallback_config": {
    "id": "__config__",
    "name": "配置文件 / 环境变量",
    "locked": true,
    "source": "config",
    "api_key": {
      "present": true,
      "length": 37
    }
  }
}
```

### `POST /api/luckmail/config`

新增或更新本机 LuckMail API 配置。新增配置时 `api_key` 必填；更新已有配置时 `api_key` 为空表示保留原密钥。`set_active: true` 会设为默认配置。

```json
{
  "id": "main",
  "name": "主力 API",
  "api_key": "luck_xxx",
  "base_url": "https://mails.luckyous.com",
  "project_code": "openai",
  "email_type": "ms_graph",
  "domain": "",
  "timeout_s": 300,
  "interval_s": 3,
  "set_active": true
}
```

成功响应与 `GET /api/luckmail/config` 相同，并额外返回 `saved_id`。管理接口只允许本机访问，并校验本地 `Origin` / `Referer`。

### `DELETE /api/luckmail/config`

删除本机保存的配置档案。

```json
{
  "id": "main"
}
```

### `POST /api/luckmail/config/test`

使用指定配置或临时 API Key 调用 LuckMail 用户信息接口，验证 `X-API-Key` 是否可用。响应不返回 API Key 明文。

```json
{
  "id": "main",
  "api_key": "",
  "base_url": "https://mails.luckyous.com"
}
```

### `POST /api/luckmail/create-and-wait`

创建临时接码订单并等待验证码。可选传入 `luckmail_api_key_profile_id` 指定主页面板中的配置；为空时使用活动配置，最后兜底到 `LUCKMAIL_API_KEY` / `config.local.json`。

请求体：

```json
{
  "luckmail_api_key_profile_id": "main",
  "project_id": 1,
  "supplier_id": 1,
  "timeout_s": 300,
  "interval_s": 3
}
```

校验：

- `project_id` 和 `supplier_id` 至少需要一个。

成功响应：

```json
{
  "ok": true,
  "stage": "luckmail_code_received",
  "email": "xxx@graph.microsoft.com",
  "code": "123456",
  "order_id": 12345,
  "elapsed_s": 15,
  "attempts": 5
}
```

超时未收到验证码时返回 HTTP 200：

```json
{
  "ok": false,
  "stage": "luckmail_code_timeout",
  "error": "等待验证码超时",
  "email": "xxx@graph.microsoft.com",
  "order_id": 12345,
  "elapsed_s": 300
}
```

### `POST /api/luckmail/token-code`

通过已购邮箱 Token 等待验证码。支持 `luckmail_api_key_profile_id`、`luckmail_token`、`timeout_s`、`interval_s`、`since_unix_ms`。

请求体：

```json
{
  "luckmail_api_key_profile_id": "main",
  "luckmail_token": "token-from-purchase",
  "timeout_s": 300,
  "interval_s": 3,
  "since_unix_ms": 1770000000000
}
```

校验：

- `luckmail_token` 必填。

成功响应：

```json
{
  "ok": true,
  "stage": "luckmail_token_code_received",
  "email": "xxx@graph.microsoft.com",
  "code": "123456",
  "elapsed_s": 12,
  "attempts": 4
}
```

### `POST /api/luckmail/token-mails`

通过已购邮箱 Token 查询邮件列表摘要。支持 `luckmail_api_key_profile_id` 与 `luckmail_token`。

请求体：

```json
{
  "luckmail_api_key_profile_id": "main",
  "luckmail_token": "token-from-purchase"
}
```

校验：

- `luckmail_token` 必填。

成功响应：

```json
{
  "ok": true,
  "stage": "luckmail_token_mails_loaded",
  "email": "xxx@graph.microsoft.com",
  "mails": [
    {
      "id": 1,
      "subject": "OpenAI - Verification Code",
      "from": "noreply@openai.com",
      "received_at": "2026-05-12T00:00:00Z",
      "has_code": true
    }
  ],
  "total": 1
}
```

### `POST /api/luckmail/purchases`

读取已购邮箱列表。支持 `luckmail_api_key_profile_id`、`page`、`page_size`、`keyword`、`user_disabled`。

请求体：

```json
{
  "luckmail_api_key_profile_id": "main",
  "page": 1,
  "page_size": 20,
  "keyword": "",
  "user_disabled": false
}
```

成功响应：

```json
{
  "ok": true,
  "stage": "luckmail_purchases_loaded",
  "purchases": [
    {
      "id": 123,
      "email": "xxx@graph.microsoft.com",
      "project_code": "openai",
      "status": "active",
      "created_at": "2026-05-12T00:00:00Z"
    }
  ],
  "total": 1,
  "page": 1,
  "page_size": 20
}
```

### `POST /api/gptpls/record`

记录 GPT Plus 订阅相关信息，用于本地追踪和审计。

请求体：

```json
{
  "checkout_url": "https://pay.openai.com/c/pay/cs_...",
  "checkout_session_id": "cs_...",
  "access_token_hash": "sha256...",
  "plan_name": "chatgptplusplan",
  "country": "ID",
  "currency": "IDR",
  "payment_method": "gopay",
  "status": "completed"
}
```

成功响应：

```json
{
  "ok": true,
  "stage": "gptpls_record_saved",
  "record_id": "rec_..."
}
```

### `POST /api/perf/client`

接收前端客户端性能日志，用于监控页面加载、API 调用耗时等指标。

请求体：

```json
{
  "entries": [
    {
      "name": "page_load",
      "duration_ms": 1200,
      "timestamp": 1770000000000,
      "details": {}
    }
  ]
}
```

成功响应：

```json
{
  "ok": true,
  "stage": "client_perf_logged",
  "entries_received": 1
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
