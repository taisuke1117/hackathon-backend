package gemini

import (
	"fmt"
	"strings"
)

// ChatMessageCtx Geminiへ渡すチャット履歴の1件
type ChatMessageCtx struct {
	IsMe    bool   `json:"is_me"`
	Content string `json:"content"`
}

// GenerateReply チャット返信文の生成。role: "buyer" or "seller"
func GenerateReply(role, productName, productDesc string, productPrice, discountProposed, discountApproved int, messages []ChatMessageCtx, instruction string) (string, error) {
	roleDesc := "フリマアプリで商品の購入を検討している購入希望者"
	otherRole := "出品者"
	if role == "seller" {
		roleDesc = "フリマアプリの出品者"
		otherRole = "購入希望者"
	}

	productInfo := fmt.Sprintf("商品名: %s\n価格: ¥%s", productName, formatPrice(productPrice))
	if productDesc != "" {
		productInfo += "\n商品説明: " + productDesc
	}

	discountInfo := ""
	if discountApproved > 0 {
		discountInfo = fmt.Sprintf("\n【値引き状況】¥%s への値引きを出品者が承認済み（購入者はこの価格で購入可能）",
			formatPrice(discountApproved))
	} else if discountProposed > 0 {
		discountInfo = fmt.Sprintf("\n【値引き状況】購入希望者が ¥%s を提示して交渉中（まだ承認されていない）",
			formatPrice(discountProposed))
	}

	historyStr := "（まだメッセージなし）"
	if len(messages) > 0 {
		start := 0
		if len(messages) > 10 {
			start = len(messages) - 10
		}
		lines := make([]string, 0, len(messages)-start)
		for _, m := range messages[start:] {
			speaker := "自分"
			if !m.IsMe {
				speaker = otherRole
			}
			lines = append(lines, speaker+": "+m.Content)
		}
		historyStr = strings.Join(lines, "\n")
	}

	prompt := fmt.Sprintf(`あなたは%sです。以下の状況を踏まえ、チャットに送る次のメッセージを1通だけ作成してください。

【商品情報】
%s%s

【これまでの会話（最新10件）】
%s

【あなたへの指示】
%s

条件: 日本語で自然かつ丁寧。会話の流れを踏まえた具体的な内容にする。前置きや説明一切不要でメッセージ本文だけを返す。150文字以内。`,
		roleDesc, productInfo, discountInfo, historyStr, instruction)

	return generate([]part{{Text: prompt}}, false)
}
