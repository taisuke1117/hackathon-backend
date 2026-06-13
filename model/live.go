package model

import "time"

type LiveRoom struct {
	RoomId      int64     `json:"room_id"`
	SellerId    string    `json:"seller_id"`
	SellerName  string    `json:"seller_name"`
	SellerIcon  string    `json:"seller_icon"`
	Title       string    `json:"title"`
	Status      string    `json:"status"` // scheduled / live / ended
	ViewerCount int       `json:"viewer_count"`
	CreatedAt   time.Time `json:"created_at"`
}

type LiveProduct struct {
	Id             int64      `json:"id"`
	RoomId         int64      `json:"room_id"`
	ProductId      int64      `json:"product_id"`
	ProductName    string     `json:"product_name"`
	ProductImage   string     `json:"product_image"`
	Position       int        `json:"position"`
	Mode           string     `json:"mode"` // auction / instant
	StartPrice     int        `json:"start_price"`
	InstantPrice   *int       `json:"instant_price,omitempty"`
	CurrentPrice   int        `json:"current_price"`
	CurrentBidder  string     `json:"current_bidder,omitempty"`
	BidderName     string     `json:"bidder_name,omitempty"`
	AuctionEndAt   *time.Time `json:"auction_end_at,omitempty"`
	SecondsLeft    int        `json:"seconds_left,omitempty"`
	Status         string     `json:"status"` // waiting / active / sold / skipped
	BuyerId        string     `json:"buyer_id,omitempty"`
}

type LiveRoomDetail struct {
	LiveRoom
	CurrentProduct *LiveProduct  `json:"current_product"`
	Queue          []LiveProduct `json:"queue"` // waiting以降の商品一覧
}

// LiveEvent: SSEでブロードキャストするイベント
type LiveEvent struct {
	Type string `json:"type"` // bid / tick / sold / next / end / comment / viewer_count
	// bid / sold / next
	Product *LiveProduct `json:"product,omitempty"`
	// tick
	SecondsLeft int `json:"seconds_left,omitempty"`
	// sold
	FinalPrice int    `json:"final_price,omitempty"`
	BuyerName  string `json:"buyer_name,omitempty"`
	// comment
	UserName string `json:"user_name,omitempty"`
	Content  string `json:"content,omitempty"`
	// viewer_count
	ViewerCount int `json:"viewer_count,omitempty"`
}

// フロントから送るリクエスト型
type CreateLiveRoomRequest struct {
	Title    string              `json:"title"`
	Products []LiveProductInput  `json:"products"`
}

type LiveProductInput struct {
	ProductId    int64  `json:"product_id"`
	Mode         string `json:"mode"`         // auction / instant
	StartPrice   int    `json:"start_price"`
	InstantPrice *int   `json:"instant_price,omitempty"`
}
