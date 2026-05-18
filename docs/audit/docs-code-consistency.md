# 文档与代码一致性审计报告

最后核对日期：2026-05-17

## 审计范围

本次审计覆盖：

- Go 后端入口、路由、请求结构、返回结构和配置项：[main.go](file:///f:/chatadd/main.go)
- Go 测试与当前验证方式：[main_test.go](file:///f:/chatadd/main_test.go)
- LuckMail Go SDK：[LuckMailSdk-Go/luckmail/](file:///f:/chatadd/LuckMailSdk-Go/luckmail/)
- 前端页面结构与运行流程：[index.html](file:///f:/chatadd/web/index.html)、[app.js](file:///f:/chatadd/web/app.js)、[styles.css](file:///f:/chatadd/web/styles.css)
- 前端辅助页面：[mail-code.html](file:///f:/chatadd/web/mail-code.html)、[readme.html](file:///f:/chatadd/web/readme.html)
- browser-use Node/Playwright 服务：[server.js](file:///f:/chatadd/.codex/runtime/browser-use-service/src/server.js)、[browser-use.js](file:///f:/chatadd/.codex/runtime/browser-use-service/src/browser-use.js)、[cli.js](file:///f:/chatadd/.codex/runtime/browser-use-service/src/cli.js)
- Chrome 扩展：[manifest.json](file:///f:/chatadd/extension/manifest.json)、[background.js](file:///f:/chatadd/extension/background.js)、[content.js](file:///f:/chatadd/extension/content.js)、[popup.js](file:///f:/chatadd/extension/popup.js)
- Windows 启动脚本：`start-browser-use.cmd`、`start-all-services.cmd`
- 文档目录：`docs/` 及其子目录
- 旧入口文档：根目录 `README.md` 与 `web/readme.html`

## 当前代码事实摘要

### 架构事实

- 项目是本地 Checkout Workbench，采用**单体 Go 服务 + 嵌入式前端 + 外部自动化服务**架构模式。
- Go 服务使用 `embed.FS` 嵌入 `web/` 静态文件，并固定监听 `127.0.0.1:18473`。
- Go 后端集中在 `main.go`（约 19000 行），包含 checkout、Stripe、Midtrans、GoPay、Chrome CDP、LuckMail 集成、审计日志、配置加载等逻辑。
- LuckMail SDK 位于 `LuckMailSdk-Go/luckmail/`，通过 `go.mod` 的 `replace` 指令本地引用，提供 client、user、supplier、models、errors 模块。
- 前端主流程在 `web/app.js`，实现状态机驱动的自动化工作流管理，页面 DOM 在 `web/index.html`，布局和响应式样式在 `web/styles.css`。
- browser-use 是独立 Node/Playwright 服务，默认监听 `127.0.0.1:38765`，提供 `GET /health` 和 `POST /run`。
- Chrome 扩展 `extension/` 为 Manifest V3，包含 background service worker、content script 和 popup 配置面板。

### API 事实

`main.go` 当前注册了以下 35 个 Go API：

```text
GET  /api/health
POST /api/sandbox/payment-authorization/assess
POST /api/voice/speak
POST /api/checkout
POST /api/checkout/start
POST /api/incognito/open
POST /api/incognito/close
POST /api/incognito/diagnostics
POST /api/login/click
POST /api/login/email-fill
POST /api/login/code-fill
POST /api/session/fetch
POST /api/gopay/force-link
POST /api/gopay/auto-link
POST /api/gopay/cdp-otp
POST /api/gopay/smart-link
POST /api/gopay/snap-probe
POST /api/gopay/monitor
POST /api/pricing/monitor
POST /api/pricing/plus-subscribe-probe
POST /api/luckmail/create-and-wait
POST /api/luckmail/token-code
POST /api/luckmail/token-mails
POST /api/luckmail/purchases
GET  /api/luckmail/config
POST /api/luckmail/config
DELETE /api/luckmail/config
POST /api/luckmail/config/test
POST /api/gptpls/record
POST /api/gopay/full-link
POST /api/gopay/auto-trigger-check
POST /api/gopay/midtrans-linking-fill
POST /api/checkout/resolve-target
POST /api/checkout/auto-fill
POST /api/checkout/payment-method-select
POST /api/checkout/authorized-subscribe-click
POST /api/perf/client
```

browser-use 当前注册：

```text
GET  /health
POST /run
OPTIONS *
```

## 发现的不一致与修正

| 编号 | 位置 | 不一致内容 | 代码事实 | 修正状态 |
| --- | --- | --- | --- | --- |
| DOC-001 | 根目录 `README.md` | 原标题和功能描述仍是"GoPay 支付流程控制台"，没有反映当前 checkout、CDP、browser-use、三栏工作台和一键启动能力 | 当前项目包含 Go 服务、嵌入式前端、Chrome CDP、browser-use 服务、Chrome 扩展、Windows 启动脚本 | 已修正 |
| DOC-002 | `web/readme.html` | 原文案描述"三步：初始化账单、验证 OTP、验证 PIN"，并列出 `/api/gopay/otp`、`/api/gopay/pin` | 当前路由注册中没有 `/api/gopay/otp` 和 `/api/gopay/pin`；主要 GoPay 入口为 `/api/gopay/full-link`、`/api/gopay/auto-link`、`/api/gopay/smart-link`、`/api/gopay/cdp-otp`、`/api/gopay/midtrans-linking-fill` | 已修正 |
| DOC-003 | `docs/` | 缺少项目级文档索引 | 当前项目需要从源码事实入口维护文档层级 | 已修正 |
| DOC-004 | `docs/architecture/overview.md` | 架构图缺少 `/api/login/*`、`/api/luckmail/*`、`/api/voice/speak`、`/api/sandbox/*`、`/api/gptpls/record`、`/api/perf/client` 路由组；模块组织表缺少 `LuckMailSdk-Go/`、`go.mod`、`config.json`、`web/mail-code.html` 等；依赖图谱不完整；功能清单缺少登录自动化、LuckMail 接码、语音播报、sandbox 评估等 | 代码中存在上述所有路由和模块 | 已修正（2026-05-12） |
| DOC-005 | `docs/api/reference.md` | 缺少 18 个 API 接口文档：`/api/incognito/diagnostics`、`/api/login/click`、`/api/login/email-fill`、`/api/login/code-fill`、`/api/gptpls/record`、`/api/perf/client`；LuckMail API（create-and-wait、token-code、token-mails、purchases）缺少请求/响应示例 | 代码中注册了全部 35 个路由 | 已修正（2026-05-12） |
| DOC-006 | `docs/development/guide.md` | 代码维护约定未提及 LuckMail SDK 本地引用和 `config.local.json` 敏感配置管理 | LuckMail SDK 通过 `go.mod` replace 指令本地引用；`config.json` 含敏感字段 | 已修正（2026-05-12） |
| DOC-007 | 历史设计文档 `docs/superpowers/specs/` | 文件是历史设计规格，未明确标注为历史设计而非当前完整 API 参考 | 当前事实应以 `main.go`、`web/`、browser-use runtime 为准 | 已在 docs 索引中标注为历史设计/规格参考 |
| DOC-008 | `docs/architecture/overview.md` | 缺少登录自动化流程和 LuckMail 邮箱接码流程的业务流程描述 | 代码中存在完整的 login/* 和 luckmail/* handler 实现 | 已修正（2026-05-12） |
| DOC-009 | `docs/architecture/overview.md` | 最后核对日期为 2026-05-07，已过期 | 当前日期为 2026-05-12 | 已修正（2026-05-12） |
| DOC-010 | `docs/development/guide.md` | 最后核对日期为 2026-05-07，已过期 | 当前日期为 2026-05-12 | 已修正（2026-05-12） |
| DOC-011 | `main.go` | `operationDisplayNames` 和 `operationTypes` 缺少 `/api/incognito/diagnostics` 和 `/api/gptpls/record` 的映射条目 | 两个路由已在 `mux.HandleFunc` 注册，但审计映射缺失 | 已修正（2026-05-17） |
| DOC-012 | `docs/README.md` | 文档层级表格缺少 `payment-authorization-tamper-sandbox-plan.md`；`superpowers/specs/` 的设计规格未标注"历史设计文档"；最后核对日期为 2026-05-11 | 该文档存在于 `docs/audit/` 目录 | 已修正（2026-05-17） |
| DOC-013 | `docs/checkout-payment-flow-analysis.md` | 日志文件扩展名为 `.log`，实际为 `.json`；时间格式为 `yyyyMMddHHmmss`，实际为 `yyyyMMdd_HHmmss_000`；最后核对日期为 2026-05-07 | 代码使用 `fmt.Sprintf(..., "20060102_150405_000")` 格式化和 `.json` 扩展名 | 已修正（2026-05-17） |
| DOC-014 | `docs/automation-log-analysis-p0-p4-plan.md` | 多处引用 `log/*.log`，实际文件扩展名为 `.json`；最后核对日期为 2026-05-12 | 日志文件为 JSON 格式 | 已修正（2026-05-17） |
| DOC-015 | `docs/development/guide.md` | 配置加载顺序未说明 `config.local.json` 优先级；配置表中缺少 `code_view_public_base_url` 字段；最后核对日期为 2026-05-12 | `loadAppConfig()` 的加载顺序为 APP_CONFIG → config.local.json → embedded config.json | 已修正（2026-05-17） |
| DOC-016 | `docs/architecture/overview.md` | browser-use runtime 模块列表缺少 `logger.js` 和 `errors.js`；缺少 `start-browser-use.ps1`；最后核对日期为 2026-05-12 | 这些文件存在于 `.codex/runtime/browser-use-service/src/` 和项目根目录 | 已修正（2026-05-17） |
| DOC-017 | `docs/audit/payment-authorization-tamper-sandbox-plan.md` | 最后核对日期为 2026-05-12 | 当前日期为 2026-05-17 | 已修正（2026-05-17） |

## 已更新文档体系

```text
docs/
  README.md
  architecture/
    overview.md
  api/
    reference.md
  development/
    guide.md
  audit/
    docs-code-consistency.md
    automation-log-analysis-p0-p4-plan.md
    payment-authorization-tamper-sandbox-plan.md
    checkout-payment-flow-analysis.md
  superpowers/
    specs/
      2026-05-06-checkout-auto-fill-target-lock-design.md
      2026-05-07-three-column-workbench-ui-design.md
```

## 真实性校验规则

后续维护文档时必须遵循以下规则：

1. API 文档只以 `mux.HandleFunc`、handler 请求结构和 `writeJSON` 响应为事实来源。
2. 前端功能说明必须同时核对 `web/index.html` 中的 DOM 与 `web/app.js` 中的实际 fetch 调用。
3. browser-use 文档必须以 `src/server.js`、`src/browser-use.js`、`src/cli.js` 与 `package.json` 为事实来源。
4. 配置项必须以 `appConfig`、`config.json`、环境变量覆盖逻辑为事实来源。
5. LuckMail SDK 文档必须以 `LuckMailSdk-Go/luckmail/` 下的源码为事实来源。
6. 历史规格文件不得冒充当前实现；若保留，必须在索引或正文中说明其性质。
7. 修改路由、请求字段、响应字段后必须同步更新 [reference.md](file:///f:/chatadd/docs/api/reference.md)。
8. 修改运行脚本或验证命令后必须同步更新 [guide.md](file:///f:/chatadd/docs/development/guide.md)。
9. 新增 API 路由时必须同步更新 `operationDisplayNames` 和 `operationTypes` 映射表。

## 待验证项

- 运行 `go test ./...` 验证 Go 代码和测试仍通过：待验证。
- 运行 browser-use `npm run check` 验证 Node runtime 语法仍通过：待验证。
- 运行 `python -m json.tool .codex\skillsets\playwright-ai.json > $null` 验证 skillset JSON：待验证。
- 再次检索旧文档中的不存在接口引用：只在本审计报告的问题记录中保留历史引用，`README.md` 与 `web/readme.html` 已无旧接口说明。