# LuckMail Go SDK

LuckMail 平台的 Go 语言 SDK，提供用户端和供应商端两套完整 API 接口。

## 特性

- ✅ **零外部依赖** - 仅使用 Go 标准库
- ✅ **完整 API 覆盖** - 用户端 + 供应商端
- ✅ **类型安全** - 所有 API 均有完整类型定义
- ✅ **Context 支持** - 所有请求均支持 context 超时控制
- ✅ **API Key 鉴权** - 通过 API Key 进行身份验证
- ✅ **智能轮询** - 内置接码轮询等待，支持回调

## 快速安装

```bash
go get github.com/TryHarder-L/luckmail
```

## 快速开始

### 用户端 - 一站式接码

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    luckmail "github.com/TryHarder-L/luckmail/luckmail"
)

func main() {
    // 初始化客户端
    client := luckmail.New("your_api_key_here")
    
    ctx := context.Background()
    
    // 一站式接码（创建订单 + 等待验证码）
    result, err := client.User.CreateAndWait(ctx, "twitter", &luckmail.OrderOptions{
        EmailType: "ms_graph",           // 邮箱类型（可选）
        Timeout:   5 * time.Minute,      // 等待超时（默认 5 分钟）
        Interval:  3 * time.Second,      // 轮询间隔（默认 3 秒）
    })
    if err != nil {
        log.Fatal(err)
    }
    
    if result.Status == "success" {
        fmt.Println("✅ 验证码:", result.VerificationCode)
        fmt.Println("📧 来自:", result.MailFrom)
    } else {
        fmt.Println("❌ 接码失败:", result.Status)
    }
}
```

### 供应商端 - 查看数据看板

```go
// 查看数据看板
summary, err := client.Supplier.GetDashboard(ctx)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("总邮箱: %d 个\n", summary.TotalEmails)
fmt.Printf("今日接码: %d 次\n", summary.TodayAssigned)
fmt.Printf("成功率: %.2f%%\n", summary.SuccessRate)
fmt.Printf("今日佣金: %s\n", summary.TodayCommission)
```

## 客户端配置

### 基础配置（API Key 模式）

```go
client := luckmail.New("your_api_key")
```

### 自定义超时

```go
client := luckmail.New(
    "your_api_key",
    luckmail.WithTimeout(60 * time.Second),
)
```

### 自定义 API 地址

```go
client := luckmail.New(
    "your_api_key",
    luckmail.WithBaseURL("https://your-domain.com"),
)
```

## 用户端 API

### 用户信息

```go
// 获取用户信息
info, err := client.User.GetUserInfo(ctx)
fmt.Printf("用户名: %s, 余额: %s\n", info.Username, info.Balance)

// 查询余额
balance, err := client.User.GetBalance(ctx)
fmt.Println("余额:", balance)
```

### 邮箱类型

```go
// 获取支持的邮箱类型
types, err := client.User.GetEmailTypes(ctx)
for _, t := range types {
    fmt.Printf("  %s: %s\n", t.Type, t.Name)
}
```

支持的邮箱类型：

| 类型 | 说明 |
|------|------|
| `ms_graph` | Microsoft Graph API（长效/短效） |
| `ms_imap` | Microsoft IMAP 密码登录 |
| `google_variant` | Google 变体邮箱 |
| `self_built` | 自建邮箱 |

### 我的邮箱管理

```go
// 获取邮箱列表
result, err := client.User.GetEmails(ctx, &luckmail.GetEmailsParams{
    Page:     1,
    PageSize: 20,
    Keyword:  "outlook",
    Status:   1,    // 1=正常 2=异常 4=禁用
})
for _, email := range result.List {
    fmt.Printf("  %s (状态: %d)\n", email.Address, email.Status)
}

// 导入私有邮箱
importResult, err := client.User.ImportEmails(ctx, &luckmail.ImportEmailsRequest{
    Type: "ms_graph",
    Emails: []map[string]interface{}{
        {
            "address":       "user@outlook.com",
            "client_id":     "xxx",
            "refresh_token": "xxx",
        },
    },
})
fmt.Printf("成功: %d, 重复: %d, 失败: %d\n",
    importResult.Success, importResult.Duplicate, importResult.Failed)

// 导出邮箱
content, err := client.User.ExportEmails(ctx, "", 1)  // 导出正常状态的邮箱
os.WriteFile("emails.txt", content, 0644)
```

### 项目列表

```go
projects, err := client.User.GetProjects(ctx, 1, 50)
for _, p := range projects.List {
    fmt.Printf("[%s] %s (超时: %ds)\n", p.Code, p.Name, p.TimeoutSeconds)
    for _, price := range p.Prices {
        fmt.Printf("  %s: 接码 %s / 购买 %s\n", 
            price.EmailType, price.CodePrice, price.BuyPrice)
    }
}
```

### 接码订单

#### 一站式接码（推荐）

```go
result, err := client.User.CreateAndWait(ctx, "twitter", &luckmail.OrderOptions{
    EmailType: "ms_graph",
    Domain:    "outlook.com",    // 指定域名（可选）
    Timeout:   5 * time.Minute,
    Interval:  3 * time.Second,
    OnPoll: func(code *luckmail.OrderCode) {
        fmt.Println("轮询中... 状态:", code.Status)
    },
})
```

#### 分步接码

```go
// 第一步：创建订单
order, err := client.User.CreateOrder(ctx, "twitter", &luckmail.OrderOptions{
    EmailType: "ms_graph",
})
fmt.Println("订单号:", order.OrderNo, "邮箱:", order.EmailAddress)

// 第二步：等待验证码（带轮询）
code, err := client.User.WaitForCode(ctx, order.OrderNo, 300*time.Second, 3*time.Second, nil)
if code.Status == "success" {
    fmt.Println("验证码:", code.VerificationCode)
}

// 手动单次查询
code, err := client.User.GetOrderCode(ctx, order.OrderNo)

// 取消订单
err = client.User.CancelOrder(ctx, order.OrderNo)

// 查看订单列表
orders, err := client.User.GetOrders(ctx, &luckmail.GetOrdersParams{
    Status: 2,   // 1=待接码 2=已完成 3=已超时 4=已取消 5=已退款
})
```

订单状态说明：

| 状态 | 说明 |
|------|------|
| `pending` | 等待验证码 |
| `success` | 接码成功 |
| `timeout` | 订单超时 |
| `cancelled` | 已取消 |

### 购买邮箱

```go
// 购买邮箱
result, err := client.User.PurchaseEmails(ctx, &luckmail.PurchaseEmailsRequest{
    ProjectCode: "twitter",
    Quantity:    5,
    EmailType:   "ms_graph",
    Domain:      "outlook.com",    // 可选
})
for _, item := range result.Purchases {
    fmt.Printf("%s -> token: %s\n", item.EmailAddress, item.Token)
}

// 获取已购邮箱列表
purchases, err := client.User.GetPurchases(ctx, nil)

// 通过 Token 查询验证码（自动轮询）
code, err := client.User.WaitForTokenCode(ctx, "tok_abc123", 120*time.Second, 3*time.Second, nil)
if code.HasNewMail {
    fmt.Println("验证码:", code.VerificationCode)
}

// 通过 Token 测活（可传空 API Key，仅依赖 token-only 接口）
alive, err := client.User.CheckTokenAlive(ctx, "tok_abc123")
if err == nil {
    fmt.Println("alive:", alive.Alive, "message:", alive.Message, "mail_count:", alive.MailCount)
}

// 单次查询
code, err := client.User.GetTokenCode(ctx, "tok_abc123")
```

### 申述

```go
// 提交申述
result, err := client.User.CreateAppeal(ctx, &luckmail.CreateAppealRequest{
    AppealType:  1,          // 1=接码订单 2=购买邮箱
    OrderID:     123,        // 接码订单 ID
    Reason:      "no_code",  // no_code / wrong_code / email_invalid
    Description: "等待 5 分钟未收到验证码",
})
fmt.Println("申述单号:", result.AppealNo)
```

## 供应商端 API

### 供应商信息

```go
// 获取个人信息
profile, err := client.Supplier.GetProfile(ctx)
fmt.Printf("余额: %s, 接码佣金率: %s\n", profile.Balance, profile.CodeCommissionRate)
```

### 邮箱管理

```go
// 获取邮箱列表
result, err := client.Supplier.GetEmails(ctx, &luckmail.GetSupplierEmailsParams{
    EmailType:   "ms_graph",
    IsShortTerm: 0,   // 0=长效 1=短效 -1=不过滤
    Status:      1,   // 1=正常 2=异常 4=禁用
})

// 导入邮箱
importResult, err := client.Supplier.ImportEmails(ctx, &luckmail.ImportSupplierEmailsRequest{
    Type:        "ms_graph",
    IsShortTerm: 0,
    Emails: []map[string]interface{}{
        {
            "address":       "user@outlook.com",
            "client_id":     "xxx",
            "refresh_token": "xxx",
        },
    },
})

// 导出邮箱
content, err := client.Supplier.ExportEmails(ctx, "", "ms_graph", 0, 1)
```

### 申述管理

```go
// 获取申述列表
appeals, err := client.Supplier.GetAppeals(ctx, &luckmail.GetAppealsParams{
    Status: 1,   // 1=待处理 2=已同意 3=待仲裁 4=已拒绝
})

// 查看申述详情
detail, err := client.Supplier.GetAppeal(ctx, "APL20240310001")

// 处理申述（单条）
// result: 1=同意退款 2=拒绝申述 3=申请仲裁
err = client.Supplier.ReplyAppeal(ctx, "APL20240310001", 1, "邮箱确有问题，同意退款")

// 批量处理申述
batchResult, err := client.Supplier.BatchReplyAppeals(ctx, &luckmail.BatchReplyAppealsRequest{
    AppealNos: []string{"APL001", "APL002"},
    Result:    2,
    Reply:     "经验证邮箱正常，拒绝申述",
})
fmt.Printf("成功: %d, 失败: %d\n", batchResult.Success, batchResult.Failed)
```

### 数据看板

```go
summary, err := client.Supplier.GetDashboard(ctx)
fmt.Printf("总邮箱: %d\n", summary.TotalEmails)
fmt.Printf("活跃邮箱: %d\n", summary.ActiveEmails)
fmt.Printf("累计接码: %d\n", summary.TotalAssigned)
fmt.Printf("累计成功: %d\n", summary.TotalSuccess)
fmt.Printf("成功率: %.2f%%\n", summary.SuccessRate)
fmt.Printf("累计佣金: %s\n", summary.TotalCommission)
fmt.Printf("可用余额: %s\n", summary.AvailableBalance)
fmt.Printf("今日接码: %d\n", summary.TodayAssigned)
fmt.Printf("今日成功: %d\n", summary.TodaySuccess)
fmt.Printf("今日佣金: %s\n", summary.TodayCommission)
```

## 错误处理

```go
result, err := client.User.GetBalance(ctx)
if err != nil {
    switch e := err.(type) {
    case *luckmail.AuthError:
        fmt.Println("鉴权失败:", e.Message)
    case *luckmail.APIError:
        fmt.Printf("API 错误 [%d]: %s\n", e.Code, e.Message)
    case *luckmail.NetworkError:
        fmt.Println("网络错误:", e.Message)
    case *luckmail.TimeoutError:
        fmt.Println("请求超时:", e.Message)
    default:
        fmt.Println("未知错误:", err)
    }
}
```

## 完整示例

- [用户端示例](examples/user/main.go)
- [供应商端示例](examples/supplier/main.go)

## 文件结构

```
LuckMailSdk-Go/
├── go.mod                        # Go 模块定义
├── README.md                     # 本文档
├── luckmail/
│   ├── client.go                 # 主客户端入口
│   ├── errors.go                 # 异常类型定义
│   ├── models.go                 # 数据模型定义
│   ├── http_client.go            # HTTP 客户端（鉴权/请求）
│   ├── user.go                   # 用户端 API 实现
│   └── supplier.go               # 供应商端 API 实现
└── examples/
    ├── user/main.go              # 用户端完整示例
    └── supplier/main.go          # 供应商端完整示例
```

## API 鉴权方式

### API Key 模式

在请求头中添加：

```
X-API-Key: your_api_key
```

## 注意事项

- API Key 在平台「个人设置」页面生成
- `GetTokenCode` / `CheckTokenAlive` / `GetTokenMails` / `GetTokenMailDetail` 属于 token-only 接口，可用空 API Key 初始化客户端
- 所有金额字段均为字符串格式（保留 4 位小数），如 `"150.0000"`
- 接码订单超时时间由项目配置决定（通常 300 秒）
- `WaitForCode` / `WaitForTokenCode` 默认轮询间隔 3 秒，可自定义
- 建议使用 `context.WithTimeout` 控制总体超时
