# 支付授权篡改与凭证重放 Sandbox 技术方案

最后核对日期：2026-05-12

## 1. 目标与边界

本文定义一套仅在本地 mock/sandbox 中运行的支付授权安全测试方案，用于验证 Checkout Workbench 对以下风险的识别、拒绝和审计能力：

- 伪造支付授权：客户端或手工 payload 声称 `authorization_accepted=true`、`stage=gopay_complete`。
- 旧凭证 replay：把历史支付 voucher、旧 checkout session 或旧 reference 注入当前流程。
- 篡改 localStorage/window evidence：在浏览器侧伪造 `checkoutWorkbenchPaymentEvidence` 或支付完成字段。
- 跨账号 voucher 注入：使用账号 A 的支付凭证替换账号 B 的 checkout 授权。

安全边界：

- 不对真实支付页面执行伪造授权、重放凭证或最终付款动作。
- 不把历史日志、localStorage、window 对象、截图或客户端 evidence 作为支付授权来源。
- 不记录、回放或输出 PIN、OTP、access token、refresh token、cookie、真实支付 token 等敏感材料。
- 任何 sandbox 场景的最终合同都必须保持 `authorization_accepted=false`。

## 2. 授权原则

支付授权只接受“当前 checkout 的服务端可信状态”，即：

- 当前账号绑定一致：`account_id` / email 与本轮 run 一致。
- 当前 checkout 一致：`checkout_session_id` / `checkout_key` 与本轮 run 一致。
- 当前支付引用一致：`payment_reference_id` 只作为诊断字段，不可单独构成授权。
- 当前服务端状态一致：必须由本轮 GoPay/Midtrans/checkout 服务端响应或后端状态机确认。

客户端可写数据只能作为 evidence，不能作为 authority。

## 3. 数据合同

### 3.1 输入模型

后端内部模型：

```go
type paymentAuthorizationTamperAttempt struct {
    Source   string
    Claims   map[string]any
    Evidence map[string]any
    Voucher  map[string]any
    Context  paymentVoucherReplayContext
}
```

`Source` 建议取值：

| source | 含义 | 授权结果 |
| --- | --- | --- |
| `manual_payload` | 手工构造的客户端成功态 | 拒绝 |
| `client_local_storage` | localStorage 取出的 evidence | 拒绝 |
| `window_evidence` | window 对象注入的 evidence | 拒绝 |
| `legacy_voucher` | 历史日志或旧流程 voucher | 拒绝 |
| `old_checkout` | 同账号旧 checkout 凭证 | 拒绝 |
| `current_checkout_server_state` | 当前服务端状态机 | 仅此类可进入真实授权判定 |

### 3.2 输出合同

所有 sandbox 篡改/重放场景返回统一拒绝合同：

```json
{
  "ok": false,
  "stage": "payment_authorization_tamper_rejected",
  "authorization_accepted": false,
  "accepted_authority": false,
  "risk_level": "high",
  "terminal": true,
  "retryable": false,
  "next_action": "use_current_checkout_authorization",
  "trusted_authority_needed": "current_checkout_server_state",
  "risk_reasons": [
    "untrusted_authorization_source",
    "client_claimed_payment_success"
  ]
}
```

voucher replay 子合同固定为：

```json
{
  "ok": false,
  "stage": "payment_voucher_replay_rejected",
  "authorization_accepted": false,
  "risk_level": "high",
  "risk_reasons": [
    "account_id_mismatch",
    "checkout_session_mismatch"
  ]
}
```

## 4. Sandbox 架构

已新增本地专用路由，默认只允许 `127.0.0.1` / `localhost` 来源：

| 路由 | 方法 | 用途 |
| --- | --- | --- |
| `/api/sandbox/payment-authorization/assess` | POST | 对任意 mock attempt 或默认 full flow 生成拒绝合同 |

当前 handler 流程：

1. 校验本机访问与本地 Origin/Referer。
2. 解析请求，只接受 mock 字段，不读取真实 token/cookie。
3. 无请求体或空 JSON 时执行默认 `full_flow` fixture。
4. 构造 `paymentAuthorizationTamperAttempt`。
5. 调用 `paymentAuthorizationTamperRiskContract`。
6. 通过审计日志记录脱敏后的 `source`、`risk_reasons`、`contract_version`。
7. 返回统一响应合同，禁止任何自动点击、CDP 写入、GoPay 提交或 checkout 付款动作。

默认请求：

```json
{}
```

默认响应关键字段：

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
  "trusted_authority_needed": "current_checkout_server_state"
}
```

自定义 mock attempt：

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

## 5. 四类测试场景

### 5.1 伪造支付授权

输入：

```json
{
  "source": "manual_payload",
  "claims": {
    "authorization_accepted": true,
    "stage": "gopay_complete"
  }
}
```

预期：

- `authorization_accepted=false`
- `risk_reasons` 包含 `untrusted_authorization_source`
- `risk_reasons` 包含 `client_claimed_payment_success`
- `risk_reasons` 包含 `server_payment_voucher_missing`

### 5.2 旧凭证 replay

输入：

```json
{
  "source": "old_checkout",
  "voucher": {
    "payment_reference_id": "pay-ref-old",
    "account_id": "snap-current",
    "checkout_session_id": "cs_live_old"
  },
  "context": {
    "current_account_id": "snap-current",
    "current_checkout_session_id": "cs_live_current"
  }
}
```

预期：

- `voucher_replay_contract.stage=payment_voucher_replay_rejected`
- `risk_reasons` 包含 `checkout_session_mismatch`
- 即使同账号，也不能授权。

### 5.3 篡改 localStorage/window evidence

输入：

```json
{
  "source": "client_local_storage",
  "evidence": {
    "checkoutWorkbenchPaymentEvidence": {
      "checkout_session_id": "cs_live_current",
      "payment_completed": true
    },
    "payment_completed": true
  }
}
```

预期：

- `risk_reasons` 包含 `client_evidence_not_authoritative`
- `risk_reasons` 包含 `local_evidence_injection_detected`
- `risk_reasons` 包含 `client_evidence_claimed_payment_success`

### 5.4 跨账号 voucher 注入

输入：

```json
{
  "source": "legacy_voucher",
  "voucher": {
    "payment_reference_id": "pay-ref-old",
    "account_id": "snap-old",
    "checkout_url": "https://chatgpt.com/checkout/openai_llc/cs_live_old"
  },
  "context": {
    "current_account_id": "snap-new",
    "current_checkout_session_id": "cs_live_new"
  }
}
```

预期：

- `risk_reasons` 包含 `account_id_mismatch`
- `risk_reasons` 包含 `checkout_session_mismatch`
- `authorization_accepted=false`

## 6. 审计字段

建议记录：

| 字段 | 示例 | 说明 |
| --- | --- | --- |
| `tamper_source` | `client_local_storage` | 尝试来源 |
| `risk_reasons` | `["local_evidence_injection_detected"]` | 归因 |
| `authorization_accepted` | `false` | 必须为 false |
| `accepted_authority` | `false` | 是否为可信授权来源 |
| `trusted_authority_needed` | `current_checkout_server_state` | 下一步可信来源 |
| `context_fingerprint` | hash | 账号/checkout 的脱敏指纹 |
| `voucher_replay_contract` | object | voucher 子合同 |

禁止记录：

- PIN、OTP、完整 token、cookie、refresh token。
- 完整手机号、完整邮箱、完整 checkout URL 中的敏感 query。
- 真实支付凭证原文。

## 7. 已落地的单元测试

当前后端已补齐以下合同级测试：

- `TestPaymentVoucherReplayRiskRejectsCrossAccountCheckoutAndSensitiveMaterial`
- `TestPaymentVoucherReplayRiskRejectsUnboundAuditVoucher`
- `TestPaymentVoucherReplayRiskRejectsEvenMatchingVoucherAsEvidenceOnly`
- `TestPaymentAuthorizationTamperRiskRejectsForgedClientClaims`
- `TestPaymentAuthorizationTamperRiskRejectsLocalEvidenceInjection`
- `TestPaymentAuthorizationTamperRiskRejectsCrossAccountVoucherInjection`
- `TestPaymentAuthorizationTamperRiskRejectsOldCheckoutReplay`
- `TestSandboxPaymentAuthorizationAssessFullFlowRejectsAllMockScenarios`
- `TestSandboxPaymentAuthorizationAssessCustomVoucherUsesMockContext`
- `TestHandleSandboxPaymentAuthorizationAssessRequiresLocalAccess`
- `TestHandleSandboxPaymentAuthorizationAssessFullFlowResponse`

这些测试只验证本地拒绝合同，不驱动真实浏览器付款。

## 8. 实施计划

### 阶段 0：合同固化（已完成）

- 建立 voucher replay 风险合同。
- 建立 payment authorization tamper 风险合同。
- 为四类 sandbox 攻击场景补单元测试。

验收：聚焦测试与全量 Go 测试通过。

### 阶段 1：Sandbox API（已完成）

- 新增 `/api/sandbox/payment-authorization/assess`。
- 新增固定 scenario fixture。
- 仅允许本机访问，默认不暴露到公网。
- 响应中显式声明 `payment_action_executed=false`、`real_checkout_touched=false`、`real_credential_used=false`。

验收：每个 scenario 都返回 `payment_authorization_tamper_rejected`。

### 阶段 2：状态机接入（1-2 天）

- 在完整 checkout 状态机中增加 `authorization_authority` 字段。
- terminal 成功态只接受 `current_checkout_server_state`。
- 当前端 evidence 与服务端状态冲突时，服务端状态优先，并输出 tamper 诊断。

验收：客户端伪造 `gopay_complete` 不会覆盖 run-level final state。

### 阶段 3：审计与回放（1 天）

- 为 sandbox 场景生成脱敏审计记录。
- 增加 replay fixture 回归测试：最近 5 个流程的状态序列不能被旧 voucher 覆盖。

验收：审计日志可定位风险原因，但不包含敏感凭证。

### 阶段 4：前端安全可视化（0.5-1 天）

- 在调试面板显示 `risk_reasons` 与 `trusted_authority_needed`。
- 对 `authorization_accepted=false` 的 terminal 合同停止轮询，提示重新生成当前 checkout。

验收：用户无需读原始日志即可知道为何拒绝。

## 9. 风险与缓解

| 风险 | 影响 | 缓解 |
| --- | --- | --- |
| 把 sandbox 误接入真实支付流 | 可能造成误操作 | 路由命名带 `/sandbox/`，handler 禁止 CDP 写入和付款动作 |
| 日志误存敏感凭证 | 凭证泄露 | 复用现有审计脱敏，新增禁止字段测试 |
| 客户端 evidence 覆盖服务端状态 | 状态机被污染 | `accepted_authority=false` 时禁止进入成功态 |
| 误判正常支付完成为 tamper | 用户体验下降 | 只对客户端自称成功、旧凭证和绑定不一致触发；当前服务端状态另行判定 |

## 10. 最终验收标准

- 四类 sandbox 场景全部被拒绝。
- 所有拒绝合同都包含明确 `risk_reasons`。
- 任何历史 voucher 即使账号、checkout、reference 完全匹配，也只能作为审计 evidence，不能作为授权。
- localStorage/window evidence 永远不能单独把 checkout 推进到 `payment_completed`。
- 跨账号、跨 checkout、含敏感材料的 voucher 必须被标记为 high risk。
- 全量 `go test ./...` 通过。
