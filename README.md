# GoPay 支付流程控制台

这是一个本地运行的 Go + 前端控制台，用来观察和拆分 GoPay 相关支付链路。界面把流程拆成几个阶段，方便查看每一步请求、返回和最终状态。

## 功能概述

- 初始化账单并展示关键阶段返回值。
- 按步骤执行 GoPay SMS/WhatsApp OTP 和 PIN 验证。
- 展示最终交易状态、订单号、金额和结算时间。
- 支持代理配置和代理测试。
- 支持通过 `config.json` 配置本地 mock 或目标接口地址。
- 支持在上游 checkout 返回 403 时显式提供 ChatGPT 会话 Cookie/User-Agent。
- 内置网页说明页：`/readme.html`。

## 重要提示

这个项目不适合新手直接使用。

你需要理解 HTTP 请求、代理、Cookie/Header、接口 mock、Go 编译运行，以及支付链路中的状态流转。项目只提供当前链路的基础编排和页面展示，不保证开箱即用。

OTP 通道可在首页选择 SMS 短信或 WhatsApp。项目只负责向 GoPay/Midtrans 发起对应通道的 OTP 请求并校验验证码，不自行生成或发送短信。

## 运行

```bash
go build .
```

启动后访问：

```text
http://localhost:18473
```

配置文件默认读取根目录的 `config.json`，也可以通过环境变量 `APP_CONFIG` 指定其他配置文件。

如果初始化账单返回 `checkout_upstream` 且状态码是 403，通常是上游要求当前浏览器会话上下文。可以在首页“ChatGPT 会话”里手动填入当前 `chatgpt.com` 的 Cookie，或在 `config.json`/环境变量里配置 `checkout_cookie`、`CHECKOUT_COOKIE`。
