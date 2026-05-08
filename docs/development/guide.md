# 开发指南

最后核对日期：2026-05-07

## 环境要求

| 工具 | 用途 |
| --- | --- |
| Go 1.25+ | 构建并运行本地 Go 服务 |
| Node.js 18+ | 运行 browser-use Node/Playwright 服务 |
| npm | 安装 browser-use 依赖 |
| Chrome | Go 后端通过系统 Chrome 无痕窗口和 CDP 执行 Session、checkout、GoPay 辅助操作 |
| Playwright 浏览器 | browser-use 服务执行自动化任务 |

## 本地运行

### 只运行 Go 服务

```powershell
go run .
```

访问：

```text
http://127.0.0.1:18473/
```

### 构建 Go 服务

```powershell
go build .
```

### 启动 browser-use 服务

推荐在 Windows 上运行：

```powershell
.\start-browser-use.cmd
```

该脚本会进入 `.codex\runtime\browser-use-service`，在缺少依赖时执行 `npm install --no-fund --no-audit`，并检查 Chromium 浏览器环境。

也可以手动运行：

```powershell
Set-Location .codex\runtime\browser-use-service
npm install --no-fund --no-audit
npx playwright install chromium
npm start
```

健康检查：

```text
http://127.0.0.1:38765/health
```

### 同时启动 Go 服务和 browser-use 服务

```powershell
.\start-all-services.cmd
```

脚本会打开两个 PowerShell 服务窗口：

- `chatadd-go-service`
- `browser-use-service`

## 配置来源

Go 服务默认使用编译嵌入的 `config.json`。如设置 `APP_CONFIG`，则优先读取该路径指向的 JSON 配置文件。

| 配置字段 | 环境变量 | 说明 |
| --- | --- | --- |
| `checkout_endpoint` | `CHECKOUT_ENDPOINT` | checkout 上游接口，默认 `https://chatgpt.com/backend-api/payments/checkout` |
| `checkout_approve_endpoint` | `CHECKOUT_APPROVE_ENDPOINT` | checkout approve 接口；为空时等于 checkout endpoint 加 `/approve` |
| `checkout_cookie` | `CHECKOUT_COOKIE` | 上游 checkout 请求使用的 Cookie，前端请求中的 `checkout_session.cookie` 优先 |
| `checkout_user_agent` | `CHECKOUT_USER_AGENT` | 上游 checkout 请求使用的 User-Agent，前端请求中的 `checkout_session.user_agent` 优先 |
| `audit_capture_sensitive` | `AUDIT_CAPTURE_SENSITIVE` | 是否在审计日志中保留完整请求/响应敏感内容 |
| `local_mock_base_url` | `LOCAL_MOCK_BASE_URL` | 本地 mock 基础地址，默认 `http://localhost:8282` |
| `midtrans_mock_base_url` | `MIDTRANS_MOCK_BASE_URL` | Midtrans mock 或目标基础地址 |
| `gopay_gwa_mock_base_url` | `GOPAY_GWA_MOCK_BASE_URL` | GoPay GWA mock 或目标基础地址 |
| `gopay_customer_mock_base_url` | `GOPAY_CUSTOMER_MOCK_BASE_URL` | GoPay customer mock 或目标基础地址 |
| `midtrans_linking_authorization` | `MIDTRANS_LINKING_AUTHORIZATION` | Midtrans linking 请求 Authorization |
| `midtrans_linking_cookie` | `MIDTRANS_LINKING_COOKIE` | Midtrans linking Cookie |
| `midtrans_charge_cookie` | `MIDTRANS_CHARGE_COOKIE` | Midtrans charge Cookie |
| `proxy_test_urls` | `PROXY_TEST_URLS` | 代理测试目标 URL 列表 |
| `enable_redteam_apis` | `ENABLE_REDTEAM_APIS` | 是否启用本地 redteam 模拟接口；默认关闭，且接口仅允许 loopback 访问 |

browser-use 服务配置：

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `BROWSER_USE_PORT` | `38765` | browser-use HTTP 服务端口 |
| `BROWSER_USE_LOG_LEVEL` | `info` | browser-use 服务日志级别 |

## 验证命令

### Go 测试

```powershell
go test ./...
```

### browser-use 语法检查

```powershell
Set-Location .codex\runtime\browser-use-service
npm run check
```

### skillset JSON 校验

```powershell
python -m json.tool .codex\skillsets\playwright-ai.json > $null
```

### 页面诊断

在修改 `web/index.html`、`web/app.js`、`web/styles.css` 后，应检查 IDE 诊断，并至少运行 `go test ./...`。如果修改 browser-use runtime，应运行 `npm run check`。

### 本地 redteam 调试

如果需要使用 `/api/redteam/*` 进行本地防御模拟，请先显式设置：

```powershell
$env:ENABLE_REDTEAM_APIS = "true"
```

注意事项：

- redteam 接口默认关闭。
- 即使已开启，也仅允许 `127.0.0.1` / loopback 请求访问。
- 这些接口只用于 defensive simulation 和只读摘要分析，不执行真实支付。

## 代码维护约定

- Go 后端目前集中在 `main.go`，新增 API 前先检查现有 handler、请求结构和审计映射。
- 新增 API 路由时必须同步更新：
  - `mux.HandleFunc` 注册表。
  - `operationDisplayNames`。
  - `operationTypes`。
  - `docs/api/reference.md`。
- 修改前端 DOM id 前必须检查 [app.js](file:///f:/chatadd/web/app.js) 的选择器绑定。
- 修改 Session、GoPay、checkout 自动填地址逻辑时，优先补充或更新 [main_test.go](file:///f:/chatadd/main_test.go) 中的针对性测试。
- 不要把 access token、refresh token、Cookie、Authorization 明文写入仓库。
- `log/`、`.playwright-mcp/`、browser-use `node_modules/` 和 browser-use artifacts 已被 `.gitignore` 忽略。

## 文档更新工作流

1. 先从源码提取事实：路由、请求结构、返回字段、配置项、前端调用点。
2. 再更新 `docs/architecture/overview.md` 和 `docs/api/reference.md`。
3. 如果发现旧文档与代码不一致，记录到 `docs/audit/docs-code-consistency.md`。
4. 运行 `go test ./...`，涉及 browser-use 时运行 `npm run check`。
5. 不用文档描述尚未在代码中实现的功能，除非明确标注为设计目标或历史规格。
