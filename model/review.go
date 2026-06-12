package model

import "time"

type Review struct {
	ReviewId     int64     `json:"review_id"`
	ProductId    int64     `json:"product_id"`
	ProductName  string    `json:"product_name"`
	ReviewerId   string    `json:"reviewer_id"`
	ReviewerName string    `json:"reviewer_name"`
	ReviewerIcon string    `json:"reviewer_icon"`
	RevieweeId   string    `json:"reviewee_id"`
	Rating       int       `json:"rating"`
	Comment      string    `json:"comment"`
	CreatedAt    time.Time `json:"created_at"`
}
