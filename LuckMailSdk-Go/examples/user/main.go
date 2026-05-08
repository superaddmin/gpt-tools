// 用户端 API 使用示例
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	luckmail "github.com/TryHarder-L/luckmail/luckmail"
)

func main() {
	// ===== 初始化客户端 =====
	client := luckmail.New(
		"your_api_key_here",
	)

	ctx := context.Background()

	// ===== 示例 1：查询余额 =====
	fmt.Println("=== 查询余额 ===")
	balance, err := client.User.GetBalance(ctx)
	if err != nil {
		log.Fatal("查询余额失败:", err)
	}
	fmt.Println("余额:", balance)

	// ===== 示例 2：获取用户信息 =====
	fmt.Println("\n=== 获取用户信息 ===")
	info, err := client.User.GetUserInfo(ctx)
	if err != nil {
		log.Fatal("获取用户信息失败:", err)
	}
	fmt.Printf("用户名: %s, 余额: %s\n", info.Username, info.Balance)

	// ===== 示例 3：获取项目列表 =====
	fmt.Println("\n=== 获取项目列表 ===")
	projects, err := client.User.GetProjects(ctx, 1, 50)
	if err != nil {
		log.Fatal("获取项目列表失败:", err)
	}
	fmt.Printf("共 %d 个项目\n", projects.Total)
	for _, p := range projects.List {
		fmt.Printf("  - [%s] %s (超时: %ds)\n", p.Code, p.Name, p.TimeoutSeconds)
	}

	// ===== 示例 4：一站式接码（最简便）=====
	fmt.Println("\n=== 一站式接码 ===")
	result, err := client.User.CreateAndWait(ctx, "twitter", &luckmail.OrderOptions{
		EmailType: "ms_graph",
		Timeout:   5 * time.Minute,
		Interval:  3 * time.Second,
		OnPoll: func(code *luckmail.OrderCode) {
			fmt.Printf("  轮询中... 状态: %s\n", code.Status)
		},
	})
	if err != nil {
		log.Printf("接码失败: %v\n", err)
	} else if result.Status == "success" {
		fmt.Printf("✅ 验证码: %s\n", result.VerificationCode)
		fmt.Printf("📧 来自: %s\n", result.MailFrom)
		fmt.Printf("📝 主题: %s\n", result.MailSubject)
	} else {
		fmt.Printf("❌ 接码失败: %s\n", result.Status)
	}

	// ===== 示例 5：分步接码（手动控制）=====
	fmt.Println("\n=== 分步接码 ===")
	order, err := client.User.CreateOrder(ctx, "facebook", nil)
	if err != nil {
		log.Printf("创建订单失败: %v\n", err)
	} else {
		fmt.Printf("订单号: %s, 邮箱: %s\n", order.OrderNo, order.EmailAddress)

		// 等待验证码
		code, err := client.User.WaitForCode(ctx, order.OrderNo, 300*time.Second, 3*time.Second, nil)
		if err != nil {
			log.Printf("等待验证码失败: %v\n", err)
		} else if code.Status == "success" {
			fmt.Printf("✅ 验证码: %s\n", code.VerificationCode)
		} else {
			// 超时取消订单
			client.User.CancelOrder(ctx, order.OrderNo)
			fmt.Printf("❌ 接码失败: %s\n", code.Status)
		}
	}

	// ===== 示例 6：购买邮箱 =====
	fmt.Println("\n=== 购买邮箱 ===")
	purchaseResult, err := client.User.PurchaseEmails(ctx, &luckmail.PurchaseEmailsRequest{
		ProjectCode: "twitter",
		Quantity:    3,
		EmailType:   "ms_graph",
	})
	if err != nil {
		log.Printf("购买邮箱失败: %v\n", err)
	} else {
		fmt.Printf("购买成功: %d 个邮箱, 总费用: %s, 剩余余额: %s\n",
			len(purchaseResult.Purchases),
			purchaseResult.TotalCost,
			purchaseResult.BalanceAfter,
		)
		for _, item := range purchaseResult.Purchases {
			fmt.Printf("  - %s (token: %s)\n", item.EmailAddress, item.Token)
		}
	}

	// ===== 示例 7：通过 Token 查询验证码 =====
	fmt.Println("\n=== Token 查询验证码 ===")
	aliveResult, err := client.User.CheckTokenAlive(ctx, "tok_abc123def456")
	if err != nil {
		log.Printf("Token 测活失败: %v\n", err)
	} else {
		fmt.Printf("alive=%v, message=%s, mail_count=%d\n", aliveResult.Alive, aliveResult.Message, aliveResult.MailCount)
	}
	tokenCode, err := client.User.WaitForTokenCode(ctx, "tok_abc123def456", 120*time.Second, 3*time.Second, nil)
	if err != nil {
		log.Printf("Token 查询失败: %v\n", err)
	} else if tokenCode.HasNewMail {
		fmt.Printf("✅ 验证码: %s\n", tokenCode.VerificationCode)
	} else {
		fmt.Println("未收到新邮件")
	}

	// ===== 示例 8：导入私有邮箱 =====
	fmt.Println("\n=== 导入私有邮箱 ===")
	importResult, err := client.User.ImportEmails(ctx, &luckmail.ImportEmailsRequest{
		Type: "ms_graph",
		Emails: []map[string]interface{}{
			{
				"address":       "user@outlook.com",
				"client_id":     "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
				"refresh_token": "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
			},
		},
	})
	if err != nil {
		log.Printf("导入邮箱失败: %v\n", err)
	} else {
		fmt.Printf("导入结果 - 成功: %d, 重复: %d, 失败: %d\n",
			importResult.Success, importResult.Duplicate, importResult.Failed)
	}

	// ===== 示例 9：提交申述 =====
	fmt.Println("\n=== 提交申述 ===")
	appealResult, err := client.User.CreateAppeal(ctx, &luckmail.CreateAppealRequest{
		AppealType:  1,
		OrderID:     123,
		Reason:      "no_code",
		Description: "等待 5 分钟未收到验证码",
	})
	if err != nil {
		log.Printf("提交申述失败: %v\n", err)
	} else {
		fmt.Printf("申述单号: %s\n", appealResult.AppealNo)
	}

	// ===== 示例 10：标签管理 =====
	fmt.Println("\n=== 标签管理 ===")

	// 创建标签
	tag, err := client.User.CreateTag(ctx, &luckmail.CreateTagRequest{
		Name:      "主力号",
		LimitType: 1,
		Remark:    "主力邮箱池，可下发",
	})
	if err != nil {
		log.Printf("创建标签失败: %v\n", err)
	} else {
		fmt.Printf("创建标签成功: ID=%d, 名称=%s\n", tag.ID, tag.Name)
	}

	// 获取所有标签
	tags, err := client.User.GetTags(ctx)
	if err != nil {
		log.Printf("获取标签列表失败: %v\n", err)
	} else {
		fmt.Printf("共 %d 个标签:\n", len(tags))
		for _, t := range tags {
			fmt.Printf("  [%d] %s (limit_type=%d, 邮箱数=%d)\n", t.ID, t.Name, t.LimitType, t.PurchaseCount)
		}
	}

	// 更新标签（注释掉避免误操作）
	// err = client.User.UpdateTag(ctx, "1", &luckmail.UpdateTagRequest{LimitType: 0, Name: "备用号"})

	// 删除标签（注释掉避免误操作）
	// err = client.User.DeleteTag(ctx, "主力号")

	// ===== 示例 11：已购邮箱标签和禁用管理 =====
	fmt.Println("\n=== 已购邮箱管理（标签 + 禁用）===")

	// 查看已购邮箱（支持更多筛选条件）
	purchases, err := client.User.GetPurchases(ctx, &luckmail.GetPurchasesParams{
		Page:            1,
		PageSize:        10,
		Keyword:         "outlook",
		HasUserDisabled: true,
		UserDisabled:    0, // 只看未禁用的
	})
	if err != nil {
		log.Printf("获取已购邮箱失败: %v\n", err)
	} else {
		fmt.Printf("已购邮箱（共 %d 个）:\n", purchases.Total)
		for _, item := range purchases.List {
			tagName := item.TagName
			if tagName == "" {
				tagName = "无"
			}
			fmt.Printf("  [%d] %s 标签:%s 禁用:%d\n", item.ID, item.EmailAddress, tagName, item.UserDisabled)
		}
	}

	// 设置单个邮箱标签（注释掉避免误操作）
	// err = client.User.SetPurchaseTag(ctx, 1, &luckmail.SetPurchaseTagRequest{TagName: "主力号"})

	// 批量设置标签（注释掉避免误操作）
	// err = client.User.BatchSetPurchaseTag(ctx, &luckmail.BatchSetPurchaseTagRequest{
	//     IDs:     []int{1, 2, 3},
	//     TagName: "主力号",
	// })

	// 禁用单个邮箱（注释掉避免误操作）
	// err = client.User.SetPurchaseDisabled(ctx, 1, 1)

	// 批量禁用（注释掉避免误操作）
	// err = client.User.BatchSetPurchaseDisabled(ctx, []int{1, 2, 3}, 1)

	// ===== 示例 12：按标签获取已购邮箱（API 下发）=====
	fmt.Println("\n=== 按标签获取已购邮箱（API 下发）===")
	apiItems, err := client.User.APIGetPurchases(ctx, &luckmail.APIGetPurchasesRequest{
		Count:       5,
		TagName:     "主力号",
		MarkTagName: "已使用",
	})
	if err != nil {
		log.Printf("获取已购邮箱失败: %v\n", err)
	} else {
		fmt.Printf("获取到 %d 个邮箱:\n", len(apiItems))
		for _, item := range apiItems {
			fmt.Printf("  %s | token: %s | 新标签: %s\n", item.EmailAddress, item.Token, item.TagName)
		}
	}

	// ===== 示例 13：通过 Token 获取邮件列表和详情 =====
	fmt.Println("\n=== Token 邮件列表 & 详情 ===")
	tokenForMails := "tok_abc123def456" // 替换为实际的已购邮箱 token
	mailList, err := client.User.GetTokenMails(ctx, tokenForMails)
	if err != nil {
		log.Printf("获取邮件列表失败: %v\n", err)
	} else {
		fmt.Printf("邮箱: %s, 项目: %s\n", mailList.EmailAddress, mailList.Project)
		fmt.Printf("保修截止: %s\n", mailList.WarrantyUntil)
		fmt.Printf("邮件数量: %d\n", len(mailList.Mails))
		for _, m := range mailList.Mails {
			fmt.Printf("  [%s] %s: %s\n", m.ReceivedAt, m.From, m.Subject)
		}

		// 获取第一封邮件的详情
		if len(mailList.Mails) > 0 {
			firstMail := mailList.Mails[0]
			detail, detailErr := client.User.GetTokenMailDetail(ctx, tokenForMails, firstMail.MessageID)
			if detailErr != nil {
				log.Printf("获取邮件详情失败: %v\n", detailErr)
			} else {
				fmt.Printf("\n📧 邮件详情:\n")
				fmt.Printf("  发件人: %s\n", detail.From)
				fmt.Printf("  收件人: %s\n", detail.To)
				fmt.Printf("  主题: %s\n", detail.Subject)
				if detail.VerificationCode != "" {
					fmt.Printf("  ✅ 验证码: %s\n", detail.VerificationCode)
				}
			}
		}
	}
}
