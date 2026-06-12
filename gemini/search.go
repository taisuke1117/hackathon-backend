package gemini

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SearchConditions AI検索: 自然文から検索条件を抽出
type SearchConditions struct {
	Keyword    string `json:"keyword"`
	Category   string `json:"category"`
	MinPrice   int    `json:"min_price"`
	MaxPrice   int    `json:"max_price"`
	Suggestion string `json:"suggestion"`
}

// ParseSearchPrompt 「20代男性に似合う1万円以内の黒い夏服」のような文を検索条件JSONへ変換
func ParseSearchPrompt(userPrompt string, categoryNames []string) (*SearchConditions, error) {
	prompt := fmt.Sprintf(`あなたはフリマアプリの検索アシスタントです。ユーザーの要望を検索条件に変換して、以下のJSONだけを返してください。
{"keyword": "商品検索に使う最重要キーワード(1〜2語)", "category": "次のリストから1つ。該当なしなら空文字: %s", "min_price": 最低価格の整数(指定なしは0), "max_price": 最高価格の整数(指定なしは0), "suggestion": "ユーザーへのひとこと提案(50文字以内)"}
ユーザーの要望: %s`,
		strings.Join(categoryNames, ", "), userPrompt)

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
