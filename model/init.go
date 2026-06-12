package model

import "time"

// AppInitResponse アプリ起動時に一括で返すオブジェクト
type AppInitResponse struct {
	User            *User          `json:"user"`
	Categories      []Category     `json:"categories"`
	Notifications   []Notification `json:"notifications"`
	LikedProductIds []int64        `json:"liked_product_ids"`
}

type Category struct {
	CategoryId int64  `json:"category_id"`
	Name       string `json:"name"`
}

type Notification struct {
	NotificationId int64  `json:"notification_id"`
	UserId         string `json:"user_id"`
	Type           string `json:"type"`
	Title          string `json:"title"`
	Content        string `json:"content"`
	LinkUrl        string `json:"link_url"`
	IsRead         bool      `json:"is_read"`
	CreatedAt      time.Time `json:"created_at"`
}
