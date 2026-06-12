package model

import "time"

// ChatRoomSummary チャット一覧の1行分（商品・相手・最終メッセージをJOIN済み）
type ChatRoomSummary struct {
	ChatroomId       int64      `json:"chatroom_id"`
	ProductId        int64      `json:"product_id"`
	ProductName      string     `json:"product_name"`
	ProductImage     string     `json:"product_image"`
	ProductPrice     int        `json:"product_price"`
	ProductStatus    string     `json:"product_status"`
	OtherUserId      string     `json:"other_user_id"`
	OtherUserName    string     `json:"other_user_name"`
	OtherUserIcon    string     `json:"other_user_icon"`
	LastMessage      string     `json:"last_message"`
	LastMessageAt    *time.Time `json:"last_message_at"`
	UnreadCount      int        `json:"unread_count"`
	DiscountProposed int        `json:"discount_proposed"`
	DiscountApproved int        `json:"discount_approved"`
}

// ChatMessage メッセージ1件
type ChatMessage struct {
	ChatId    int64     `json:"chat_id"`
	SenderId  string    `json:"sender_id"`
	Content   string    `json:"content"`
	IsRead    bool      `json:"is_read"`
	CreatedAt time.Time `json:"created_at"`
}

// ChatRoomDetail ルーム詳細（メタ情報＋メッセージ一覧）
type ChatRoomDetail struct {
	ChatroomId       int64         `json:"chatroom_id"`
	ProductId        int64         `json:"product_id"`
	ProductName      string        `json:"product_name"`
	ProductImage     string        `json:"product_image"`
	ProductPrice     int           `json:"product_price"`
	ProductStatus    string        `json:"product_status"`
	SellerId         string        `json:"seller_id"`
	ProposerId       string        `json:"proposer_id"`
	OtherUserName    string        `json:"other_user_name"`
	OtherUserIcon    string        `json:"other_user_icon"`
	DiscountProposed int           `json:"discount_proposed"`
	DiscountApproved int           `json:"discount_approved"`
	Messages         []ChatMessage `json:"messages"`
}
