package gemini

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

var ErrNoAPIKey = errors.New("GEMINI_API_KEY is not set")

func model() string {
	if v := os.Getenv("GEMINI_MODEL"); v != "" {
		return v
	}
	return "gemini-flash-latest"
}

type part struct {
	Text       string      `json:"text,omitempty"`
	InlineData *inlineData `json:"inline_data,omitempty"`
}

type inlineData struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"`
}

type generateRequest struct {
	Contents []struct {
		Parts []part `json:"parts"`
	} `json:"contents"`
	GenerationConfig *struct {
		ResponseMimeType string `json:"response_mime_type,omitempty"`
	} `json:"generationConfig,omitempty"`
}

// generate Gemini APIを呼んで最初の候補テキストを返す。jsonMode=trueでJSON出力を強制
func generate(parts []part, jsonMode bool) (string, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return "", ErrNoAPIKey
	}

	var req generateRequest
	req.Contents = make([]struct {
		Parts []part `json:"parts"`
	}, 1)
	req.Contents[0].Parts = parts
	if jsonMode {
		req.GenerationConfig = &struct {
			ResponseMimeType string `json:"response_mime_type,omitempty"`
		}{ResponseMimeType: "application/json"}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	// 新形式のAPIキー(AQ.プレフィックス)はクエリパラメータでは認証できないため、
	// X-goog-api-key ヘッダーで渡す（旧AIza形式のキーもこのヘッダーで動く）
	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent",
		model())
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-goog-api-key", apiKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini api error (%d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", errors.New("gemini returned no candidates")
	}
	return result.Candidates[0].Content.Parts[0].Text, nil
}

// ListingResult 出品支援の自動生成結果
type ListingResult struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Categories  []string `json:"categories"`
	Tags        []string `json:"tags"`
	Price       int      `json:"price_suggestion"`
}

// AnalyzeListing 商品画像から商品名・説明文・カテゴリ・タグ・参考価格を生成する
func AnalyzeListing(imageURL string, categoryNames []string) (*ListingResult, error) {
	imgResp, err := http.Get(imageURL)
	if err != nil {
		return nil, fmt.Errorf("画像の取得に失敗: %w", err)
	}
	defer imgResp.Body.Close()
	imgBytes, err := io.ReadAll(io.LimitReader(imgResp.Body, 8<<20)) // 最大8MB
	if err != nil {
		return nil, err
	}
	mimeType := imgResp.Header.Get("Content-Type")
	if !strings.HasPrefix(mimeType, "image/") {
		mimeType = "image/jpeg"
	}

	prompt := fmt.Sprintf(`あなたはフリマアプリの出品アシスタントです。この商品画像を解析して、以下のJSONだけを返してください。
{"title": "魅力的な商品名(40文字以内)", "description": "丁寧な商品説明文(状態・特徴・おすすめポイントを改行区切りで)", "categories": ["次のリストから当てはまるものを1〜3個: %s"], "tags": ["検索用キーワードを3〜6個"], "price_suggestion": 想定中古販売価格の整数(円)}`,
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

	// 商品情報セクション
	productInfo := fmt.Sprintf("商品名: %s\n価格: ¥%s", productName, formatPrice(productPrice))
	if productDesc != "" {
		productInfo += "\n商品説明: " + productDesc
	}

	// 値引き交渉状況
	discountInfo := ""
	if discountApproved > 0 {
		discountInfo = fmt.Sprintf("\n【値引き状況】¥%s への値引きを出品者が承認済み（購入者はこの価格で購入可能）",
			formatPrice(discountApproved))
	} else if discountProposed > 0 {
		discountInfo = fmt.Sprintf("\n【値引き状況】購入希望者が ¥%s を提示して交渉中（まだ承認されていない）",
			formatPrice(discountProposed))
	}

	// 会話履歴（最新10件）
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

func formatPrice(p int) string {
	// カンマ区切り整数文字列
	s := fmt.Sprintf("%d", p)
	n := len(s)
	if n <= 3 {
		return s
	}
	result := ""
	for i, c := range s {
		if i > 0 && (n-i)%3 == 0 {
			result += ","
		}
		result += string(c)
	}
	return result
}

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
