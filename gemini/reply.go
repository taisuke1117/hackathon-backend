package gemini

import (
	"fmt"
	"strings"
)

// ChatMessageCtx: Geminiに渡すチャット履歴の1メッセージ
type ChatMessageCtx struct {
	IsMe    bool   `json:"is_me"`    // true=自分のメッセージ、false=相手のメッセージ
	Content string `json:"content"`
}

// GenerateReply: チャットでの次の返信メッセージをGeminiで自動生成する
//
// 使い方: チャット画面に「Gemini返信提案」ボタンがあり、押すとこれが呼ばれる
//
// 引数:
//   role             - "buyer"（購入希望者）or "seller"（出品者）。誰として返信するか
//   productName      - 商品名（文脈に含める）
//   productDesc      - 商品説明
//   productPrice     - 定価
//   discountProposed - 値引き提案中の金額（0なら提案なし）
//   discountApproved - 承認済みの値引き金額（0なら未承認）
//   messages         - これまでのチャット履歴（最新10件を使う）
//   instruction      - ユーザーが入力した「こんな感じで返して」という指示
func GenerateReply(role, productName, productDesc string, productPrice, discountProposed, discountApproved int, messages []ChatMessageCtx, instruction string) (string, error) {
	// roleに応じてGeminiへの人格設定と相手の呼び方を決める
	roleDesc := "フリマアプリで商品の購入を検討している購入希望者"
	otherRole := "出品者"
	if role == "seller" {
		roleDesc = "フリマアプリの出品者"
		otherRole = "購入希望者"
	}

	// 商品情報の文字列を組み立て
	productInfo := fmt.Sprintf("商品名: %s\n価格: ¥%s", productName, formatPrice(productPrice))
	if productDesc != "" {
		productInfo += "\n商品説明: " + productDesc
	}

	// 値引き状況を文字列にする（なければ空文字）
	discountInfo := ""
	if discountApproved > 0 {
		discountInfo = fmt.Sprintf("\n【値引き状況】¥%s への値引きを出品者が承認済み（購入者はこの価格で購入可能）",
			formatPrice(discountApproved))
	} else if discountProposed > 0 {
		discountInfo = fmt.Sprintf("\n【値引き状況】購入希望者が ¥%s を提示して交渉中（まだ承認されていない）",
			formatPrice(discountProposed))
	}

	// 履歴の文字列化（最新10件だけ使う。多すぎるとプロンプトが長くなりすぎる）
	historyStr := "（まだメッセージなし）"
	if len(messages) > 0 {
		start := 0
		if len(messages) > 10 {
			start = len(messages) - 10 // 末尾10件だけ使う
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

	// Geminiへのプロンプト
	// 「メッセージ本文だけを返す」「150文字以内」と明示して余計な出力を防ぐ
	prompt := fmt.Sprintf(`あなたは%sです。以下の状況を踏まえ、チャットに送る次のメッセージを1通だけ作成してください。

【商品情報】
%s%s

【これまでの会話（最新10件）】
%s

【あなたへの指示】
%s

条件: 日本語で自然かつ丁寧。会話の流れを踏まえた具体的な内容にする。前置きや説明一切不要でメッセージ本文だけを返す。150文字以内。`,
		roleDesc, productInfo, discountInfo, historyStr, instruction)

	// false = プレーンテキストで返す（JSONモードではない）
	return generate([]part{{Text: prompt}}, false)
}
