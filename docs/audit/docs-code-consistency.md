# 文档与代码一致性审计报告

最后核对日期：2026-05-07

## 审计范围

本次审计覆盖：

- Go 后端入口、路由、请求结构、返回结构和配置项：[main.go](file:///f:/chatadd/main.go)
- Go 测试与当前验证方式：[main_test.go](file:///f:/chatadd/main_test.go)
- 前端页面结构与运行流程：[index.html](file:///f:/chatadd/web/index.html)、[app.js](file:///f:/chatadd/web/app.js)、[styles.css](file:///f:/chatadd/web/styles.css)
- browser-use Node/Playwright 服务：[server.js](file:///f:/chatadd/.codex/runtime/browser-use-service/src/server.js)、[browser-use.js](file:///f:/chatadd/.codex/runtime/browser-use-service/src/browser-use.js)、[cli.js](file:///f:/chatadd/.codex/runtime/browser-use-service/src/cli.js)
- Windows 启动脚本：`start-browser-use.cmd`、`start-all-services.cmd`
- 文档目录：`docs/` 及其子目录
- 旧入口文档：根目录 `README.md` 与 `web/readme.html`

## 当前代码事实摘要

### 架构事实

- 项目是本地 Checkout Workbench，而不是单一的 GoPay 三步控制台。
- Go 服务使用 `embed.FS` 嵌入 `web/` 静态文件，并固定监听 `127.0.0.1:18473`。
- Go 后端集中在 `main.go`，包含 checkout、Stripe、Midtrans、GoPay、Chrome CDP、审计日志、配置加载等逻辑。
- 前端主流程在 `web/app.js`，页面 DOM 在 `web/index.html`，布局和响应式样式在 `web/styles.css`。
- browser-use 是独立 Node/Playwright 服务，默认监听 `127.0.0.1:38765`，提供 `GET /health` 和 `POST /run`。
- Chrome 扩展 `extension/` 用于浏览器态数据提取辅助。

### API 事实

`main.go` 当前注册了以下 Go API：

```text
GET  /api/health
POST /api/checkout
POST /api/checkout/start
POST /api/incognito/open
POST /api/session/fetch
POST /api/gopay/force-link
POST /api/gopay/auto-link
POST /api/gopay/cdp-otp
POST /api/gopay/smart-link
POST /api/gopay/snap-probe
POST /api/gopay/monitor
POST /api/pricing/monitor
POST /api/gopay/full-link
POST /api/gopay/auto-trigger-check
POST /api/gopay/midtrans-linking-fill
POST /api/checkout/resolve-target
POST /api/checkout/auto-fill
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
| DOC-001 | 根目录 `README.md` | 原标题和功能描述仍是“GoPay 支付流程控制台”，没有反映当前 checkout、CDP、browser-use、三栏工作台和一键启动能力 | 当前项目包含 Go 服务、嵌入式前端、Chrome CDP、browser-use 服务、Chrome 扩展、Windows 启动脚本 | 已修正：根 README 已更新为 Checkout Workbench 当前能力说明 |
| DOC-002 | `web/readme.html` | 原文案描述“三步：初始化账单、验证 OTP、验证 PIN”，并列出 `/api/gopay/otp`、`/api/gopay/pin` | 当前路由注册中没有 `/api/gopay/otp` 和 `/api/gopay/pin`；主要 GoPay 入口为 `/api/gopay/full-link`、`/api/gopay/auto-link`、`/api/gopay/smart-link`、`/api/gopay/cdp-otp`、`/api/gopay/midtrans-linking-fill` | 已修正：`web/readme.html` 已改为当前 Checkout Workbench 与已注册 API 说明 |
| DOC-003 | `docs/` | 缺少项目级文档索引 | 当前项目需要从源码事实入口维护文档层级 | 已修正：新增 [README.md](file:///f:/chatadd/docs/README.md) |
| DOC-004 | `docs/architecture/` | 缺少与当前源码一致的架构总览、模块组织和依赖图谱 | 代码存在 Go 服务、前端、browser-use、Chrome 扩展和 Windows 脚本多模块协作 | 已修正：新增 [overview.md](file:///f:/chatadd/docs/architecture/overview.md) |
| DOC-005 | `docs/api/` | 缺少与 `mux.HandleFunc` 完全对齐的 API 参考 | 当前 `main.go` 注册 17 个 Go API；browser-use 注册 2 个业务 API | 已修正：新增 [reference.md](file:///f:/chatadd/docs/api/reference.md) |
| DOC-006 | `docs/development/` | 缺少当前开发、运行、配置和验证工作流 | 当前应使用 `go run .`、`go test ./...`、browser-use `npm run check`、Windows `.cmd` 脚本和配置环境变量 | 已修正：新增 [guide.md](file:///f:/chatadd/docs/development/guide.md) |
| DOC-007 | 历史设计文档 `docs/superpowers/specs/` | 文件是历史设计规格，未明确标注为历史设计而非当前完整 API 参考 | 当前事实应以 `main.go`、`web/`、browser-use runtime 为准 | 已在 docs 索引中标注为历史设计/规格参考 |

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
5. 历史规格文件不得冒充当前实现；若保留，必须在索引或正文中说明其性质。
6. 修改路由、请求字段、响应字段后必须同步更新 [reference.md](file:///f:/chatadd/docs/api/reference.md)。
7. 修改运行脚本或验证命令后必须同步更新 [guide.md](file:///f:/chatadd/docs/development/guide.md)。

## 待验证项

- 运行 `go test ./...` 验证 Go 代码和测试仍通过：已通过。
- 运行 browser-use `npm run check` 验证 Node runtime 语法仍通过：已通过。
- 运行 `python -m json.tool .codex\skillsets\playwright-ai.json > $null` 验证 skillset JSON：已通过。
- 再次检索旧文档中的不存在接口引用：只在本审计报告的问题记录中保留历史引用，`README.md` 与 `web/readme.html` 已无旧接口说明。