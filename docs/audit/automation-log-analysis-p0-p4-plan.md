# 自动化日志全量分析与 P0-P4 改造方案

最后核对日期：2026-05-11

## 1. 文档目的

本文基于 `log/` 目录中的全量本地审计日志，对 Checkout Workbench 的自动化链路进行系统分析，并给出 P0-P4 分级改造方案。目标是减少无效轮询、降低人为干预、提升 GoPay/Checkout/CDP 自动化稳定性，并让 PIN/OTP/支付页状态具备可验证、可复盘、可维护的工程闭环。

事实来源：

- 后端路由、审计字段、CDP/GoPay/Checkout 逻辑：[main.go](../../main.go)
- 前端流程调度、按钮工作流、轮询器、状态渲染：[web/app.js](../../web/app.js)
- 前端页面结构：[web/index.html](../../web/index.html)
- 日志样本：`log/*.log`
- 既有支付链路文档：[checkout-payment-flow-analysis.md](checkout-payment-flow-analysis.md)

## 2. 日志样本与标准化框架

### 2.1 样本范围

本轮分析解析了 `log/` 下全部非空 JSON 日志：

| 指标 | 数值 |
| --- | ---: |
| 日志条数 | 17,780 |
| 日志体积 | 57.59 MB |
| 起始时间 | 2026-05-07 14:06 |
| 结束时间 | 2026-05-11 14:26 |

### 2.2 标准化字段

所有日志统一抽取为以下分析维度：

| 维度 | 字段 | 用途 |
| --- | --- | --- |
| 时间线 | `operation_time`、小时、日期 | 还原流程阶段、识别高频轮询窗口 |
| 接口 | `request_path`、`request_method` | 归类流程节点 |
| 结果 | `operation_result`、`status_code`、`error_message` | 区分业务失败、传输失败、软失败 |
| 阶段 | `metadata.operation_flow.stage` | 状态机节点归一化 |
| 自动化信号 | `pin_auto_filled`、`pin_auto_submitted`、`pay_now_block_reason`、`cdp_url_host` | 判断自动操作是否真正发生 |
| CDP 目标 | `selected_target_id`、`selected_target_url`、`execution_context_frame_url` | 判断是否选中正确页面/iframe |
| 关联键 | `trace_id`、`checkout_key`、`account_id`、`target_id` | 跨接口串联同一任务 |
| 性能 | `duration_ms`、P95 | 识别慢接口与等待策略问题 |

### 2.3 日志分析方法

使用 PowerShell 对 JSON 日志批量解析：

```powershell
Get-ChildItem -Path log -File |
  Where-Object {$_.Length -gt 0} |
  ForEach-Object {
    $j = Get-Content -Raw -Path $_.FullName | ConvertFrom-Json
    [pscustomobject]@{
      time = $j.operation_time
      path = $j.request_path
      result = $j.operation_result
      status = $j.status_code
      error = $j.error_message
      stage = $j.metadata.operation_flow.stage
      duration = $j.metadata.duration_ms
    }
  }
```

## 3. 总体统计结论

### 3.1 接口调用量

| 接口 | 调用量 | 主要含义 |
| --- | ---: | --- |
| `/api/pricing/plus-subscribe-probe` | 5,955 | Plus/订阅支付页探测轮询 |
| `/api/gopay/cdp-otp` | 4,954 | GoPay OTP/PIN/Pay now 页面轮询与自动化 |
| `/api/checkout/resolve-target` | 1,428 | Checkout 支付目标锁定 |
| `/api/gopay/auto-link` | 323 | GoPay 绑定 API 路径 |
| `/api/incognito/open` | 309 | 无痕窗口准备 |
| `/api/gopay/force-link` | 230 | GoPay 强制绑定 |
| `/api/login/click` | 190 | 登录入口点击 |
| `/api/luckmail/token-mails` | 182 | LuckMail 邮件列表读取 |
| `/api/checkout/start` | 142 | Checkout 链接生成 |
| `/api/session/fetch` | 127 | Session JSON 获取 |

### 3.2 失败率与耗时

| 接口 | 调用量 | 失败率 | 平均耗时 | P95 | 结论 |
| --- | ---: | ---: | ---: | ---: | --- |
| `/api/pricing/plus-subscribe-probe` | 5,955 | 100.0% | 38.7 ms | 8 ms | 典型高频无效轮询，耗时不高但噪声极大 |
| `/api/gopay/cdp-otp` | 4,954 | 24.5% | 61.1 ms | 364 ms | CDP target 不稳定，常在页面未就绪时轮询 |
| `/api/gopay/auto-link` | 323 | 99.1% | 1991.7 ms | 4940 ms | Token 过期/不存在/429 是主要失败源 |
| `/api/login/click` | 190 | 45.3% | 28826.1 ms | 57655 ms | 登录页面/弹窗等待成本高 |
| `/api/login/email-fill` | 105 | 29.5% | 27872.1 ms | 119148 ms | 邮箱输入框 readiness 不稳定 |
| `/api/login/code-fill` | 59 | 52.5% | 39058.3 ms | 60871 ms | 验证码输入框定位与页面切换问题明显 |
| `/api/luckmail/token-code` | 73 | 8.2% | 16636.9 ms | 95411 ms | 接码成功率较好，耗时来自等待邮件 |
| `/api/checkout/start` | 142 | 9.9% | 7415.4 ms | 15750 ms | 主要受外部 checkout API 影响 |

## 4. 关键问题诊断

### 4.1 Plus 探测高频空转

`/api/pricing/plus-subscribe-probe` 是最大噪声源，日志显示最新阶段页面停留在：

```text
https://chatgpt.com/
```

页面按钮包含：

```text
登录 / 免费注册 / 查看套餐和定价
```

但探测仍然持续执行。典型组合：

| 组合 | 次数 | 说明 |
| --- | ---: | --- |
| `plus_subscribe_probe, plus=true, subscribe_payment=false, safe=false, cdp_ready=true, chatgpt_count=1` | 2,972 | 已识别 Plus 相关内容，但未进入订阅并付款态 |
| `plus_subscribe_probe, plus=false, subscribe_payment=false, safe=false, cdp_ready=true, chatgpt_count=1` | 1,060 | ChatGPT 页面存在，但不在订阅页 |
| `chatgpt_target_not_found` | 889 | 找不到 ChatGPT target |
| `cdp_not_ready` | 330 | CDP 不可用 |

结论：当前探测缺少“无效状态熔断”和“下一动作推荐”。当页面停在首页或登录入口时，系统仍持续探测订阅页，造成重复劳动和误导。

### 4.2 GoPay CDP 目标不稳定

`/api/gopay/cdp-otp` 失败来源：

| 错误 | 次数 |
| --- | ---: |
| `CDP not ready` | 540 |
| `未在 CDP 中找到匹配 midtrans.com 的页面` | 393 |
| `未在 CDP 中找到 GoPay OTP/PIN 页面` | 278 |

结论：GoPay 监听器经常在没有支付页 target、没有 Midtrans target、或 Chrome CDP 尚未就绪时启动。当前缺少前置 target readiness gate。

### 4.3 PIN 自动输入的真实现状

PIN 相关日志显示：

| PIN 阶段 | 自动填入 | 自动提交 | 次数 | 说明 |
| --- | --- | --- | ---: | --- |
| binding | true | true | 20 | 第一处绑定 PIN 有成功记录 |
| payment | true | true | 6 | 付款确认 PIN 历史上有成功记录 |
| binding | false | false | 110 | 多数为等待、缺少 PIN、已处理或历史旧逻辑 |
| payment | false | false | 5 | 支付 PIN 曾出现但未自动提交，需要上下文和输入策略保护 |

最新修复已经加入：

- `pin_context_selected`
- `execution_context_frame_url`
- `pin_input_event_source`
- `pin_input_observed_value`
- `pin_input_events`

用于区分：

- `automation_synthetic`：脚本自动输入
- `user_trusted_event`：人工真实输入
- `existing_value_observed_no_event`：已有值但未捕获输入事件

当前最新失败并不是 PIN 脚本失败，而是流程没有进入 `/api/gopay/cdp-otp`。

### 4.4 GoPay 绑定 token 生命周期缺陷

GoPay 绑定失败分组：

| 错误 | 次数 | 说明 |
| --- | ---: | --- |
| `token not found` | 239 auto-link + 73 force-link | token/reference 已不存在 |
| `token has expired` | 104 force-link | token 已过期 |
| `429` | 81 auto-link | 重试过快或被限制 |

结论：需要将 token/reference 视为短生命周期资源。出现过期、404、429 后应立即改变状态，而不是重复尝试同一 token。

### 4.5 登录链路人工干预点

登录失败分组：

| 阶段 | 次数 | 说明 |
| --- | ---: | --- |
| `login_window_not_ready_after_click` | 53 | 点击后登录窗口未稳定出现 |
| `login_code_input_not_found` | 29 | 验证码输入框未定位 |
| `login_popup_not_opened` | 12 | 登录弹窗未打开 |
| `login_email_operation_timed_out` | 10 | 邮箱填入等待过长 |
| `login_email_input_not_found` | 10 | 邮箱输入框未找到 |

结论：登录链路应被状态机托管，并明确区分：

- 页面未加载
- 弹窗未出现
- 电子邮箱模式未切换
- 输入框存在但不可交互
- 验证码等待中

### 4.6 LuckMail 稳定性评估

LuckMail 成功项：

| 接口 | 成功次数 |
| --- | ---: |
| `/api/luckmail/token-mails` | 166 |
| `/api/luckmail/token-code` | 67 |
| `/api/luckmail/purchases` | 35 |

问题项：

- `create-and-wait` 4 次全部失败或等待结束。
- `token-code` 少量失败，主要来自等待结束、token 格式、API 请求错误。

结论：LuckMail 更适合“手动/配置 token 后分步轮询”，不适合把创建和等待强绑定为一个长阻塞动作。

## 5. 自动化目标

### 5.1 总目标

将当前“按钮串联 + 分散轮询”的流程升级为“状态机驱动 + readiness gate + 熔断/退避 + 可观测审计”的自动化系统。

### 5.2 量化目标

| 目标 | 当前状态 | 目标状态 |
| --- | --- | --- |
| Plus 探测无效轮询 | 5,955 次，100% 失败 | 减少 80%-90% |
| CDP 无目标失败 | 1,213 次 cdp-otp 失败 | 减少 50%+ |
| GoPay 无效 token 重试 | 300+ 次 | 同一 token 失败后不再重复尝试 |
| 登录等待超时 | P95 57-119 秒 | 超时前给出明确下一动作 |
| PIN 输入来源判断 | 历史日志无法判断人工/自动 | 通过 `pin_input_event_source` 精确区分 |
| 支付链路稳定性 | 状态分散在多个按钮 | 状态机统一托管 |

## 6. P0-P4 详细改造方案

## P0：建立支付流程状态机

### P0.1 目标

消除“任意按钮可重复触发任意接口”的混乱状态，统一流程入口、状态转移、错误处理和恢复动作。

### P0.2 状态定义

```text
idle
  -> incognito_opening
  -> login_guiding
  -> login_email_filling
  -> login_code_waiting
  -> session_fetching
  -> checkout_creating
  -> checkout_target_resolving
  -> checkout_autofilling
  -> payment_method_selecting
  -> payment_page_opening
  -> gopay_linking
  -> gopay_otp_waiting
  -> gopay_pin_binding
  -> gopay_pay_now_ready
  -> gopay_pay_now_clicked
  -> gopay_pin_payment
  -> payment_completed
  -> record_exporting
  -> browser_closing

terminal:
  payment_failed
  payment_expired
  cancelled
  manual_required
  blocked
```

### P0.3 状态转移规则

| 当前状态 | 进入条件 | 成功条件 | 失败/暂停条件 | 下一状态 |
| --- | --- | --- | --- | --- |
| `incognito_opening` | 用户启动流程 | CDP ready + ChatGPT target 存在 | CDP 启动失败 | `login_guiding` |
| `login_guiding` | ChatGPT 页面可见 | 登录弹窗/已登录 shell | 登录按钮不可交互 | `login_email_filling` 或 `session_fetching` |
| `session_fetching` | 已登录 | 读取 session JSON | 未登录/会话无效 | `login_guiding` |
| `checkout_creating` | session/token 有效 | 返回 checkout URL | checkout API 401/502/409 | `manual_required` 或重试 |
| `checkout_autofilling` | checkout target 锁定 | 地址填入完成、支付方式选中 | target 丢失 | `checkout_target_resolving` |
| `gopay_linking` | Midtrans/GoPay 页面存在 | 进入 OTP/PIN | token 404/407/429 | `manual_required` 或 token 重建 |
| `gopay_pin_binding` | PIN binding 输入框存在 | `pin_auto_submitted=true` | PIN 缺失/已提交/输入失败 | `manual_required` |
| `gopay_pay_now_clicked` | Pay now 按钮存在 | 一次真实 CDP 鼠标点击 | 禁止重复点击 | `gopay_pin_payment` 或继续监听 |
| `gopay_pin_payment` | payment PIN context 存在 | `pin_auto_submitted=true` | context 未出现/输入失败 | `manual_required` |

### P0.4 实现路径

前端新增一个集中式状态对象：

```js
const automationRun = {
  runId: "",
  state: "idle",
  previousState: "",
  enteredAt: 0,
  attempts: {},
  locks: {},
  lastError: null,
  context: {
    traceId: "",
    checkoutUrl: "",
    checkoutKey: "",
    accountId: "",
    cdpTargetId: "",
    selectedPaymentMethod: ""
  }
};
```

后端保持现有接口，但响应体需要继续强化：

- 所有关键 API 返回 `stage`、`next_action`、`retryable`、`terminal`。
- 软失败使用明确业务阶段，例如 `chatgpt_home_detected`、`payment_target_missing`、`gopay_token_expired`。
- 审计日志继续保留 `trace_id`、`checkout_key`、`target_id`、`account_id`。

### P0.5 验收标准

- 同一时刻只允许一个主流程 run。
- 每个状态都有可观测进入时间、重试次数、退出原因。
- 不再出现首页上持续刷 `plus-subscribe-probe` 而没有下一动作的情况。
- `/api/gopay/cdp-otp` 只在确认存在 Midtrans/GoPay target 后启动。

## P1：削减 Plus 探测无效轮询

### P1.1 目标

减少 `plus-subscribe-probe` 的高频空转，并在检测到错误页面时主动切换动作。

### P1.2 当前问题

5 月 11 日 11 点到 14 点之间，每小时有 `692-1702` 次 `plus-subscribe-probe`。大量日志显示页面停在 `https://chatgpt.com/` 首页，但探测器仍按订阅页逻辑持续轮询。

### P1.3 策略

引入探测分类：

| 分类 | 判断条件 | 动作 |
| --- | --- | --- |
| `chatgpt_home_logged_out` | URL 为 `chatgpt.com/`，按钮含 `登录/免费注册` | 停止订阅探测，进入登录引导 |
| `chatgpt_home_logged_in` | URL 为 `chatgpt.com/`，有已登录 shell | 点击/导航到套餐入口 |
| `pricing_visible_no_payment` | plus=true 但 subscribe_payment=false | 降低轮询频率，等待用户/页面推进 |
| `subscribe_payment_visible` | subscribe_payment=true | 触发 session fetch / checkout |
| `cdp_not_ready` | CDP 不可用 | 停止轮询，提示启动无痕 |
| `chatgpt_target_not_found` | target 数为 0 | 停止轮询，尝试打开/恢复 ChatGPT |

### P1.4 前端实现

在 `web/app.js` 的 `runPlusSubscribeProbeOnce` 外层增加熔断：

```js
if (data.stage === "plus_subscribe_probe" && data.url === "https://chatgpt.com/" && !data.plus_plan_detected) {
  plusSubscribeNoProgressCount += 1;
}

if (plusSubscribeNoProgressCount >= 3) {
  stopPlusSubscribeWatcher("chatgpt_home_no_progress");
  transitionAutomationState("login_guiding", { reason: "chatgpt_home_no_progress" });
}
```

### P1.5 后端实现

在 `/api/pricing/plus-subscribe-probe` 返回体中新增：

```json
{
  "page_classification": "chatgpt_home_logged_out",
  "next_action": "login_guiding",
  "retryable": false,
  "poll_after_ms": 0
}
```

### P1.6 验收标准

- 首页状态下最多探测 3 次。
- `plus-subscribe-probe` 小时级调用量下降 80% 以上。
- 前端状态灯显示“需登录/需进入套餐页”，而不是“持续运行中”。

## P2：GoPay token 生命周期管理

### P2.1 目标

避免对已过期、已不存在或已限流的 GoPay token/reference/account_id 进行重复无效请求。

### P2.2 当前问题

GoPay 绑定失败集中在：

- `token not found`
- `token has expired`
- `429`

这类错误通常不应该立即重试同一 token。

### P2.3 策略

维护本地 token 状态表，存储在前端内存或后端轻量 runtime map：

```go
type gopayTokenState struct {
  AccountID string
  ReferenceID string
  State string // fresh, used, expired, not_found, rate_limited
  LastError string
  LastSeenAt time.Time
  RetryAfter time.Time
}
```

状态规则：

| 错误 | 状态 | 动作 |
| --- | --- | --- |
| 404 `token not found` | `not_found` | 废弃，不再重试 |
| 407 `token has expired` | `expired` | 废弃，要求重新生成支付页/绑定页 |
| 429 | `rate_limited` | 指数退避，不立即重试 |
| 2xx + reference_id | `fresh` | 允许进入 OTP/PIN |
| 完成绑定 | `used` | 不再用于新绑定 |

### P2.4 实现路径

- `handleGopayForceLink` 和 `handleGopayAutoLink` 输出 `token_state`、`retry_after_ms`、`next_action`。
- 前端记录 `gopayMidtransRetryNotBefore` 时，不只按按钮冷却，还按 token/account_id 冷却。
- 出现 `not_found/expired` 后，自动清理当前 GoPay reference，并提示“重新打开支付页/重新绑定”。

### P2.5 验收标准

- 同一 `account_id + reference_id` 不重复产生超过 1 次 `token not found`。
- `429` 后不在冷却期内再次请求同一 token。
- GoPay auto-link 失败率明显下降，失败日志转为更明确的 `manual_required`。

## P3：CDP 目标 readiness gate 与支付安全边界

### P3.1 目标

只有在正确页面 target 存在时才启动对应自动化，避免 CDP 空轮询与错误目标执行。

### P3.2 当前问题

`/api/gopay/cdp-otp` 失败中：

- `CDP not ready`：540
- `midtrans.com target missing`：393
- `GoPay OTP/PIN target missing`：278

### P3.3 readiness gate

在前端启动 `startGopayOTPAutoCapture` 前先调用轻量 target probe，或复用 `/api/checkout/resolve-target` 结果。

允许启动 GoPay CDP 自动化的条件：

```text
CDP ready
AND (
  current target host contains app.midtrans.com
  OR current target host contains gopayapi.com
  OR selected checkout target has submitted/redirected to Midtrans
)
```

不满足时：

```text
state = payment_target_waiting
poll_after_ms = 1500-3000
no cdp-otp call
```

### P3.4 支付安全边界

必须保持以下边界：

- Pay now 只允许一次 CDP trusted mouse click。
- 不重复 synthetic click。
- 不干扰支付页面原生跳转、重定向、结算触发。
- 不自动点击 OpenAI 最终人工确认类按钮。
- PIN 输入只在明确 PIN 页面或 PIN execution context 中执行。

### P3.5 第二 PIN 上下文策略

当前代码已加入 execution context 策略：

- 默认在选中 target 执行观察脚本。
- 如果父页面看不到 PIN 输入框，则枚举 `Runtime.executionContextCreated`。
- 优先选择 `pin-web-client.gopayapi.com/payment/validate-pin`。
- 返回 `pin_context_selected` 和 `execution_context_frame_url`。

后续验收应检查：

```text
pin_context_selected = true
execution_context_frame_url contains pin-web-client.gopayapi.com/payment/validate-pin
pin_stage = payment
pin_auto_filled = true
pin_auto_submitted = true
```

### P3.6 验收标准

- `/api/gopay/cdp-otp` 的 503 失败减少 50% 以上。
- Pay now 后如果 20 秒未跳转，只继续被动监听，不重复点击。
- 第二 PIN 页面出现时能够在日志中看到正确 context。

## P4：人工干预显式化与可观测性增强

### P4.1 目标

将隐性失败变成明确状态，使用户知道“系统在等什么、下一步该做什么、是否需要人工处理”。

### P4.2 人工干预点

| 节点 | 人工原因 | 自动化响应 |
| --- | --- | --- |
| 登录验证码 | 需要邮件/人工验证 | 显示 `login_code_waiting`，接码成功后填入 |
| GoPay OTP | OTP 保持手动或 LuckMail/短信外部输入 | 显示 `gopay_otp_waiting` |
| Pay now 后未跳转 | 支付页拦截/网络/余额/页面阻塞 | 停止重复点击，继续监听 PIN/失败态 |
| PIN 来源不明 | 无法判断人工/自动 | 使用 `pin_input_event_source` |
| token 过期 | 绑定资源失效 | 标记 `manual_required: regenerate_token` |

### P4.3 PIN 输入审计字段

已新增字段：

| 字段 | 说明 |
| --- | --- |
| `pin_input_event_source` | `automation_synthetic` / `user_trusted_event` / `existing_value_observed_no_event` |
| `pin_input_event_is_trusted` | 浏览器事件是否真实用户触发 |
| `pin_input_observed_value` | 当前观测 PIN 值，本地敏感审计开启时记录 |
| `pin_input_expected_value` | 系统准备自动输入的 PIN |
| `pin_input_matches_expected` | 观测值是否等于系统 PIN |
| `pin_input_events` | 最近 PIN 输入事件摘要 |

### P4.4 前端可视化策略

状态灯规则：

| 颜色 | 含义 |
| --- | --- |
| 绿色 | 当前步骤完成或正常监听 |
| 红色 | 当前步骤失败或需要立即处理 |
| 黄色/中性 | 等待外部页面、OTP、人工输入、冷却期 |

状态文案必须包含：

- 当前状态
- 阻塞原因
- 下一动作
- 是否可重试
- 冷却剩余时间

### P4.5 验收标准

- 用户无需读日志也能知道卡在哪一步。
- 日志能区分人工 PIN 与自动 PIN。
- 所有 terminal 状态都有明确 `next_action`。

## 7. 多角色评审

### 7.1 自动化架构师评审

结论：可行，且 P0 是必要前置。

理由：

- 当前项目的失败不是单点 selector 问题，而是流程控制缺少状态机。
- 大量轮询发生在错误页面或无 target 状态，状态机能直接减少噪声。
- P0 不要求重写全部 API，可以先在前端编排层实现。

风险：

- 如果 P0 一次性改动过大，可能影响已有按钮功能。

建议：

- 先保留现有按钮，新增“推荐流程运行器”作为总入口。
- 各按钮仍可手动调用，但必须同步状态机状态。

### 7.2 后端/CDP 工程师评审

结论：P2/P3 可行，需谨慎处理支付页动作边界。

依据：

- `main.go` 已具备 CDP target 查找、Runtime evaluate、frame context 逻辑。
- `handleGopayCDPOTP` 已能返回 target 与 PIN 字段。
- 新增 target readiness probe 可以复用现有 `getCDPTargets`。

风险：

- CDP 事件读取与同步命令共用 websocket，必须避免并发读同一个连接。
- 支付页面跳转期间 context 会销毁，需要把 transient error 当作可重试观察而非失败。

建议：

- readiness gate 使用独立短连接。
- 对 `execution context destroyed`、`target closed`、`navigated` 继续沿用 transient 分类。

### 7.3 前端工作流/UX 评审

结论：P1/P4 对效率提升最明显。

依据：

- 当前页面已存在状态灯、语音播报、工作流按钮。
- 但用户仍需要从日志判断卡点，说明状态提示不够动作化。

风险：

- 过多状态文案会挤占主面板空间。

建议：

- 每个状态只展示一行主状态 + 一行下一动作。
- 详细 JSON 放在折叠区。
- `manual_required` 状态必须给出按钮，例如“重新打开套餐页”“重新获取 Session”“重新生成支付页”。

### 7.4 QA/SRE 评审

结论：必须先建立验收指标，否则无法证明优化有效。

建议指标：

| 指标 | 当前基线 | 目标 |
| --- | ---: | ---: |
| `plus-subscribe-probe` 每小时调用量 | 最高 1702 | 降到 200 以下 |
| `cdp-otp` 503 比例 | 24.5% 总失败中大量 503 | 降低 50% |
| `auto-link token not found` 重复 | 239 | 同 token 不重复 |
| 登录超时 P95 | 57-119 秒 | 明确暂停，不盲等 |
| PIN 来源可判定率 | 旧日志无法判定 | 新日志 100% 可判定 |

测试方法：

- 单元测试：状态分类函数、token 状态转移、target readiness 判定。
- 集成测试：模拟 target 缺失、token expired、Pay now stalled、PIN iframe 出现。
- 日志回放：读取历史日志，验证新状态机能得出正确 next_action。

### 7.5 安全/合规评审

结论：本地敏感审计可接受，但必须保持开关和隔离。

依据：

- 项目是本地运行工具。
- `config.json` 当前 `audit_capture_sensitive=true`，允许记录 PIN 原始值用于排障。

风险：

- `log/` 包含敏感信息，不应提交或外传。
- 文档和代码应继续提醒 `log/` 是运行态数据。

建议：

- 默认通过 `.gitignore` 排除 `log/`。
- 敏感字段记录必须受 `audit_capture_sensitive` 控制。
- 导出诊断报告时提供脱敏模式。

### 7.6 运营/使用者评审

结论：方案能减少人工重复判断，但需要保持可手动接管。

建议：

- 自动流程失败后不要卡死，要显示“重试/跳过/手动完成/重新开始”。
- 对支付相关动作保持保守，宁可等待人工也不要重复点击。
- 成功后自动保存 GPT Plus 记录和凭证导出状态。

## 8. 实施步骤

### 阶段 1：低风险观测增强

1. 保留现有流程。
2. 给 `plus-subscribe-probe` 增加 `page_classification`、`next_action`、`poll_after_ms`。
3. 给 GoPay 绑定接口增加 `token_state`、`retry_after_ms`。
4. 前端只展示这些字段，不改变执行路径。

验收：

- 日志中每个失败都能给出 `next_action`。
- UI 能显示阻塞原因。

### 阶段 2：轮询熔断与 readiness gate

1. 首页探测连续 3 次无进展后停止 `plus-subscribe-probe`。
2. 启动 `cdp-otp` 前确认 Midtrans/GoPay target 存在。
3. `cdp_not_ready` 后停止当前轮询，转入 CDP 恢复状态。

验收：

- 11-14 点那类每小时千级探测不再出现。
- CDP target missing 类 503 显著减少。

### 阶段 3：状态机入口

1. 新增“推荐流程运行器”。
2. 将现有按钮动作封装为状态机 transition。
3. 每个状态写入统一 `trace_id`。
4. 支持暂停、恢复、取消。

验收：

- 一次运行能从无痕窗口推进到支付页或明确人工阻塞。
- 所有关键日志可按 `trace_id` 串联。

### 阶段 4：GoPay token 管理

1. 建立 token/reference 状态缓存。
2. `expired/not_found` 立即废弃。
3. `429` 指数退避。
4. UI 显示冷却和重新生成动作。

验收：

- 同一无效 token 不重复打接口。
- 429 期间不会继续暴力重试。

### 阶段 5：回放验证与回归测试

1. 编写历史日志回放脚本。
2. 对旧日志输出新状态机判定结果。
3. 增加 Go 单元测试和前端静态检查。

验收：

- 历史问题能被分类为正确 `next_action`。
- `go test ./...`、`node --check web/app.js` 通过。

## 9. 优先级汇总

| 优先级 | 工作项 | 影响 | 风险 | 建议顺序 |
| --- | --- | --- | --- | --- |
| P0 | 状态机 | 最高 | 中 | 先做框架，不一次迁移全部按钮 |
| P1 | Plus 探测熔断 | 高 | 低 | 最先落地，可快速降噪 |
| P2 | GoPay token 生命周期 | 高 | 中 | 需要后端/前端协同 |
| P3 | CDP readiness gate | 高 | 中 | 与支付安全边界绑定实施 |
| P4 | 人工干预显式化 | 中高 | 低 | 与 UI 状态灯同步推进 |

## 10. 最终可行性结论

P0-P4 方案与当前项目实际结构匹配，且可以渐进实施：

- 不需要重写后端单体架构。
- 不需要引入新数据库。
- 不需要改变支付页原生跳转机制。
- 前端可以先作为状态机编排层承接大部分改造。
- 后端主要补充分类字段、readiness gate、token 状态与日志字段。

总体可行性：高。

最推荐的落地顺序：

```text
P1 Plus 探测熔断
  -> P3 CDP readiness gate
  -> P4 状态提示与人工干预显式化
  -> P2 GoPay token 生命周期
  -> P0 完整状态机收口
```

其中 P0 是最终形态，但为了降低风险，应以“薄状态机 + 现有按钮兼容”的方式逐步收敛，而不是一次性替换全部流程。

## 11. 最新运行日志多角色验证报告

核对时间：2026-05-11 21:23（Asia/Shanghai）

验证参与角色：

- 自动化架构师：验证 P0-P4 状态机覆盖、实施顺序、terminal 收口和跨 watcher 协调。
- 后端/CDP 工程师：验证 CDP readiness gate、GoPay PIN/OTP、Pay now 安全边界、后端响应字段和审计字段。
- 前端工作流/UX 专家：验证状态灯、按钮流程、语音播报、轮询退避、Session 校验和用户可见下一动作。
- 运营/使用者代表：验证人工接管、失败提示、重复劳动和实际操作可理解性。

### 11.1 验证方法

本次验证不再按“每个日志文件只包含一个 JSON”的假设解析，而是按 JSONL/多 JSON 事件逐行解析 `log/*.log`。这是必要修正，因为多个 `.log` 文件实际包含多条审计 JSON；单文件 `ConvertFrom-Json` 会漏掉 `/api/gopay/auto-trigger-check`、`/api/gopay/smart-link`、`/api/voice/speak` 等事件，导致基线偏低。

验证范围：

| 范围 | 数值 |
| --- | ---: |
| 全量可解析事件 | 27,781 |
| 全量起始时间 | 2026-05-07 14:06 |
| 全量结束时间 | 2026-05-11 21:23 |
| 增量验证窗口 | 2026-05-11 14:26 至 21:23 |
| 增量事件数 | 6,430 |

增量窗口主要接口分布：

| 接口 | 调用量 | 验证含义 |
| --- | ---: | --- |
| `/api/pricing/plus-subscribe-probe` | 4,605 | P1 的主要验证对象，仍为最大噪声源 |
| `/api/checkout/resolve-target` | 508 | checkout target 解析仍高频运行 |
| `/api/voice/speak` | 481 | P4 语音播报可靠性需要纳入验证 |
| `/api/gopay/auto-trigger-check` | 263 | 真实流程存在 `auto_trigger_waiting/ready` 分支 |
| `/api/gopay/cdp-otp` | 157 | P3/P4 的 GoPay CDP 自动化验证对象 |
| `/api/session/fetch` | 21 | Session 获取频次较低，但需要强校验 accessToken |
| `/api/checkout/start` | 20 | checkout 生成链路样本 |

### 11.2 总体验证结论

P0-P4 方案方向正确，且最新成功支付链路证明 GoPay binding PIN 自动输入、Pay now 一次真实点击边界、支付完成记录等关键能力已经具备工程基础。但方案文档需要补充最新日志反映出的真实行为：当前最大问题已经从“ChatGPT 首页空转”扩展为“checkout 订阅确认页空转”，并且方案要求的 `page_classification`、`next_action`、`poll_after_ms`、`token_state` 等机器可读字段在最新日志中仍未落地。

最终验证结论：**有条件通过**。

通过条件：

1. 先实现 P0.0 运行生命周期与 terminal 统一收口。
2. P1 不只处理首页空转，还必须处理 checkout 订阅确认页空转。
3. P3 readiness gate 必须泛化到所有 CDP watcher，而不仅是 `/api/gopay/cdp-otp`。
4. P4 必须把“失败态”变成“下一动作”，否则用户仍需要读日志。
5. P2 token 生命周期需要成为后端 API 合同，而不是前端临时冷却变量。

### 11.3 最新成功链路对 P0 的验证

最新完整成功链路显示真实流程并非严格线性。脱敏后的阶段序列为：

```text
/api/session/fetch
  -> /api/checkout/start
  -> /api/checkout/payment-method-select
  -> /api/checkout/auto-fill
  -> /api/gopay/auto-trigger-check: auto_trigger_waiting
  -> /api/gopay/auto-trigger-check: auto_trigger_ready
  -> /api/gopay/midtrans-linking-fill: midtrans_linking_submitted
  -> /api/gopay/cdp-otp: page_observed
  -> /api/gopay/cdp-otp: pin_entry_binding
  -> /api/gopay/cdp-otp: gopay_complete
  -> /api/gptpls/record: gptpls_payment_success_recorded
```

日志证据：

| 观察项 | 最新日志表现 | 对方案的影响 |
| --- | --- | --- |
| GoPay binding PIN | `pin_stage=binding`、`pin_auto_filled=true`、`pin_auto_submitted=true`、`pin_input_event_source=automation_synthetic` | 证明第一处 PIN 自动输入已可验证 |
| payment PIN | 增量窗口 `pin_stage=payment && pin_auto_submitted=true` 为 0 | P3 验收不能假设第二 PIN 必然出现，需作为可选分支 |
| 支付完成 | `stage=gopay_complete`、`payment_completed=true`、`payment_complete_reason=url_or_text_success` | terminal 状态可由 URL/文本成功信号触发 |
| 成功记录 | `/api/gptpls/record` 成功 | P0 terminal 后应进入记录/导出或关闭窗口状态 |

文档修订要求：

- P0 状态图应新增 `auto_trigger_waiting`、`auto_trigger_ready`、`midtrans_linking_submitted`、`checkout_payment_method_selected`、`gopay_complete`。
- `gopay_pin_payment` 不能作为必经线性步骤，应改为 Pay now 后的可选分支：`payment_pin_target_waiting -> gopay_pin_payment -> payment_completed`。
- `payment_completed` 必须触发 run-level cleanup，停止 Plus probe、checkout watcher、GoPay CDP poll 和语音队列里的过期提示。

### 11.4 P1 Plus 探测验证结论

P1 的问题判断被最新日志强烈验证，但原方案覆盖面不足。

增量窗口 `/api/pricing/plus-subscribe-probe` 统计：

| 指标 | 数值 |
| --- | ---: |
| 调用量 | 4,605 |
| 业务失败 | 4,605 |
| `server_throttled` | 0 |
| `page_classification` | 0 |
| `next_action` | 0 |
| `poll_after_ms` | 0 |
| `plus=true, subscribe=false, manual_required=true` | 2,923 |
| `chatgpt_target_not_found` | 796 |
| `cdp_not_ready` | 343 |

关键冲突：

- 代码中已存在 `server_throttled` 注入逻辑，但最新日志没有该字段，说明当前运行服务可能未加载最新代码，或后端缓存路径未被命中。该问题不能只依赖源码字符串测试，必须补 handler 级测试和运行时健康标识。
- 日志主噪声已变成 `chatgpt.com/checkout/...` 上的订阅确认页：页面有 Plus 套餐、月付/年付、订阅、条款等文本，但没有 `subscribe_payment_detected`，因此一直 `safe_trigger_fetch_session=false`。
- 这类状态不应归类为失败轮询，而应归类为 `checkout_subscription_confirmation_required` 或 `checkout_created_not_submitted`，并转入人工确认状态。

P1 修订建议：

| 新分类 | 判断条件 | 动作 |
| --- | --- | --- |
| `checkout_subscription_confirmation_required` | URL 含 `/checkout/`，`plus_plan_detected=true`，`subscribe_payment_detected=false`，页面含订阅/条款/计费选项 | 停止高频探测，提示用户审阅并手动点击订阅 |
| `checkout_created_not_submitted` | checkout URL 存在但未跳转到 Midtrans/支付页 | 退避到 30-60 秒，显示“等待用户确认订阅或页面跳转” |
| `terminal_after_payment` | 当前 run 已 `gopay_complete/payment_completed` | 停止所有订阅探测 |
| `cdp_recovering` | CDP not ready | 不再打业务探测，进入无痕诊断/恢复 |

P1 新验收标准：

- checkout 确认页同一 trace 连续 3 次无进展后停止高频探测。
- `/api/pricing/plus-subscribe-probe` 返回 `page_classification`、`next_action`、`retryable`、`terminal`、`poll_after_ms`。
- 最新日志中 `server_throttled` 或 `poll_after_ms` 必须可见，不能只在源码中存在。

### 11.5 P2 GoPay token 生命周期验证结论

P2 尚未落地为 API 合同。

增量窗口 GoPay token/绑定相关接口：

| 指标 | 数值 |
| --- | ---: |
| `/api/gopay/auto-link`、`force-link`、`smart-link`、`midtrans-linking-fill` 合计 | 57 |
| 失败 | 37 |
| `token_state` 字段 | 0 |
| `next_action` 字段 | 0 |
| `retryable` 字段 | 0 |

日志仍能看到同一失效资源重复出现 `token not found`、`token has expired`、`429`。这与 P2 中“同一 token/reference 失败后不重复尝试”的目标冲突。

P2 必须调整为后端优先：

- 404 -> `token_state=not_found`、`terminal=true`、`next_action=regenerate_checkout`
- 407 -> `token_state=expired`、`terminal=true`、`next_action=regenerate_checkout`
- 429 -> `token_state=rate_limited`、`retryable=true`、`retry_after_ms`、`next_action=wait_cooldown`
- 2xx -> `token_state=fresh|used`

前端只负责消费这些字段并禁用同一 `account_id/reference_id/checkout_key` 的重复请求。

### 11.6 P3 CDP readiness gate 与支付安全边界验证结论

P3 部分有效，但范围需要扩大。

GoPay CDP 增量窗口：

| 指标 | 数值 |
| --- | ---: |
| `/api/gopay/cdp-otp` 调用量 | 157 |
| 失败/503 | 12 |
| binding PIN 自动提交 | 5 |
| payment PIN 自动提交 | 0 |
| `gopay_complete/payment_completed` | 4 |
| `pay_now_block_reason=trusted_click_already_sent` | 58 |

验证通过项：

- Pay now 一次真实点击边界基本成立。日志出现 `trusted_click_already_sent`，未观察到重复点击扩大。
- binding PIN 自动输入和提交有明确证据。
- 支付完成可被 `gopay_complete` 和 `payment_completed=true` 识别。

需要修订项：

- readiness gate 不能只包 `/api/gopay/cdp-otp`。Plus probe 同样产生大量 `cdp_not_ready` 和 `chatgpt_target_not_found`，应共用 `cdp_recovering/target_waiting` gate。
- Pay now 后如果长时间停留 `pay_now_post_click_wait`，应提升 `poll_after_ms`，并输出 `next_action=wait_payment_pin_or_manual_check`。
- 第二 PIN 未在最新日志中成功出现，验收应改为“若 payment PIN target 出现，则必须能记录 context 并尝试输入”，不能要求每次支付都出现 payment PIN。

### 11.7 P4 可观测性与人工接管验证结论

P4 的字段方向正确，但用户可见闭环不足。

主要问题：

1. 最新失败日志大量缺少 `next_action`。增量窗口中 Plus probe、GoPay token 失败、部分 voice failure 都不能直接告诉用户下一步。
2. 状态灯目前更像按钮上次动作结果，而不是主流程状态机。`web/app.js` 里主要通过 `setWorkflowButtonState()` 给按钮设置 `data-flow-state`，尚未形成文档中描述的 `automationRun`。
3. 运行中红绿交替的灯效可能被误读为错误。P4 应要求红色只表示失败或需要立即处理；运行中建议使用蓝色/中性脉冲。
4. 语音播报有成功记录，但也出现 `local_voice_speak_failed` 和超时。关键人工动作应有浏览器语音兜底，并避免过长排队。

P4 最小合同建议：

```json
{
  "stage": "checkout_subscription_confirmation_required",
  "human_message": "已到订阅确认页，需要你审阅条款并手动点击订阅。",
  "next_action": "review_and_click_subscribe",
  "retryable": false,
  "terminal": false,
  "poll_after_ms": 60000,
  "primary_button": "我已点击订阅，继续监听",
  "secondary_button": "重新打开支付页"
}
```

### 11.8 多角色交叉结论

| 角色 | 结论 | 关键证据 | 必须补齐 |
| --- | --- | --- | --- |
| 自动化架构师 | 有条件通过 | 最新成功链路包含 P0 未列出的 `auto_trigger_waiting/ready`、`midtrans_linking_submitted`、`gopay_complete`；完成后 Plus probe 仍继续 | P0.0 terminal cleanup、状态图补分支 |
| 后端/CDP 工程师 | 有条件通过 | Pay now 一次点击和 binding PIN 自动化成立；Plus probe 分类/限流、token_state 未在日志体现 | 后端响应合同、handler 级测试、统一 CDP gate |
| 前端工作流/UX 专家 | 有条件通过 | 状态灯是按钮状态；获取 Session 实际会继续生成并打开支付页；失败态缺下一动作 | 主流程状态条、按钮命名/顺序修正、消费 `next_action` |
| 运营/使用者代表 | 有条件通过 | 用户仍需读日志判断订阅确认、token 失效、Pay now 等待、PIN/OTP 手动点 | 手动接管按钮、动作化文案、失败态播报 |

### 11.9 修订后的实施顺序

原建议顺序 `P1 -> P3 -> P4 -> P2 -> P0` 仍适合渐进落地，但最新日志显示需要先补一个更小的 P0.0，否则完成后 watcher 不收口。

修订顺序：

```text
P0.0 运行生命周期与 terminal cleanup
  -> P1 Plus/checkout 探测分类、熔断、退避
  -> P3 通用 CDP readiness gate 与 Pay now 后低频监听
  -> P4 next_action、human_message、手动接管按钮和语音提示
  -> P2 GoPay token/reference 生命周期状态化
  -> P0 完整状态机迁移
```

### 11.10 最终验证结论

方案总体可行，但当前文档需要根据最新日志补充以下底线：

- **不是所有成功支付都会出现第二次 payment PIN**；payment PIN 应为条件分支。
- **checkout 订阅确认页是当前最大空转来源**；P1 必须覆盖它，而不只覆盖首页。
- **terminal 状态必须停止所有 watcher**；支付已完成后继续 Plus probe 是当前最明确的状态机缺陷。
- **API 响应必须提供下一动作**；没有 `next_action`，前端无法降低人工判断成本。
- **运行服务版本必须可验证**；源码已有字段但日志没有字段时，应优先检查服务是否重启、页面是否刷新、handler 级测试是否覆盖。

本轮验证给出的最终状态：**方案可继续执行，但须按 P0.0/P1/P3/P4/P2 的顺序补齐验证条件后，才能宣称 P0-P4 已完整落地。**
