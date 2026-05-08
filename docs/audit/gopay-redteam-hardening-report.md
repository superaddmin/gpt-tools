# GoPay 本地红黑测试与安全加固报告

最后核对日期：2026-05-08

## 1. 文档目的

本文记录 2026-05-08 在本地环境对 Checkout Workbench / GoPay 辅助链路执行的红黑测试结果，以及随后落地的服务端安全加固。本文以当前源码和本地验证结果为事实来源，主要覆盖 [main.go](file:///f:/chatadd/main.go)、[main_test.go](file:///f:/chatadd/main_test.go)、[app.js](file:///f:/chatadd/web/app.js) 与既有审计文档 [checkout-payment-flow-analysis.md](file:///f:/chatadd/docs/audit/checkout-payment-flow-analysis.md)。

本文刻意不触发真实支付，不提取、不复用任何历史敏感凭证；所有复用与重放验证均基于本地 defensive simulation 接口与只读日志审计完成。

## 2. 测试范围

本轮覆盖四类目标：

1. 正常流程验证：本地页面、核心路由、基础工作流可用性。
2. 异常流程验证：超时、重复请求、缺失字段、页面未就绪等异常路径。
3. 边界条件验证：0 元试用资格、非法 scheme、超长监控时长、大响应体。
4. 安全验证：资格复用、支付信息重放、跨账号复用、模拟 SQL 注入输入、监控接口资源消耗。

## 3. 测试方法

### 3.1 黑盒入口

- 主页面与本地 API：`http://127.0.0.1:18473`
- 重点接口：
  - `/api/redteam/eligibility-replay-simulate`
  - `/api/redteam/payment-replay-last`
  - `/api/redteam/payment-replay-simulate`
  - `/api/gopay/monitor`
  - `/api/pricing/monitor`
  - `/api/incognito/open`

### 3.2 事实来源

- 路由注册与后端逻辑：[main.go](file:///f:/chatadd/main.go)
- 单元测试与验证辅助：[main_test.go](file:///f:/chatadd/main_test.go)
- 前端调用与状态展示：[app.js](file:///f:/chatadd/web/app.js)
- 审计日志：`log/*.log`

### 3.3 安全边界

- 不执行真实支付。
- 不自动复用历史支付资格或支付凭证。
- 不导出 OTP、PIN、Cookie、token、client secret 等敏感信息。
- 所有“重放”仅用于验证服务端拒绝策略与审计行为。

## 4. 关键测试用例与结果

| 类别 | 用例 | 预期 | 实际结果 | 结论 |
| --- | --- | --- | --- | --- |
| 正常 | 主页面加载 | 页面和静态资源可访问 | 已完成基础访问验证 | 通过 |
| 正常 | 资格复用同主体模拟 | 同账号上下文被识别为同主体，但不生成真实资格 | 返回 `eligibility_context_verified` | 通过 |
| 安全 | 跨账号资格复用模拟 | 新试用请求被拒绝 | 返回 `eligibility_replay_rejected` | 通过 |
| 安全 | 跨账号支付信息复用模拟 | 支付上下文复用被拒绝 | 返回 `payment_replay_rejected` | 通过 |
| 安全 | 金额不一致重放 | 因金额不一致拒绝 | 返回 `payment_amount_mismatch_detected` | 通过 |
| 安全 | request nonce 重复 | 因 nonce 复用拒绝 | 返回 `request_nonce_reuse_detected` | 通过 |
| 安全 | 非法 URL scheme | 服务端拒绝非 http/https URL | 返回 400 | 通过 |
| 异常 | monitor 超长持续时间 | 服务端应限制时长，避免长阻塞 | 加固前存在上限偏大问题 | 已修复 |
| 边界 | pricing monitor 大量资源抓取 | 服务端应限制响应规模 | 加固前返回体偏大 | 已修复 |
| 审计 | 日志复盘支付链路 | 能复盘摘要但不可导出敏感凭证 | 满足只读脱敏要求 | 通过 |

## 5. 主要发现

### 5.1 已确认有效的防御能力

- 资格复用模拟接口明确要求 `mode=defensive_simulation`，拒绝非模拟模式请求，见 [validateEligibilityReplaySimulationRequest](file:///f:/chatadd/main.go#L1210-L1228)。
- 支付重放模拟接口会综合账号、checkout session、nonce、金额、币种、支付引用等信号做拒绝判定，相关逻辑位于 [handlePaymentReplaySimulate](file:///f:/chatadd/main.go#L1370-L1423) 和对应评估逻辑所在区域。
- 0 元试用链路已存在金额校验，非 0 元上下文会在 checkout 阶段被阻断，该能力已在既有测试中覆盖，见 [main_test.go](file:///f:/chatadd/main_test.go)。
- 审计日志对请求与响应做摘要保留，同时保持敏感字段脱敏，符合只读审计目标，见 [checkout-payment-flow-analysis.md](file:///f:/chatadd/docs/audit/checkout-payment-flow-analysis.md#L120-L182)。

### 5.2 加固前存在的主要问题

- `/api/redteam/*` 为本地调试模拟接口，但默认可被调用，存在误触发与暴露风险。
- `/api/gopay/monitor` 的默认与最大监控时长偏大，非流式场景可能积累较大事件数组。
- `/api/pricing/monitor` 的抓取内容较多，资源条目、提示片段、body 文本与 storage 摘要可能导致响应体过大。

## 6. 已落地的服务端加固

### 6.1 redteam 接口保护

本轮已在 [main.go](file:///f:/chatadd/main.go#L1294-L1330) 新增以下保护：

- `EnableRedteamAPIs` 配置项
- `redteamAPIsEnabled()`
- `isLoopbackRequest()`
- `ensureRedteamAccess()`

保护策略如下：

1. redteam 接口默认关闭。
2. 只有显式开启 `ENABLE_REDTEAM_APIS=true` 或配置中开启后才允许访问。
3. 请求必须来自 loopback 地址，否则返回 403。

已接入的接口：

- [handleEligibilityReplaySimulate](file:///f:/chatadd/main.go#L1186-L1205)
- [handlePaymentReplayLast](file:///f:/chatadd/main.go#L1338-L1354)
- [handlePaymentReplaySimulate](file:///f:/chatadd/main.go#L1370-L1389)

### 6.2 monitor 限时与裁剪

已在 [handleGopayMonitor](file:///f:/chatadd/main.go#L9182-L9243) 落地以下调整：

- 最大时长由 `120s` 收紧到 `30s`
- 默认时长由 `30s` 收紧到 `12s`
- 非流式返回增加 `truncateMonitorEvents(events, 120)` 裁剪

对应辅助函数：

- [truncateMonitorEvents](file:///f:/chatadd/main.go#L9245-L9250)

### 6.3 pricing monitor 限时与限长

已在 [handlePricingMonitor](file:///f:/chatadd/main.go#L9394-L9498) 落地以下调整：

- 最大时长由 `180s` 收紧到 `60s`
- 默认时长由 `12s` 收紧到 `8s`
- `resources` 上限由 200 收紧到 80
- `promo_hints` 上限由 40 收紧到 20
- `body_text` 长度由 5000 收紧到 2000
- `localStorage` / `sessionStorage` 键数由 80 收紧到 30
- 单个 storage 值长度由 300 收紧到 160

## 7. 验证结果

本轮已通过单元测试验证加固逻辑，命令如下：

```bash
go test ./...
```

结果：通过。

重点新增/修正测试包括：

- redteam 关闭时返回 403
- 非 loopback 访问 redteam 返回 403
- loopback 请求识别正确
- monitor 事件裁剪逻辑正确
- 原有 redteam 测试在开启配置和本机来源下继续通过

测试事实来源：

- [main_test.go](file:///f:/chatadd/main_test.go)

## 8. 覆盖范围评估

### 8.1 已覆盖

- 同主体与跨主体资格复用判定
- 支付上下文复用、金额不一致、nonce 复用、引用复用
- monitor / pricing monitor 的资源消耗边界
- 审计日志的只读脱敏复盘能力

### 8.2 尚未覆盖

- 真实第三方支付网络抖动下的全链路状态一致性
- 外部支付平台真实回调，因为当前系统并无独立 webhook/callback 接收端
- 持久化账号/IP/设备指纹风控，因为当前系统仍主要依赖当前会话与模拟判定，不是完整风控系统

## 9. 结论

本轮红黑测试表明，当前系统在“本地安全模拟”模式下已经能够较好地识别并拒绝资格复用、支付信息重放、跨账号上下文复用等高风险行为；同时，审计日志可以支持只读复盘，但不会直接泄露可用于真实支付的敏感凭证。

在此基础上，本轮又完成了服务端首轮加固：redteam 接口已默认关闭并限制为本机显式启用，monitor 与 pricing monitor 已增加更严格的限时与返回裁剪。相比加固前，当前版本更适合作为本地诊断工作台使用，资源边界和调试接口暴露面都更收敛。

## 10. 后续建议

1. 为 monitor 类接口增加简单频率限制或并发保护，继续降低误用时的本地资源占用。
2. 如果后续需要更强的资格复用防护，应引入持久化主体标识、设备指纹与风控命中记录，而不仅是当前会话级判断。
3. 若未来接入真实订单系统，应补充独立回调接收、验签、幂等与订单状态机；本地日志继续只做辅助审计。
