// 供应商端 API 使用示例
package main

import (
	"context"
	"fmt"
	"log"

	luckmail "github.com/TryHarder-L/luckmail/luckmail"
)

func main() {
	// ===== 初始化客户端 =====
	client := luckmail.New(
		"your_supplier_api_key_here",
	)

	ctx := context.Background()

	// ===== 示例 1：获取供应商信息 =====
	fmt.Println("=== 获取供应商信息 ===")
	profile, err := client.Supplier.GetProfile(ctx)
	if err != nil {
		log.Fatal("获取供应商信息失败:", err)
	}
	fmt.Printf("用户名: %s\n", profile.Username)
	fmt.Printf("可用余额: %s\n", profile.Balance)
	fmt.Printf("冻结余额: %s\n", profile.FrozenBalance)
	fmt.Printf("接码佣金率: %s\n", profile.CodeCommissionRate)
	fmt.Printf("购买佣金率: %s\n", profile.BuyCommissionRate)

	// ===== 示例 2：查看数据看板 =====
	fmt.Println("\n=== 数据看板 ===")
	summary, err := client.Supplier.GetDashboard(ctx)
	if err != nil {
		log.Fatal("获取看板数据失败:", err)
	}
	fmt.Printf("总邮箱: %d 个\n", summary.TotalEmails)
	fmt.Printf("活跃邮箱: %d 个\n", summary.ActiveEmails)
	fmt.Printf("成功率: %.2f%%\n", summary.SuccessRate)
	fmt.Printf("今日接码: %d 次\n", summary.TodayAssigned)
	fmt.Printf("今日成功: %d 次\n", summary.TodaySuccess)
	fmt.Printf("今日佣金: %s\n", summary.TodayCommission)
	fmt.Printf("累计佣金: %s\n", summary.TotalCommission)

	// ===== 示例 3：获取邮箱列表 =====
	fmt.Println("\n=== 邮箱列表 ===")
	emails, err := client.Supplier.GetEmails(ctx, &luckmail.GetSupplierEmailsParams{
		Page:        1,
		PageSize:    20,
		EmailType:   "ms_graph",
		IsShortTerm: 0, // 长效邮箱
	})
	if err != nil {
		log.Printf("获取邮箱列表失败: %v\n", err)
	} else {
		fmt.Printf("长效 MS Graph 邮箱: %d 个\n", emails.Total)
		for _, email := range emails.List {
			fmt.Printf("  - %s (状态: %d, 成功率: %d/%d)\n",
				email.Address, email.Status, email.SuccessCount, email.TotalUsed)
		}
	}

	// ===== 示例 4：导入邮箱 =====
	fmt.Println("\n=== 导入邮箱 ===")
	importResult, err := client.Supplier.ImportEmails(ctx, &luckmail.ImportSupplierEmailsRequest{
		Type:        "ms_graph",
		IsShortTerm: 0,
		Emails: []map[string]interface{}{
			{
				"address":       "user1@outlook.com",
				"client_id":     "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
				"refresh_token": "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
			},
			{
				"address":       "user2@hotmail.com",
				"client_id":     "yyyyyyyy-yyyy-yyyy-yyyy-yyyyyyyyyyyy",
				"refresh_token": "yyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy",
			},
		},
	})
	if err != nil {
		log.Printf("导入邮箱失败: %v\n", err)
	} else {
		fmt.Printf("导入结果 - 成功: %d, 重复: %d, 失败: %d\n",
			importResult.Success, importResult.Duplicate, importResult.Failed)
	}

	// ===== 示例 5：查看待处理申述 =====
	fmt.Println("\n=== 待处理申述 ===")
	appeals, err := client.Supplier.GetAppeals(ctx, &luckmail.GetAppealsParams{
		Status: 1, // 待处理
	})
	if err != nil {
		log.Printf("获取申述列表失败: %v\n", err)
	} else {
		fmt.Printf("待处理申述: %d 个\n", appeals.Total)
		for _, appeal := range appeals.List {
			fmt.Printf("  - [%s] 订单: %s, 原因: %s\n",
				appeal.AppealNo, appeal.OrderNo, appeal.Reason)
		}
	}

	// ===== 示例 6：处理单个申述 =====
	fmt.Println("\n=== 处理申述 ===")
	if len(appeals.List) > 0 {
		appealNo := appeals.List[0].AppealNo

		// 查看申述详情
		detail, err := client.Supplier.GetAppeal(ctx, appealNo)
		if err != nil {
			log.Printf("获取申述详情失败: %v\n", err)
		} else {
			fmt.Printf("申述单号: %s\n", detail.AppealNo)
			fmt.Printf("申述原因: %s\n", detail.Reason)
			fmt.Printf("申述状态: %d\n", detail.Status)
		}

		// 同意退款
		err = client.Supplier.ReplyAppeal(ctx, appealNo, 1, "邮箱确有问题，同意退款")
		if err != nil {
			log.Printf("处理申述失败: %v\n", err)
		} else {
			fmt.Printf("申述 %s 处理成功（同意退款）\n", appealNo)
		}
	}

	// ===== 示例 7：批量处理申述 =====
	fmt.Println("\n=== 批量处理申述 ===")
	batchResult, err := client.Supplier.BatchReplyAppeals(ctx, &luckmail.BatchReplyAppealsRequest{
		AppealNos: []string{"APL001", "APL002", "APL003"},
		Result:    2, // 拒绝申述
		Reply:     "经验证邮箱状态正常，拒绝申述",
	})
	if err != nil {
		log.Printf("批量处理申述失败: %v\n", err)
	} else {
		fmt.Printf("批量处理结果 - 成功: %d, 失败: %d\n", batchResult.Success, batchResult.Failed)
	}
}
