package model

import "time"

// ProductSummary 一覧表示用の商品（ホーム・マイページ・取引管理など）
type ProductSummary struct {
	ProductId       int64     `json:"product_id"`
	Name            string    `json:"name"`
	Price           int       `json:"price"`
	ImageUrl        string    `json:"image_url"`
	LikesCount      int       `json:"likes_count"`
	LikedByMe       bool      `json:"liked_by_me"`
	Status          string    `json:"status"` // available / negotiating / unshipped / shipped
	Category        string    `json:"category"`
	UnreadChatCount int       `json:"unread_chat_count"`
	CreatedAt       time.Time `json:"created_at"`
}

// ProductDetail 商品詳細ページ用
type ProductDetail struct {
	ProductId  int64      `json:"product_id"`
	SellerId   string     `json:"seller_id"`
	BuyerId    string     `json:"buyer_id"`
	Name       string     `json:"name"`
	Price      int        `json:"price"`
	Detail     string     `json:"detail"`
	Condition  string     `json:"condition"`
	Images     []string   `json:"images"`
	Categories []Category `json:"categories"`
	Tags       []string   `json:"tags"`
	ViewsCount int        `json:"views_count"`
	LikesCount int        `json:"likes_count"`
	LikedByMe  bool       `json:"liked_by_me"`
	Status     string     `json:"status"`
	Seller     *User      `json:"seller"`
	CreatedAt  time.Time  `json:"created_at"`
}

// SaveProductRequest 出品・編集の入力ボディ
type SaveProductRequest struct {
	Name        string   `json:"name"`
	Price       int      `json:"price"`
	Detail      string   `json:"detail"`
	Condition   string   `json:"condition"`
	CategoryIds []int64  `json:"category_ids"`
	Tags        []string `json:"tags"`
	ImageUrls   []string `json:"image_urls"`
}

// SearchCondition 商品一覧の検索条件
type SearchCondition struct {
	Keyword    string
	CategoryId int64
	MinPrice   int
	MaxPrice   int
}

// SellerSummary 取引・出品管理ダッシュボードのサマリー
type SellerSummary struct {
	TotalSales           int `json:"total_sales"`
	SoldCount            int `json:"sold_count"`
	ActiveChatCount      int `json:"active_chat_count"`
	CurrentListingCount  int `json:"current_listing_count"`
	PendingDeliveryCount int `json:"pending_delivery_count"`
}
