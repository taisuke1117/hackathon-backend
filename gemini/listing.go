package gemini

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ListingResult 出品支援の自動生成結果
type ListingResult struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Condition   string   `json:"condition"`
	Categories  []string `json:"categories"`
	Tags        []string `json:"tags"`
	Price       int      `json:"price_suggestion"`
}

// AnalyzeListing 商品画像から出品情報一式を生成する
func AnalyzeListing(imageURL string, categoryNames []string) (*ListingResult, error) {
	imgResp, err := http.Get(imageURL)
	if err != nil {
		return nil, fmt.Errorf("画像の取得に失敗: %w", err)
	}
	defer imgResp.Body.Close()
	imgBytes, err := io.ReadAll(io.LimitReader(imgResp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	mimeType := imgResp.Header.Get("Content-Type")
	if !strings.HasPrefix(mimeType, "image/") {
		mimeType = "image/jpeg"
	}

	prompt := fmt.Sprintf(`あなたはフリマアプリ（メルカリのようなCtoCプラットフォーム）で自分の持ち物を出品しようとしているユーザーの代わりに出品情報を作成するアシスタントです。

この商品画像を解析して、実際のフリマアプリの出品ページにそのまま使える情報を以下のJSONだけで返してください。

{
  "title": "ブランド名・型番・サイズ・色などを含む魅力的な商品名（40文字以内）",
  "condition": "画像から判断して次の6段階から1つだけ選ぶ: 新品、未使用 / 未使用に近い / 目立った傷や汚れなし / やや傷や汚れあり / 傷や汚れあり / 全体的に状態が悪い",
  "description": "フリマアプリの説明文形式で以下のセクション構成にする（改行は\\nで表現）:\n【商品説明】\\n自宅で使用していた〇〇を出品します。〇〇などの特徴があります。\\n\\n【商品の状態】\\n（conditionと同じ状態を記載。目視できる傷・汚れ・使用感を具体的に）\\n\\n【配送について】\\n丁寧に梱包してお送りします。\\n\\n【注意事項】\\n・素人保管のため神経質な方はご遠慮ください\\n・ご不明点はコメントにてお気軽にどうぞ",
  "categories": ["次のカテゴリリストから最も当てはまるものを1〜2個: %s"],
  "tags": ["フリマ・オークションで実際に使われる検索キーワードを3〜6個"],
  "price_suggestion": フリマ・オークションサイトでの実際の中古取引相場を考慮した適切な販売価格（整数・円。新品定価ではなく中古市場価格を基準にする）
}`,
		strings.Join(categoryNames, ", "))

	text, err := generate([]part{
		{InlineData: &inlineData{MimeType: mimeType, Data: base64.StdEncoding.EncodeToString(imgBytes)}},
		{Text: prompt},
	}, true)
	if err != nil {
		return nil, err
	}

	var result ListingResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return nil, fmt.Errorf("生成結果のJSONパースに失敗: %w", err)
	}
	return &result, nil
}
