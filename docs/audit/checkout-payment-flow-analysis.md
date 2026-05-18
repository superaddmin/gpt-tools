# Checkout 支付完成链路分析与报错日志方案

最后核对日期：2026-05-17

## 1. 文档目的

本文记录当前 Checkout Workbench 中从账号准备、checkout 链接生成、支付页填充、GoPay 绑定、PIN 支付到最终状态观测的完整信息流、异常点和日志落盘方案。本文以当前源码为事实来源，主要覆盖 [main.go](file:///f:/chatadd/main.go)、[app.js](file:///f:/chatadd/web/app.js)、[index.html](file:///f:/chatadd/web/index.html) 和 [styles.css](file:///f:/chatadd/web/styles.css)。

本项目当前定位是本地 checkout/payment 流程编排与观测工作台，不是传统订单系统。因此当前没有数据库订单表、订单状态表或异步支付回调表。支付链路的可追踪事实主要来自：

- 前端运行态状态与页面展示。
- Go 后端 API 返回体。
- Chrome CDP 页面、网络、控制台观测结果。
- `log/` 目录中的账号维度 JSON Lines 审计日志。

## 2. 当前支付全流程总览

```text
账号/浏览器准备
  → 打开无痕窗口 / 读取 Session
  → 生成 OpenAI/Stripe checkout 链接
  → 解析或锁定真实 checkout 支付页
  → 自动填充账单地址与支付页信息
  → 判断 GoPay 是否可触发
  → 进入 Midtrans/GoPay linking 页面
  → 填充手机号并推进绑定
  → OTP/PIN 页面观测或操作
  → GoPay 全流程支付辅助
  → 查询/提取支付凭证和最终状态
  → 前端渲染最终状态并写入账号日志
```

## 3. 关键 API 与链路职责

| 阶段 | API | 职责 | 主要输出 | 日志阶段 |
| --- | --- | --- | --- | --- |
| 健康检查 | `/api/health` | 确认本地服务可用 | 服务状态 | `system_health` |
| 浏览器准备 | `/api/incognito/open` | 启动或复用系统 Chrome 无痕窗口 | 浏览器启动结果 | `login_browser_prepare` |
| Session 获取 | `/api/session/fetch` | 从浏览器上下文读取 Session JSON | session 结果或错误 | `session_capture` |
| Checkout 生成 | `/api/checkout`、`/api/checkout/start` | 基于 token、套餐、账单地区生成 checkout URL | `checkout_url`、`checkout_session_id` | `checkout_create` |
| 目标解析 | `/api/checkout/resolve-target` | 锁定当前真实 checkout 支付页 | `target_url`、页面诊断 | `checkout_target_resolve` |
| 页面填充 | `/api/checkout/auto-fill` | 自动填写地址、选择或激活支付方式 | 自动化动作结果 | `checkout_page_fill` |
| GoPay 触发判断 | `/api/gopay/auto-trigger-check` | 判断是否满足 GoPay 自动触发条件 | `ready`、`reason`、`conditions` | `gopay_auto_trigger_decision` |
| Midtrans 填充 | `/api/gopay/midtrans-linking-fill` | 在 Midtrans/GoPay 页面填充手机号 | 页面填充/跳转结果 | `gopay_midtrans_page_linking` |
| GoPay 全流程 | `/api/gopay/full-link` | 执行绑定、PIN 支付、状态提取 | `stage`、`payment_voucher`、`transaction_id` | `gopay_full_payment_flow` |
| 页面监控 | `/api/gopay/monitor` | 采集支付页面、网络、控制台事件 | 页面与网络诊断 | `payment_monitoring` |
| 定价监控 | `/api/pricing/monitor` | 采集定价页状态 | 页面诊断 | `pricing_monitoring` |

## 4. 支付结果数据接收机制

当前项目没有独立的支付回调接收端，也没有数据库状态机。支付结果主要通过以下路径进入系统：

1. 后端主动调用或解析 GoPay/Midtrans 相关接口，返回支付处理结果。
2. CDP 监控读取浏览器当前页面 URL、文本、网络事件和控制台事件。
3. 前端根据 API 返回体中的 `ok`、`stage`、`ready`、`reason`、`payment_voucher`、`transaction_id` 等字段渲染状态。
4. 审计中间件将每个 `/api/` 请求和响应摘要写入 `log/`，用于后续复盘。

这意味着最终状态不是由数据库订单表驱动，而是由 API 调用结果、浏览器页面观测和审计日志共同构成。

## 5. 页面状态转换规则

前端页面的状态转换依赖 `web/app.js` 中的异步 API 调用结果，典型规则如下：

- API 请求开始：按钮进入执行中状态，流程监控追加操作日志。
- Checkout 链接生成成功：展示 checkout URL，允许复制、打开无痕窗口、自动填充。
- 目标解析成功：锁定 checkout 页面目标，后续自动填充优先使用锁定目标。
- 自动填充成功：页面提示填充动作、目标 URL、是否触发 GoPay。
- GoPay 触发检查 `ready=false`：展示等待原因，例如未提交 checkout、未检测到 GoPay、非 0 元首期等。
- GoPay 触发检查 `ready=true`：允许或自动进入 Midtrans/GoPay 填充链路。
- GoPay 全流程 `ok=true` 且 `stage=gopay_complete`：展示支付完成摘要和凭证字段。
- GoPay 全流程 `ok=false`：展示失败阶段、错误原因、可复盘诊断字段。

## 6. 正常信息流路径

```text
用户提供账号 token / session
  → POST /api/checkout
  → 后端调用 checkout endpoint 并初始化 Stripe 页面
  → 返回 checkout_url 和 checkout_session_id
  → 前端打开或锁定 checkout 页面
  → POST /api/checkout/resolve-target
  → POST /api/checkout/auto-fill
  → POST /api/gopay/auto-trigger-check
  → ready=true
  → POST /api/gopay/midtrans-linking-fill
  → POST /api/gopay/full-link
  → 返回 stage=gopay_complete
  → 提取 payment_reference_id / transaction_id / payment_voucher
  → 前端最终状态展示
  → 审计日志落盘到 log/{account_email}_{yyyyMMdd_HHmmss_000}.json
```

## 7. 异常与报错分析

| 异常点 | 触发条件 | 当前处理 | 建议分析字段 |
| --- | --- | --- | --- |
| Session 获取失败 | 浏览器未登录、CDP 不可达、返回体不符合预期 | API 返回错误，前端提示失败 | `stage`、`error`、`duration_ms`、`request_user_agent` |
| Checkout 链接生成失败 | token 失效、checkout endpoint 异常、网络错误 | 返回 HTTP 错误或业务错误 | `checkout_session_id`、`status_code`、`response_summary.error` |
| 非支付页目标 | 当前页面不是 `pay.openai.com` checkout 页 | `auto-trigger-check` 返回 `not_checkout_payment_page` | `checkout_key`、`target_url`、`reason` |
| 未检测到 GoPay | 页面没有 GoPay 文案或未切换支付方式 | 返回 `gopay_not_detected` | `conditions.gopay_detected`、`stage` |
| 首期金额不符合 | 页面不是 0 元首期或文本解析失败 | 返回 `today_due_not_zero` | `conditions.today_due_zero`、`reason` |
| 重复触发 | 同一 checkout key 已被后端 claim | 返回 `checkout_already_triggered` | `checkout_key`、`already_triggered` |
| Midtrans 页面填充失败 | 页面未加载、输入框不可见、CDP 操作失败 | 返回 browser/page 诊断 | `browser_result`、`network_diagnostics` |
| OTP 需要人工处理 | 页面存在 OTP 输入或验证码流程 | 标记 `otp_manual_required` | `has_otp_field`、`otp_manual_required` |
| PIN 支付失败 | PIN 候选均失败或支付接口拒绝 | `stage=payment_pin_all_failed` 或 `gopay_payment_failed` | `pin_stage`、`stages`、`error`，不记录明文 PIN |
| 最终状态不明确 | Midtrans 状态接口无有效交易状态 | 保留支付引用和网络诊断 | `payment_reference_id`、`transaction_id`、`midtrans_status` |

## 8. 日志落盘方案

当前后端的审计中间件覆盖所有 `/api/` 路由。每次 API 调用都会构建一条 JSON Lines 日志记录并写入 `log/` 目录。文件命名规则为：

```text
log/{account_email}_{yyyyMMdd_HHmmss_000}.json
```

如果无法识别账号邮箱，则落盘到：

```text
log/unknown_account_{yyyyMMdd_HHmmss_000}.json
```

单条日志包含以下核心字段：

- `operation_time`：操作时间，RFC3339Nano。
- `operation_type`：操作类型，例如 `checkout_create`、`gopay_full_flow`。
- `operation_name`：中文操作名称。
- `operation_detail`：请求载荷脱敏摘要。
- `operation_result`：`success` 或 `failed`。
- `status_code`：HTTP 状态码。
- `account_email`：账号邮箱，优先来自 `X-Account-Email` 或请求载荷。
- `ip_address`：客户端 IP。
- `request_path` / `request_method`：接口路径和方法。
- `error_message`：HTTP 或业务失败摘要。
- `metadata`：分析用扩展字段。

## 9. 新增专业分析日志字段

为支持后续支付流程优化，当前已增强 `metadata` 字段：

| 字段 | 含义 |
| --- | --- |
| `analysis_log_version` | 当前日志结构版本，值为 `checkout-payment-flow-v1` |
| `duration_ms` | API 处理耗时 |
| `request_body_bytes` | 原始请求体字节数 |
| `captured_response_body_bytes` | 被审计捕获的响应体字节数 |
| `account_log_file_prefix` | 账号日志文件名前缀 |
| `flow_stage` | 归一化支付链路阶段 |
| `flow_step_index` | 阶段排序索引，用于还原执行顺序 |
| `flow_route_role` | 当前接口在全流程中的职责说明 |
| `request_host` | 请求 Host |
| `request_user_agent` | User-Agent 摘要，经过敏感字符串检测 |
| `request_payload` | 脱敏后的请求载荷 |
| `response_summary` | 响应摘要字段 |
| `operation_flow` | 支付链路关键字段集合 |
| `payment_voucher` | 支付凭证摘要，不包含 PIN/OTP/token |
| `correlation_identifiers` | 跨接口关联标识，例如 checkout session、payment reference、transaction id |

## 10. 敏感信息安全边界

支付流程日志必须可分析，但不能泄露敏感数据。当前脱敏策略覆盖：

- `token`、`access_token`。
- Cookie、authorization。
- OTP、PIN、PIN token。
- `client_secret`、session JSON。
- GoPay challenge、payment method id 等敏感支付中间值。
- 类 JWT 长字符串。

`correlation_identifiers` 只收录用于排查链路的非密钥字段，例如 `checkout_session_id`、`payment_reference_id`、`transaction_id`，不会收录 `pin`、`otp`、`token`、`cookie`。

## 11. 后续优化建议

1. 在前端每次完整支付链路开始时生成 `flow_id`，随所有 API 请求传入后端，进一步提升跨接口关联能力。
2. 在最终状态区域展示最近一次 `payment_reference_id`、`transaction_id` 和失败阶段，方便人工复盘。
3. 将 `log/` 中 JSON Lines 后续接入离线分析脚本，按账号、阶段、耗时、失败原因聚合。
4. 对 `gopay_auto_trigger_decision` 阶段建立失败原因分布统计，优先优化高频阻断原因。
5. 对 `duration_ms` 和 `network_diagnostics` 做慢请求分析，定位页面卡顿、支付跳转延迟和外部接口波动。
6. 如果后续引入真实订单系统，应增加数据库订单状态表、支付事件表、回调验签和幂等处理，本地日志只作为辅助审计。

## 12. 结论

当前实现已经形成账号维度、接口维度、支付阶段维度的审计日志基础。增强后的日志能够完整保存 checkout 到 GoPay 支付完成链路中的关键请求、响应摘要、阶段职责、耗时、支付凭证和错误原因，同时保持 OTP、PIN、token、Cookie 等敏感信息脱敏。后续可基于 `log/` 中的 JSON Lines 记录持续分析页面流畅度、支付阻断点和异常路径，从而优化全流程支付页面体验。
