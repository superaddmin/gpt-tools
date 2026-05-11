# 项目文档索引

最后核对日期：2026-05-11

本目录中的文档以当前源码为唯一事实来源。后端事实主要来自 [main.go](file:///f:/chatadd/main.go)，前端事实主要来自 [index.html](file:///f:/chatadd/web/index.html)、[app.js](file:///f:/chatadd/web/app.js)、[styles.css](file:///f:/chatadd/web/styles.css)，browser-use 服务事实来自 [.codex/runtime/browser-use-service](file:///f:/chatadd/.codex/runtime/browser-use-service)。

## 文档层级

| 文档 | 内容 |
| --- | --- |
| [架构总览](architecture/overview.md) | 项目模块、依赖关系、业务流程和运行时边界 |
| [API 参考](api/reference.md) | Go 本地服务和 browser-use 服务的接口定义、请求参数、返回结构 |
| [开发指南](development/guide.md) | 本地运行、验证命令、配置项、脚本和文档维护规范 |
| [文档一致性审计](audit/docs-code-consistency.md) | 本次代码与文档一致性检查结果、已修正偏差和遗留注意事项 |
| [Checkout 支付完成链路分析与报错日志方案](audit/checkout-payment-flow-analysis.md) | Checkout 到 GoPay 支付完成链路、异常路径和账号维度日志字段方案 |
| [自动化日志全量分析与 P0-P4 改造方案](audit/automation-log-analysis-p0-p4-plan.md) | 基于全量日志的瓶颈识别、状态机方案、P0-P4 自动化改造路径与多角色可行性评审 |
| [Checkout Auto-Fill Target Lock Design](superpowers/specs/2026-05-06-checkout-auto-fill-target-lock-design.md) | 自动填地址目标锁定设计规格 |
| [三栏工作台 UI 优化设计](superpowers/specs/2026-05-07-three-column-workbench-ui-design.md) | 当前三栏工作台 UI 设计规格 |

## 当前系统边界

项目由四个主要部分组成：

1. Go 本地 HTTP 服务，监听 `127.0.0.1:18473`。
2. 嵌入式前端页面，位于 `web/`。
3. browser-use Node/Playwright 本地服务，默认监听 `127.0.0.1:38765`。
4. Chrome 扩展，位于 `extension/`，用于凭证提取辅助场景。

## 文档维护规则

- API 文档必须与 [main.go](file:///f:/chatadd/main.go#L895-L911) 中注册的路由保持一致。
- 前端流程文档必须与 [app.js](file:///f:/chatadd/web/app.js) 中的实际 `fetch` 调用保持一致。
- browser-use 文档必须与 [server.js](file:///f:/chatadd/.codex/runtime/browser-use-service/src/server.js) 和 [browser-use.js](file:///f:/chatadd/.codex/runtime/browser-use-service/src/browser-use.js) 保持一致。
- 历史设计规格可以保留设计意图，但如与代码实现不同，必须在审计文档中明确标注。
