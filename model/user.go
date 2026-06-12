package model

type User struct {
	UserId          string  `json:"user_id"`
	Name            string  `json:"name"`
	Mail            string  `json:"mail"`
	Place           string  `json:"place"`
	IconUrl         string  `json:"icon_url"`
	Bio             string  `json:"bio"`
	ShippingAddress string  `json:"shipping_address"`
	TotalSales      int     `json:"total_sales"`
	Rating          float64 `json:"rating"`
	ReviewCount     int     `json:"review_count"`
}

// RegisterUserRequest 初回ログイン時のユーザー登録ボディ
type RegisterUserRequest struct {
	Name    string `json:"name"`
	Mail    string `json:"mail"`
	IconUrl string `json:"icon_url"`
}

// UpdateUserRequest プロフィール更新ボディ
type UpdateUserRequest struct {
	Name            string `json:"name"`
	Place           string `json:"place"`
	IconUrl         string `json:"icon_url"`
	Bio             string `json:"bio"`
	ShippingAddress string `json:"shipping_address"`
}
