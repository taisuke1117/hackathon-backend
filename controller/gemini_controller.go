package controller

import (
	"errors"
	"main/dao"
	"main/gemini"
	"net/http"
)

type GeminiController struct {
	userDao *dao.UserDao
}

func NewGeminiController(userDao *dao.UserDao) *GeminiController {
	return &GeminiController{userDao: userDao}
}

func (c *GeminiController) categoryNames() []string {
	categories, err := c.userDao.GetAllCategories()
	if err != nil {
		return []string{}
	}
	names := make([]string, 0, len(categories))
	for _, cat := range categories {
		names = append(names, cat.Name)
	}
	return names
}

func handleGeminiError(w http.ResponseWriter, err error) {
	if errors.Is(err, gemini.ErrNoAPIKey) {
		writeError(w, http.StatusServiceUnavailable, "Gemini APIキーが未設定です（GEMINI_API_KEY）")
		return
	}
	writeError(w, http.StatusBadGateway, "AI生成に失敗しました: "+err.Error())
}

// AnalyzeListing POST /api/gemini/listing {image_url} — 出品情報の自動生成
func (c *GeminiController) AnalyzeListing(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ImageUrl string `json:"image_url"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.ImageUrl == "" {
		writeError(w, http.StatusBadRequest, "image_urlは必須です")
		return
	}
	result, err := gemini.AnalyzeListing(req.ImageUrl, c.categoryNames())
	if err != nil {
		handleGeminiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// GenerateReply POST /api/gemini/reply — チャット返信文生成
func (c *GeminiController) GenerateReply(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Role             string                 `json:"role"` // "buyer" or "seller"
		ProductName      string                 `json:"product_name"`
		ProductDesc      string                 `json:"product_description"`
		ProductPrice     int                    `json:"product_price"`
		DiscountProposed int                    `json:"discount_proposed"`
		DiscountApproved int                    `json:"discount_approved"`
		Messages         []gemini.ChatMessageCtx `json:"messages"`
		Instruction      string                 `json:"instruction"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Instruction == "" {
		writeError(w, http.StatusBadRequest, "instructionは必須です")
		return
	}
	text, err := gemini.GenerateReply(
		req.Role, req.ProductName, req.ProductDesc,
		req.ProductPrice, req.DiscountProposed, req.DiscountApproved,
		req.Messages, req.Instruction,
	)
	if err != nil {
		handleGeminiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"text": text})
}

// AiSearch POST /api/gemini/search {prompt} — 自然文を検索条件に変換
func (c *GeminiController) AiSearch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prompt string `json:"prompt"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "promptは必須です")
		return
	}
	conditions, err := gemini.ParseSearchPrompt(req.Prompt, c.categoryNames())
	if err != nil {
		handleGeminiError(w, err)
		return
	}
	// カテゴリ名をIDに解決して返す（フロントがそのまま商品検索APIを叩けるように）
	var categoryId int64
	if conditions.Category != "" {
		if categories, err := c.userDao.GetAllCategories(); err == nil {
			for _, cat := range categories {
				if cat.Name == conditions.Category {
					categoryId = cat.CategoryId
					break
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"keyword":     conditions.Keyword,
		"category":    conditions.Category,
		"category_id": categoryId,
		"min_price":   conditions.MinPrice,
		"max_price":   conditions.MaxPrice,
		"suggestion":  conditions.Suggestion,
	})
}
