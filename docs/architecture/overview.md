# 架构总览

最后核对日期：2026-05-07

## 代码架构解构

当前项目是一个本地 Checkout Workbench，核心目标是把 ChatGPT/OpenAI checkout、Stripe/Midtrans/GoPay 页面状态、Chrome 无痕窗口和 browser-use 自动化能力整合到一个本地控制台中。

```text
用户浏览器
  │
  │ 访问 http://127.0.0.1:18473/
  ▼
Go 本地 HTTP 服务 main.go
  ├─ 嵌入 web/ 静态页面
  ├─ /api/checkout* 生成支付链接、解析 checkout 页面、自动填地址
  ├─ /api/session/fetch 通过 Chrome CDP 获取 ChatGPT Session JSON
  ├─ /api/incognito/open 启动或复用系统 Chrome 无痕窗口
  ├─ /api/gopay/* GoPay 绑定、OTP、PIN、Midtrans 页面填充、全流程支付辅助
  ├─ /api/*/monitor 通过 CDP 采集页面、网络和控制台事件
  └─ audit 日志中间件记录 API 操作

browser-use Node/Playwright 服务
  ├─ GET /health
  └─ POST /run
       ├─ 启动 Playwright 浏览器
       ├─ 导航、等待条件、扫描表单
       ├─ 填写/提交表单
       └─ 提取页面全文、结构化数据和属性

Chrome 扩展 extension/
  └─ 辅助提取已登录站点 Cookies、LocalStorage、SessionStorage
```

## 模块组织结构

| 路径 | 职责 | 事实来源 |
| --- | --- | --- |
| `main.go` | Go 服务入口、HTTP 路由、Checkout/GoPay/CDP/审计逻辑 | [main.go](file:///f:/chatadd/main.go) |
| `main_test.go` | Go 单元测试、HTTP 集成辅助、基准测试 | [main_test.go](file:///f:/chatadd/main_test.go) |
| `web/index.html` | 页面 DOM 结构、部分 browser-use 内联逻辑 | [index.html](file:///f:/chatadd/web/index.html) |
| `web/app.js` | 前端主流程、API 调用、状态渲染、GoPay 自动触发 | [app.js](file:///f:/chatadd/web/app.js) |
| `web/styles.css` | 三栏工作台、响应式布局和组件样式 | [styles.css](file:///f:/chatadd/web/styles.css) |
| `.codex/runtime/browser-use-service/src/server.js` | browser-use HTTP 服务 | [server.js](file:///f:/chatadd/.codex/runtime/browser-use-service/src/server.js) |
| `.codex/runtime/browser-use-service/src/browser-use.js` | Playwright 自动化执行器 | [browser-use.js](file:///f:/chatadd/.codex/runtime/browser-use-service/src/browser-use.js) |
| `.codex/runtime/browser-use-service/src/extractor.js` | 页面内容、链接、表单、结构化数据提取 | [extractor.js](file:///f:/chatadd/.codex/runtime/browser-use-service/src/extractor.js) |
| `.codex/runtime/browser-use-service/src/cli.js` | browser-use CLI 输入解析 | [cli.js](file:///f:/chatadd/.codex/runtime/browser-use-service/src/cli.js) |
| `extension/` | Manifest V3 Chrome 凭证提取器 | [manifest.json](file:///f:/chatadd/extension/manifest.json) |
| `start-browser-use.cmd` | Windows 启动 browser-use 服务 | [start-browser-use.cmd](file:///f:/chatadd/start-browser-use.cmd) |
| `start-all-services.cmd` | Windows 同时启动 Go 服务和 browser-use 服务 | [start-all-services.cmd](file:///f:/chatadd/start-all-services.cmd) |

## 组件依赖关系图谱

```text
web/app.js
  ├─ 调用 /api/health
  ├─ 调用 /api/checkout 与 /api/checkout/start
  ├─ 调用 /api/incognito/open
  ├─ 调用 /api/session/fetch
  ├─ 调用 /api/checkout/resolve-target
  ├─ 调用 /api/checkout/auto-fill
  ├─ 调用 /api/gopay/cdp-otp
  ├─ 调用 /api/gopay/auto-trigger-check
  ├─ 调用 /api/gopay/midtrans-linking-fill
  ├─ 调用 /api/gopay/full-link
  └─ 调用 /api/gopay/monitor

main.go
  ├─ 读取 config.json 或 APP_CONFIG
  ├─ 调用外部 checkout endpoint
  ├─ 通过 cmd 启动 Chrome 无痕窗口
  ├─ 通过 CDP WebSocket 读取/操作 Chrome 页面
  ├─ 调用 Midtrans / GoPay mock 或目标接口
  └─ 写入 log/ 审计日志

web/index.html browser-use 面板
  └─ 调用 http://127.0.0.1:38765/health 与 /run

browser-use-service
  └─ 依赖 Playwright chromium/firefox/webkit
```

## 核心业务流程

### 支付链接生成流程

```text
用户粘贴 access token 或 Session JSON
  → 前端 extractAccessToken 标准化 token
  → POST /api/checkout 或 /api/checkout/start
  → Go 后端校验 token、plan_name、billing_details、checkout_ui_mode、proxy
  → 请求 checkout_endpoint
  → initStripePaymentPage
  → 校验 0-IDR trial total
  → 生成 pay.openai.com / Stripe checkout 长链接
  → 前端展示 checkout_url、复制和无痕打开按钮
```

### Session 获取流程

```text
用户点击获取 Session JSON
  → 前端确保 Chrome 无痕窗口已打开
  → POST /api/session/fetch { stream: true }
  → Go 后端通过 CDP 连接 chatgpt.com 页面
  → 监听 /api/auth/session 相关数据
  → 以 NDJSON 返回 started/event/data/done 或 error
  → 前端回填 Session JSON 到 token 输入框
```

### 自动填地址流程

```text
用户先通过工具打开 checkout 链接
  → 前端记录 latestOpenedCheckoutURL
  → POST /api/checkout/auto-fill { expected_url }
  → 后端通过 CDP 找到 checkout 页面或 frame
  → 比对检测 URL 与 expected_url
  → 只在匹配时生成随机美国地址并填入表单
  → 前端启动提交监听器，等待后续 GoPay 自动触发
```

### GoPay 全流程辅助

```text
用户输入手机号、区号、OTP 通道、可选 OTP/PIN
  → POST /api/gopay/full-link
  → 后端从 checkout/Stripe/Snap/Midtrans 流程解析 account_id 或 transaction
  → 创建或复用 GoPay linking
  → reference 校验
  → user consent 请求 OTP
  → OTP 候选验证
  → PIN token 与 PIN 校验
  → Midtrans charge/payment validate/confirm/process/status
  → 返回 stage、stages、summary、payment_voucher 等诊断信息
```

### browser-use 自动化流程

```text
用户填写 URL、表单 JSON、等待条件
  → 前端调用 browser-use /health
  → POST http://127.0.0.1:38765/run
  → normalizeTaskInput 校验 url 并设置默认浏览器/超时/重试
  → Playwright 启动浏览器与上下文
  → 页面导航、等待 selector/text/urlIncludes
  → 扫描表单、定位字段、填写、可选提交
  → safeExtractPageData 提取页面数据
  → finally 中关闭 page/context/browser
```

## 功能特性清单

- 本地 Go 服务与嵌入式前端。
- Checkout 支付链接生成。
- ChatGPT Session JSON 自动提取。
- 系统 Chrome 无痕窗口启动和复用。
- checkout 页面目标解析和自动填地址目标锁定。
- GoPay 强制绑定、自动绑定、智能绑定、全流程绑定。
- Midtrans redirection 页面填充。
- GoPay OTP 页面 CDP 识别与自动填入。
- GoPay/定价页面 CDP 监控。
- API 审计日志与敏感字段脱敏/可选完整捕获。
- browser-use Node/Playwright 服务和 CLI。
- Chrome 凭证提取扩展。
- Windows 一键启动脚本。

## 运行时约束

- Go 服务固定监听 `127.0.0.1:18473`。
- Chrome CDP 默认使用端口 `9223`。
- browser-use 服务默认监听 `127.0.0.1:38765`，可通过 `BROWSER_USE_PORT` 覆盖。
- browser-use 服务默认允许跨域 `GET,POST,OPTIONS`，允许 `Content-Type` 与 `X-Account-Email` 请求头。
- API 错误统一倾向返回 JSON；通用错误结构为 `{ "error": string, "status": number }`。
