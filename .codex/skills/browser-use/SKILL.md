---
name: "browser-use"
description: "Uses Playwright with AI-oriented page analysis for browser automation, form filling, and full-page data extraction. Invoke when tasks require operating a website, submitting forms, or extracting comprehensive page content."
---

# Browser Use

## Purpose

`browser-use` 是项目专属浏览器自动化技能，用于在网页中执行可重复、可审计、可恢复的操作流程，并对页面内容进行完整分析与提取。

该技能必须覆盖以下核心能力：

- 使用 Playwright 启动和控制浏览器
- 导航到目标页面并等待页面进入稳定状态
- 按给定字段规范填写网页表单
- 提交表单并检测成功、失败或部分完成状态
- 提取页面中的全部可获取数据，包括文本、结构化数据、属性值、链接、媒体、表格、列表、表单值和元数据
- 生成清晰的日志、执行摘要和错误报告

## Runtime

本技能已落地为本地 Node + Playwright 运行时：

- 运行时根目录：`f:\chatadd\.codex\runtime\browser-use-service`
- CLI：`node src/cli.js`
- HTTP 服务：`node src/server.js`
- 健康检查：`GET http://127.0.0.1:38765/health`
- 执行入口：`POST http://127.0.0.1:38765/run`

## When To Invoke

在以下场景必须优先使用该技能：

- 用户要求自动打开网页并执行点击、输入、选择、上传、提交等操作
- 用户要求在网页表单中填写指定数据
- 用户要求分析一个网页的完整内容或抓取全部页面数据
- 用户要求将浏览器自动化与 AI 分析结合，生成结构化提取结果
- 任务需要明确的错误处理、重试、截图、日志和执行轨迹

## Inputs

技能调用时应尽量整理出以下输入：

- `url`: 目标网页地址
- `task`: 目标操作说明
- `formData`: 需要写入表单的数据对象，键应与字段语义对应
- `fieldHints`: 可选字段定位提示，例如 label、placeholder、name、id、selector
- `submit`: 是否提交表单
- `waitFor`: 提交后等待的文本、URL 片段、选择器或状态条件
- `extract`: 需要提取的数据范围，默认使用 full
- `outputSchema`: 期望返回的结构化模式
- `headless`: 是否无头运行，默认 true
- `timeoutMs`: 页面与动作超时时间
- `headers`: 页面导航时附加的请求头
- `storageState`: 可选 Playwright storageState 文件路径

## Operating Rules

### 1. Browser Setup

执行时遵循以下顺序：

1. 启动 Playwright 浏览器实例
2. 创建隔离上下文，避免污染已有会话
3. 注册控制台日志、页面错误、请求失败和响应异常监听器
4. 设置统一导航超时与动作超时
5. 如任务涉及鉴权，优先从显式输入中读取 Cookie、Header 或存储态

### 2. Navigation

进入页面时必须：

- 记录开始时间、目标 URL 和浏览器参数
- 使用 `goto` 导航并等待 `domcontentloaded`
- 视页面类型追加等待：`load`、关键元素出现、网络空闲或业务提示文本出现
- 若发生重定向，记录完整跳转链
- 若页面加载失败，进入错误恢复流程

### 3. Form Filling

填写表单时必须：

- 先扫描页面中的全部输入控件：`input`、`textarea`、`select`、`button`、可编辑区域
- 优先按语义定位字段：`label`、`name`、`placeholder`、`aria-label`、`id`
- 仅在语义定位失败时回退到显式 selector
- 写入前记录字段名称、定位方式、控件类型和原始状态
- 写入后验证值是否成功反映到 DOM
- 对以下控件分别处理：
  - 文本输入框：`fill`
  - 下拉框：`selectOption`
  - 单选/复选：`check` / `uncheck`
  - 日期时间字段：按页面格式写入并再次校验
  - 富文本或 contenteditable：使用聚焦后键入或脚本写入
  - 文件上传：仅在任务明确提供文件路径时设置文件
- 若字段缺失或值不兼容，记录 warning 并汇总到最终结果

### 4. Submission

若 `submit=true`，必须：

- 优先识别与当前表单关联的提交按钮
- 提交前再次校验必填字段是否已填入
- 点击提交后同时监控以下信号：
  - URL 变化
  - 成功或失败提示文本
  - 弹窗、toast、alert
  - 接口失败
  - 新出现的表单校验错误
- 若页面无明确成功标记，返回 uncertain 状态并说明原因

### 5. Full Data Extraction

提取页面内容时必须尽可能完整，至少包含以下维度：

- 页面基础元信息：`title`、`url`、`meta`、canonical、lang
- 文本内容：可见文本、标题、段落、按钮文案、标签文本
- 结构化数据：JSON-LD、Microdata、Open Graph、Twitter Card
- DOM 结构摘要：主要区块、表单、表格、列表、卡片区域
- 链接与资源：`href`、`src`、图片 alt、脚本和样式资源引用
- 属性值：`id`、`class`、`name`、`value`、`placeholder`、`role`、`aria-*`、`data-*`
- 表单当前状态：字段值、默认值、校验状态、是否禁用
- 表格与列表数据：按行列或层级展开
- 媒体数据：图片、视频、音频、下载链接
- 页面内嵌 JSON 或脚本中的可解析业务数据

提取输出建议组织为：

```json
{
  "page": {},
  "text": {},
  "structured": {},
  "forms": [],
  "tables": [],
  "lists": [],
  "links": [],
  "media": [],
  "attributes": [],
  "embeddedData": [],
  "logs": [],
  "errors": []
}
```

## Logging Requirements

日志必须详细且可审计，至少覆盖：

- `session.start`: 浏览器启动参数、时间戳、目标任务
- `navigation.start` / `navigation.done`: 页面加载过程、状态码、重定向信息
- `form.scan`: 检测到的表单与字段清单
- `form.fill`: 每个字段的定位方式、写入值摘要、校验结果
- `form.submit`: 提交动作、等待条件、提交结果
- `extract.start` / `extract.done`: 提取范围、节点数量、输出摘要
- `warning`: 字段缺失、元素不可见、值校验失败、结构异常
- `error`: 超时、定位失败、页面崩溃、脚本执行失败、网络异常
- `session.end`: 总耗时、成功状态、失败原因、产出摘要

日志级别应至少区分：

- `info`
- `warning`
- `error`
- `debug`（仅在需要深度排查时输出）

## Error Handling

技能实现必须具备完整错误处理机制：

### Recoverable Errors

以下错误应优先恢复而非立即终止：

- 元素暂未出现
- 页面局部渲染延迟
- 某个字段定位失败但存在其他候选定位方式
- 单个提取节点解析失败
- 页面资源请求个别失败

恢复策略：

- 等待后重试
- 切换备用定位策略
- 缩小交互范围后再尝试
- 对单个节点跳过并记录 warning

### Non-recoverable Errors

以下错误应立即终止当前关键流程并输出错误摘要：

- 浏览器或页面上下文创建失败
- 目标 URL 非法或无法访问
- 页面崩溃
- 提交前核心必填字段全部无法定位
- 关键提交动作连续失败且超过重试上限

终止时必须：

- 保留最后阶段日志
- 尽量截取当前页面状态
- 输出明确错误分类、阶段、原因和已尝试恢复动作

## Execution Pattern

推荐的执行伪代码如下：

```text
start session
launch browser
create context and page
attach loggers and failure listeners
goto target url
wait for stable state
scan forms and page structure
fill form fields from formData
validate filled values
submit if requested
extract full page data
assemble structured result
return logs + extracted data + status
handle any error with retry, snapshot, and summary
close page/context/browser safely
```

## Output Contract

技能最终应返回统一结果对象：

```json
{
  "status": "success | partial | failed | uncertain",
  "task": "string",
  "url": "string",
  "submitted": true,
  "summary": "string",
  "data": {},
  "logs": [],
  "warnings": [],
  "errors": []
}
```

### Status Semantics

- `success`: 页面操作完成且提取结果完整可用
- `partial`: 主要任务完成，但有部分字段或数据未成功处理
- `failed`: 关键流程失败，无法得到可用结果
- `uncertain`: 操作已执行，但页面没有提供足够信号确认最终状态

## Safety Constraints

- 不记录或回显敏感凭据原文，日志中仅保留脱敏摘要
- 不在未获明确指令时执行高风险提交、支付确认或账户修改
- 不依赖脆弱的绝对定位，优先语义化选择器
- 不因单个节点失败而丢弃整个提取结果

## Recommended Playwright Techniques

- 使用 `locator` 而不是一次性 `querySelector`
- 使用显式等待而不是固定 sleep
- 使用 `evaluate` 提取复杂结构化数据
- 使用 `page.on('console')`、`page.on('pageerror')`、`page.on('requestfailed')` 收集诊断信息
- 在失败时保留截图、DOM 摘要和最近操作日志

## Example Invocation Intent

- 打开注册页，填写姓名、邮箱、手机号与密码，提交后提取成功提示与用户面板数据
- 打开后台列表页，提取所有表格内容、链接地址、按钮动作和 data 属性
- 打开详情页，抓取全文文本、JSON-LD、图片链接、表单默认值和 meta 信息
