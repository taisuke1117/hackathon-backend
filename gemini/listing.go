package gemini

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ListingResult: Geminiが返す出品情報の自動生成結果
// controllerがこれをJSONでフロントに返す
type ListingResult struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Condition   string   `json:"condition"`
	Categories  []string `json:"categories"`   // カテゴリ名の配列（マスターと照合して使う）
	Tags        []string `json:"tags"`
	Price       int      `json:"price_suggestion"` // 中古相場に基づく推奨価格
}

// AnalyzeListing: 商品画像のURLを受け取り、Geminiで出品情報を自動生成する
//
// 処理の流れ:
//   1. 画像URLから画像バイナリを取得
//   2. Base64エンコードしてGeminiにマルチモーダルリクエストを送る
//   3. Geminiが JSON形式で商品名・説明・状態・カテゴリ・タグ・価格を返す
//   4. JSONをパースして ListingResult を返す
//
// フロントはこの結果で出品フォームのフィールドを自動入力する
func AnalyzeListing(imageURL string, categoryNames []string) (*ListingResult, error) {
	// ── 画像を取得 ──────────────────────────────────────────────
	// フロントが先にGCSに画像をアップロードして、そのURLをここに渡してくる
	imgResp, err := http.Get(imageURL)
	if err != nil {
		return nil, fmt.Errorf("画像の取得に失敗: %w", err)
	}
	defer imgResp.Body.Close()

	// 最大8MB（8<<20 = 8 * 2^20）まで読み込む
	imgBytes, err := io.ReadAll(io.LimitReader(imgResp.Body, 8<<20))
	if err != nil {
		return nil, err
	}

	// MIMEタイプを取得（jpeg/png/webpなど）。不明なら jpeg と仮定する
	mimeType := imgResp.Header.Get("Content-Type")
	if !strings.HasPrefix(mimeType, "image/") {
		mimeType = "image/jpeg"
	}

	// ── Geminiへのプロンプトを作成 ────────────────────────────
	// categoryNames: マスターのカテゴリ名一覧をプロンプトに埋め込み、
	//                Geminiがそこから選ぶよう指示する
	prompt := fmt.Sprintf(`あなたはフリマアプリ（メルカリのようなCtoCプラットフォーム）で自分の持ち物を出品しようとしているユーザーの代わりに出品情報を作成するアシスタントです。

この商品画像を解析して、実際のフリマアプリの出品ページにそのまま使える情報を以下のJSONだけで返してください。

{
  "title": "ブランド名・型番・サイズ・色などを含む魅力的な商品名（40文字以内）",
  "condition": "画像から判断して次の6段階から1つだけ選ぶ: 新品、未使用 / 未使用に近い / 目立った傷や汚れなし / やや傷や汚れあり / 傷や汚れあり / 全体的に状態が悪い",
  "description": "フリマアプリの説明文形式で以下のセクション構成で書く（改行は\nで表現）:\n【商品説明】\n自宅で使用していた〇〇を出品します。〇〇などの特徴があります。\n\n【商品の状態】\n（conditionと同じ状態を記載。目視できる傷・汚れ・使用感を具体的に）\n\n【配送について】\n丁寧に梱包してお送りします。\n\n【注意事項】\n・素人保管のため神経質な方はご遠慮ください\n・ご不明点はコメントにてお気軽にどうぞ",
  "categories": ["次のカテゴリリストから最も当てはまるものを1〜2個: %s"],
  "tags": ["フリマ・オークションで実際に使われる検索キーワードを3〜6個"],
  "price_suggestion": フリマ・オークションサイトでの実際の中古取引相場を考慮した適切な販売価格（整数・円。新品定価ではなく中古市場価格を基準にする）
}`,
		strings.Join(categoryNames, ", "))

	// ── Geminiにリクエストを送る ──────────────────────────────
	// テキストと画像の両方を渡すマルチモーダルリクエスト
	text, err := generate([]part{
		// InlineData: 画像をBase64でそのまま埋め込む（URLではなくバイナリ）
		{InlineData: &inlineData{MimeType: mimeType, Data: base64.StdEncoding.EncodeToString(imgBytes)}},
		// Text: 上で作ったプロンプト
		{Text: prompt},
	}, true) // true = JSONモード（Geminiに純粋なJSONだけ返させる）
	if err != nil {
		return nil, err
	}

	// ── Geminiの返答をJSONパース ──────────────────────────────
	var result ListingResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return nil, fmt.Errorf("生成結果のJSONパースに失敗: %w", err)
	}
	// Geminiがリテラルの \n（2文字）を出力した場合も実際の改行に統一する
	result.Description = strings.ReplaceAll(result.Description, `\n`, "\n")
	return &result, nil
}
