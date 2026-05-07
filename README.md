# Checkout Workbench 本地控制台

这是一个本地运行的 Go + 前端 + browser-use 工具台，用于生成 OpenAI checkout 支付链接、观察 ChatGPT/Stripe/Midtrans/GoPay 链路、通过 Chrome CDP 辅助提取 Session 与页面状态，并提供 Playwright 页面自动化分析能力。

## 功能概述

- 生成可直接打开的 OpenAI checkout 长链接。
- 支持粘贴 ChatGPT access token、JWT 或 `/api/auth/session` JSON，并由前端自动提取 token。
- 支持在上游 checkout 需要浏览器态时显式提供 ChatGPT Cookie/User-Agent。
- 支持启动带 CDP 调试端口的系统 Chrome 无痕窗口。
- 支持通过 CDP 获取 ChatGPT Session JSON。
- 支持 checkout 页面目标解析和随机美国地址自动填写，并校验当前页面必须匹配最近打开的支付链接。
- 支持 GoPay 自动绑定、智能绑定、Midtrans linking 页面填充、CDP OTP 填入、全流程支付辅助和页面监控。
- 支持 browser-use Node/Playwright 服务，用于网页导航、表单填写、页面数据提取和结构化分析。
- 支持 Chrome 扩展辅助提取已登录站点的 Cookies、LocalStorage、SessionStorage。
- 支持 Windows 一键启动 browser-use 服务或同时启动 Go 服务与 browser-use 服务。
- 支持 API 审计日志与敏感字段脱敏；完整敏感捕获需显式开启。

## 重要提示

这个项目不适合新手直接使用。

你需要理解 HTTP 请求、代理、Cookie/Header、Chrome CDP、Playwright、Go 编译运行、接口 mock，以及支付链路中的状态流转。项目只提供当前链路的本地编排、观测和页面辅助能力，不保证对任何外部站点开箱即用。

OTP 通道可选择 SMS 或 WhatsApp。项目只负责向 GoPay/Midtrans 相关链路发起请求、校验验证码或在浏览器页面中填入验证码，不自行生成或发送短信。

不要把 access token、refresh token、Cookie、Authorization、OTP、PIN 等敏感数据提交到仓库。普通环境应保持 `audit_capture_sensitive` 关闭。

## 运行

### Go 服务

```powershell
go run .
```

启动后访问：

```text
http://localhost:18473
```

也可以先构建：

```powershell
go build .
```

### browser-use 服务

```powershell
.\start-browser-use.cmd
```

默认地址：

```text
http://127.0.0.1:38765
```

### 同时启动全部本地服务

```powershell
.\start-all-services.cmd
```

## 配置

配置文件默认读取根目录 `config.json`，也可以通过环境变量 `APP_CONFIG` 指定其他配置文件。

常用配置包括：

- `checkout_endpoint` / `CHECKOUT_ENDPOINT`
- `checkout_approve_endpoint` / `CHECKOUT_APPROVE_ENDPOINT`
- `checkout_cookie` / `CHECKOUT_COOKIE`
- `checkout_user_agent` / `CHECKOUT_USER_AGENT`
- `audit_capture_sensitive` / `AUDIT_CAPTURE_SENSITIVE`
- `local_mock_base_url` / `LOCAL_MOCK_BASE_URL`
- `midtrans_mock_base_url` / `MIDTRANS_MOCK_BASE_URL`
- `gopay_gwa_mock_base_url` / `GOPAY_GWA_MOCK_BASE_URL`
- `gopay_customer_mock_base_url` / `GOPAY_CUSTOMER_MOCK_BASE_URL`
- `midtrans_linking_authorization` / `MIDTRANS_LINKING_AUTHORIZATION`
- `midtrans_linking_cookie` / `MIDTRANS_LINKING_COOKIE`
- `midtrans_charge_cookie` / `MIDTRANS_CHARGE_COOKIE`
- `proxy_test_urls` / `PROXY_TEST_URLS`

browser-use 服务支持：

- `BROWSER_USE_PORT`
- `BROWSER_USE_LOG_LEVEL`

## 验证

```powershell
go test ./...
```

```powershell
Set-Location .codex\runtime\browser-use-service
npm run check
```

```powershell
python -m json.tool .codex\skillsets\playwright-ai.json > $null
```

## 文档

完整文档入口位于 [docs/README.md](file:///f:/chatadd/docs/README.md)。

- 架构总览：[overview.md](file:///f:/chatadd/docs/architecture/overview.md)
- API 参考：[reference.md](file:///f:/chatadd/docs/api/reference.md)
- 开发指南：[guide.md](file:///f:/chatadd/docs/development/guide.md)
- 文档一致性审计：[docs-code-consistency.md](file:///f:/chatadd/docs/audit/docs-code-consistency.md)

内置网页说明页：`/readme.html`。