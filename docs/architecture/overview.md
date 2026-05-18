# 架构总览

最后核对日期：2026-05-17

## 代码架构解构

当前项目是一个本地 Checkout Workbench，核心目标是把 ChatGPT/OpenAI checkout、Stripe/Midtrans/GoPay 页面状态、Chrome 无痕窗口、LuckMail 邮箱接码和 browser-use 自动化能力整合到一个本地控制台中。

项目采用**单体 Go 服务 + 嵌入式前端 + 外部自动化服务**的架构模式：

- **Go 本地 HTTP 服务**（`main.go`）：单一入口，集中管理所有路由、业务逻辑、CDP 交互、审计日志
- **嵌入式 Web 前端**（`web/`）：通过 Go `embed.FS` 编译进二进制，三栏工作台 UI
- **browser-use Node/Playwright 服务**：独立进程，通过 HTTP 与前端通信
- **Chrome 扩展**（`extension/`）：Manifest V3，辅助凭证提取
- **LuckMail Go SDK**（`LuckMailSdk-Go/`）：本地 SDK 依赖，封装 LuckMail API

```text
用户浏览器
  │
  │ 访问 http://127.0.0.1:18473/
  ▼
Go 本地 HTTP 服务 main.go
  ├─ 嵌入 web/ 静态页面（index.html, app.js, styles.css 等）
  ├─ /api/health                          健康检查
  ├─ /api/voice/speak                     本机语音播报
  ├─ /api/sandbox/payment-authorization/assess  支付授权 sandbox 评估
  ├─ /api/checkout*                       生成支付链接、解析 checkout、自动填地址、支付方式选择、授权订阅点击
  ├─ /api/incognito/*                     无痕窗口打开/关闭/诊断
  ├─ /api/login/*                         登录自动化（点击、邮箱填写、验证码填入）
  ├─ /api/session/fetch                   通过 Chrome CDP 获取 ChatGPT Session JSON
  ├─ /api/gopay/*                         GoPay 绑定（force/auto/smart/full-link）、OTP/PIN CDP 操作、
  │                                        Midtrans 页面填充、自动触发检查、Snap 探测、页面监控
  ├─ /api/pricing/*                       定价页监控、Plus 订阅探测
  ├─ /api/luckmail/*                      LuckMail 接码（创建等待、token 验证码、邮件列表、已购列表、配置管理）
  ├─ /api/gptpls/record                   GPT Plus 记录
  ├─ /api/perf/client                     客户端性能日志
  └─ audit 日志中间件记录所有 /api/ 操作

browser-use Node/Playwright 服务（独立进程，默认 127.0.0.1:38765）
  ├─ GET /health
  └─ POST /run
       ├─ 启动 Playwright 浏览器
       ├─ 导航、等待条件、扫描表单
       ├─ 填写/提交表单
       └─ 提取页面全文、结构化数据和属性

Chrome 扩展 extension/
  ├─ background.js    Service Worker：无痕窗口管理、登录检测、凭证提取
  ├─ content.js       页面注入：浮动按钮 UI、与 Service Worker 通信
  └─ popup.html/js    配置面板：目标 URL、检测规则、手动触发

LuckMail SDK LuckMailSdk-Go/
  ├─ luckmail/client.go      主客户端、配置选项
  ├─ luckmail/http_client.go HTTP 客户端、鉴权、请求封装
  ├─ luckmail/user.go        用户端 API（余额、邮箱、项目、订单、接码）
  ├─ luckmail/supplier.go    供应商端 API（看板、邮箱、申述）
  ├─ luckmail/models.go      数据模型定义
  └─ luckmail/errors.go      错误类型定义
```

## 模块组织结构

| 路径 | 职责 | 事实来源 |
| --- | --- | --- |
| `main.go` | Go 服务入口、HTTP 路由、Checkout/GoPay/CDP/审计逻辑、LuckMail 集成 | [main.go](file:///f:/chatadd/main.go) |
| `main_test.go` | Go 单元测试、HTTP 集成辅助、基准测试 | [main_test.go](file:///f:/chatadd/main_test.go) |
| `go.mod` | Go 模块定义、依赖声明 | [go.mod](file:///f:/chatadd/go.mod) |
| `go.sum` | Go 依赖校验和 | [go.sum](file:///f:/chatadd/go.sum) |
| `config.json` | 应用配置（checkout endpoint、LuckMail API key 等） | [config.json](file:///f:/chatadd/config.json) |
| `web/index.html` | 页面 DOM 结构、三栏工作台布局、browser-use 内联逻辑 | [index.html](file:///f:/chatadd/web/index.html) |
| `web/app.js` | 前端主流程、API 调用、状态机、GoPay 自动触发、LuckMail 接码 UI | [app.js](file:///f:/chatadd/web/app.js) |
| `web/styles.css` | 三栏工作台、响应式布局和组件样式 | [styles.css](file:///f:/chatadd/web/styles.css) |
| `web/mail-code.html` | LuckMail 邮箱验证码独立页面 | [mail-code.html](file:///f:/chatadd/web/mail-code.html) |
| `web/readme.html` | 项目说明独立页面 | [readme.html](file:///f:/chatadd/web/readme.html) |
| `web/wechat-group-qr-placeholder.svg` | 微信群二维码占位图 | [wechat-group-qr-placeholder.svg](file:///f:/chatadd/web/wechat-group-qr-placeholder.svg) |
| `LuckMailSdk-Go/luckmail/` | LuckMail Go SDK（client、user、supplier、models、errors） | [client.go](file:///f:/chatadd/LuckMailSdk-Go/luckmail/client.go) |
| `.codex/runtime/browser-use-service/src/server.js` | browser-use HTTP 服务 | [server.js](file:///f:/chatadd/.codex/runtime/browser-use-service/src/server.js) |
| `.codex/runtime/browser-use-service/src/browser-use.js` | Playwright 自动化执行器 | [browser-use.js](file:///f:/chatadd/.codex/runtime/browser-use-service/src/browser-use.js) |
| `.codex/runtime/browser-use-service/src/extractor.js` | 页面内容、链接、表单、结构化数据提取 | [extractor.js](file:///f:/chatadd/.codex/runtime/browser-use-service/src/extractor.js) |
| `.codex/runtime/browser-use-service/src/cli.js` | browser-use CLI 输入解析 | [cli.js](file:///f:/chatadd/.codex/runtime/browser-use-service/src/cli.js) |
| `.codex/runtime/browser-use-service/src/logger.js` | browser-use 日志记录模块 | [logger.js](file:///f:/chatadd/.codex/runtime/browser-use-service/src/logger.js) |
| `.codex/runtime/browser-use-service/src/errors.js` | browser-use 自定义错误类型 | [errors.js](file:///f:/chatadd/.codex/runtime/browser-use-service/src/errors.js) |
| `extension/` | Manifest V3 Chrome 凭证提取器（background/content/popup） | [manifest.json](file:///f:/chatadd/extension/manifest.json) |
| `start-browser-use.cmd` | Windows 启动 browser-use 服务 | [start-browser-use.cmd](file:///f:/chatadd/start-browser-use.cmd) |
| `start-browser-use.ps1` | Windows PowerShell 启动 browser-use 服务 | [start-browser-use.ps1](file:///f:/chatadd/start-browser-use.ps1) |
| `start-all-services.cmd` | Windows 同时启动 Go 服务和 browser-use 服务 | [start-all-services.cmd](file:///f:/chatadd/start-all-services.cmd) |

## 组件依赖关系图谱

```text
web/app.js
  ├─ 调用 /api/health
  ├─ 调用 /api/voice/speak
  ├─ 调用 /api/checkout 与 /api/checkout/start
  ├─ 调用 /api/incognito/open
  ├─ 调用 /api/incognito/close
  ├─ 调用 /api/incognito/diagnostics
  ├─ 调用 /api/login/click
  ├─ 调用 /api/login/email-fill
  ├─ 调用 /api/login/code-fill
  ├─ 调用 /api/session/fetch
  ├─ 调用 /api/checkout/resolve-target
  ├─ 调用 /api/checkout/auto-fill
  ├─ 调用 /api/checkout/payment-method-select
  ├─ 调用 /api/checkout/authorized-subscribe-click
  ├─ 调用 /api/gopay/cdp-otp
  ├─ 调用 /api/gopay/auto-trigger-check
  ├─ 调用 /api/gopay/midtrans-linking-fill
  ├─ 调用 /api/gopay/full-link
  ├─ 调用 /api/gopay/monitor
  ├─ 调用 /api/gopay/snap-probe
  ├─ 调用 /api/pricing/monitor
  ├─ 调用 /api/pricing/plus-subscribe-probe
  ├─ 调用 /api/luckmail/create-and-wait
  ├─ 调用 /api/luckmail/token-code
  ├─ 调用 /api/luckmail/token-mails
  ├─ 调用 /api/luckmail/purchases
  ├─ 调用 /api/luckmail/config
  ├─ 调用 /api/luckmail/config/test
  ├─ 调用 /api/gptpls/record
  ├─ 调用 /api/perf/client
  └─ 调用 /api/sandbox/payment-authorization/assess

main.go
  ├─ 读取 config.json 或 APP_CONFIG 环境变量
  ├─ 调用外部 checkout endpoint
  ├─ 通过 cmd 启动 Chrome 无痕窗口
  ├─ 通过 CDP WebSocket 读取/操作 Chrome 页面
  ├─ 调用 Midtrans / GoPay / Snap API
  ├─ 调用 LuckMail API（通过 LuckMailSdk-Go）
  ├─ 调用 Stripe API（initStripePaymentPage、updateStripePaymentPage 等）
  └─ 写入 log/ 审计日志

web/index.html browser-use 面板
  └─ 调用 http://127.0.0.1:38765/health 与 /run

browser-use-service
  └─ 依赖 Playwright chromium/firefox/webkit

LuckMailSdk-Go
  └─ 依赖 Go 标准库 net/http
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

### GoPay 自动触发快速路径

支付页面点击“订阅”后，前端的 checkout 提交监听器会轮询 `/api/checkout/resolve-target`，一旦检测到当前 checkout 已提交并出现 Midtrans redirection/linking 页面，就调用 `/api/gopay/auto-trigger-check` 做条件判定；条件满足时立即调用 `/api/gopay/midtrans-linking-fill`。

这条路径响应很快，主要来自以下设计：

- 前端在自动填地址完成后就启动提交监听器，不再等用户手动点击工具里的“GoPay 一键绑定”。
- 监听器只在检测到当前 checkout 已提交、且 Midtrans redirection 页面出现后触发，避免提前调用后端 full-link 管道。
- 后端通过 Chrome CDP 直接连接已经打开的 Midtrans 页面，在页面上下文内选择国家码、填写手机号并点击 `Link and pay`，省掉新建浏览器会话和重新解析 checkout 的成本。
- 自动触发使用 checkout key 做一次性去重，同一个 checkout 只触发一次，避免重复点击造成页面状态抖动。
- 点击后采用状态驱动等待：如果进入 OTP/PIN/GoPay 下一步就立即返回；如果出现 `technical error`，会记录并进行有限恢复，不再把“已点击”误判为成功。
- 点击窗口内会临时安装 fetch/XHR 诊断钩子，记录 Midtrans/GoPay 相关请求的状态码、耗时、header 摘要和响应诊断字段，便于后续从 `log/` 定位 4xx、5xx、风控或会话异常。

因此，当前“Link and pay”自动触发的体感速度主要来自前端提前布置监听器和后端 CDP 原地注入的组合优化，而不是完整 GoPay full-link 后端流程变快。

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

### 登录自动化流程

```text
用户点击登录按钮
  → POST /api/login/click
  → Go 后端通过 CDP 在 chatgpt.com 页面查找并点击登录按钮
  → 等待登录页面加载
  → 用户输入邮箱
  → POST /api/login/email-fill { email }
  → Go 后端通过 CDP 在登录页填写邮箱并提交
  → 等待验证码页面加载
  → 用户通过 LuckMail 接码获取验证码
  → POST /api/login/code-fill { code }
  → Go 后端通过 CDP 在验证码页面填入验证码
  → 完成登录
```

### LuckMail 邮箱接码流程

```text
用户配置 LuckMail API Key
  → POST /api/luckmail/config { api_key }
  → 后端保存配置到 config.json
  → POST /api/luckmail/config/test 测试 API Key 有效性
  → 用户创建接码任务
  → POST /api/luckmail/create-and-wait { project_id, supplier_id, timeout }
  → 后端调用 LuckMail SDK 创建邮箱并轮询等待验证码
  → 返回邮箱地址和验证码
  → 用户查询已购邮箱
  → POST /api/luckmail/purchases 获取已购邮箱列表
  → POST /api/luckmail/token-code { email } 获取指定邮箱验证码
  → POST /api/luckmail/token-mails { email } 获取指定邮箱邮件列表
```

## 功能特性清单

- 本地 Go 服务与嵌入式前端（单文件二进制部署）。
- Checkout 支付链接生成（支持 trial/paid 模式、代理配置）。
- ChatGPT Session JSON 自动提取（CDP 监听 /api/auth/session）。
- 系统 Chrome 无痕窗口启动、复用、关闭和诊断。
- 登录自动化：登录按钮点击、邮箱填写、验证码自动填入。
- checkout 页面目标解析和自动填地址目标锁定。
- checkout 支付方式选择（Stripe 支付方式创建与确认）。
- 本地授权订阅点击（authorized subscribe click）。
- GoPay 强制绑定、自动绑定、智能绑定、全流程绑定。
- Midtrans redirection 页面填充（国家码、手机号、Link and pay）。
- GoPay OTP 页面 CDP 识别与自动填入。
- GoPay PIN 页面 CDP 识别与自动填入。
- GoPay 自动触发检查（前端监听器 + 后端条件判定）。
- GoPay/定价页面 CDP 监控（页面截图、网络请求、控制台事件）。
- Snap 探测（GoPay Snap API 状态查询）。
- Plus 订阅探测（ChatGPT Plus 订阅页状态检测）。
- LuckMail 邮箱接码（创建邮箱、等待验证码、token 验证码查询）。
- LuckMail 已购邮箱管理（邮件列表、已购列表、配置管理）。
- GPT Plus 记录管理。
- 本机语音播报（通过 PowerShell 语音合成）。
- 支付授权 sandbox 评估（防篡改安全检测）。
- 客户端性能日志收集。
- API 审计日志与敏感字段脱敏/可选完整捕获。
- browser-use Node/Playwright 服务和 CLI。
- Chrome 凭证提取扩展（Manifest V3）。
- Windows 一键启动脚本（Go 服务 + browser-use 服务）。

## 运行时约束

- Go 服务固定监听 `127.0.0.1:18473`。
- Chrome CDP 默认使用端口 `9223`。
- browser-use 服务默认监听 `127.0.0.1:38765`，可通过 `BROWSER_USE_PORT` 覆盖。
- browser-use 服务默认允许跨域 `GET,POST,OPTIONS`，允许 `Content-Type` 与 `X-Account-Email` 请求头。
- API 错误统一倾向返回 JSON；通用错误结构为 `{ "error": string, "status": number }`。
