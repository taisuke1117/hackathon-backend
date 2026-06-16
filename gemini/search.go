package gemini

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SearchConditions: Geminiが自然文から変換した検索条件
type SearchConditions struct {
	Keyword    string `json:"keyword"`    // 検索キーワード（1〜2語）
	Category   string `json:"category"`   // カテゴリ名（マスターと一致するもの）
	MinPrice   int    `json:"min_price"`  // 下限価格（指定なしは0）
	MaxPrice   int    `json:"max_price"`  // 上限価格（指定なしは0）
	Suggestion string `json:"suggestion"` // ユーザーへのひとこと（UIに表示する）
}

// ParseSearchPrompt: 自然文をGeminiで構造化された検索条件に変換する
// categoryNames: 存在しないカテゴリ名を勝手に生成させないためマスター一覧を渡す
func ParseSearchPrompt(userPrompt string, categoryNames []string) (*SearchConditions, error) {
	prompt := fmt.Sprintf(`あなたはフリマアプリの検索アシスタントです。ユーザーの要望を検索条件に変換して、以下のJSONだけを返してください。
{"keyword": "商品検索に使う最重要キーワード(1〜2語)", "category": "次のリストから1つ。該当なしなら空文字: %s", "min_price": 最低価格の整数(指定なしは0), "max_price": 最高価格の整数(指定なしは0), "suggestion": "ユーザーへのひとこと提案(50文字以内)"}
ユーザーの要望: %s`,
		strings.Join(categoryNames, ", "), userPrompt)

	// true = JSONモード（コードブロックなしの純粋なJSONを返させる）
	text, err := generate([]part{{Text: prompt}}, true)
	if err != nil {
		return nil, err
	}

	var result SearchConditions
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return nil, fmt.Errorf("生成結果のJSONパースに失敗: %w", err)
	}
	return &result, nil
}
