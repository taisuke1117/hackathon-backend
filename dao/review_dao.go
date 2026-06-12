package dao

import (
	"database/sql"
	"main/model"
)

type ReviewDao struct {
	DB *sql.DB
}

func NewReviewDao(db *sql.DB) *ReviewDao {
	return &ReviewDao{DB: db}
}

// Create 受取評価を登録。発送済み商品の購入者だけが出品者を評価できる
// 戻り値: 出品者ID, 商品名（通知用）
func (d *ReviewDao) Create(reviewerId string, productId int64, rating int, comment string) (string, string, error) {
	var sellerId, status, name string
	var buyerId sql.NullString
	err := d.DB.QueryRow(
		"SELECT seller_id, status, name, buyer_id FROM products WHERE product_id = ?", productId).
		Scan(&sellerId, &status, &name, &buyerId)
	if err == sql.ErrNoRows {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", err
	}
	if !buyerId.Valid || buyerId.String != reviewerId {
		return "", "", ErrNotPurchased
	}
	if status != "shipped" {
		return "", "", ErrConflict
	}

	_, err = d.DB.Exec(`
		INSERT INTO reviews (product_id, reviewer_id, reviewee_id, rating, comment)
		VALUES (?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE rating = VALUES(rating), comment = VALUES(comment)`,
		productId, reviewerId, sellerId, rating, comment)
	if err != nil {
		return "", "", err
	}
	return sellerId, name, nil
}

// ListByReviewee あるユーザーが受け取った評価一覧（プロフィールページ用）
func (d *ReviewDao) ListByReviewee(userId string) ([]model.Review, error) {
	rows, err := d.DB.Query(`
		SELECT r.review_id, r.product_id, p.name, r.reviewer_id, u.name, COALESCE(u.icon_url, ''),
		       r.reviewee_id, r.rating, COALESCE(r.comment, ''), r.created_at
		FROM reviews r
		JOIN products p ON p.product_id = r.product_id
		JOIN users u ON u.user_id = r.reviewer_id
		WHERE r.reviewee_id = ?
		ORDER BY r.created_at DESC LIMIT 100`, userId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]model.Review, 0)
	for rows.Next() {
		var rv model.Review
		if err := rows.Scan(&rv.ReviewId, &rv.ProductId, &rv.ProductName, &rv.ReviewerId, &rv.ReviewerName, &rv.ReviewerIcon,
			&rv.RevieweeId, &rv.Rating, &rv.Comment, &rv.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, rv)
	}
	return list, rows.Err()
}

// FindByProductAndReviewer 商品×評価者の評価を1件取得（評価済みかの判定用）
func (d *ReviewDao) FindByProductAndReviewer(productId int64, reviewerId string) (*model.Review, error) {
	var rv model.Review
	err := d.DB.QueryRow(`
		SELECT review_id, product_id, reviewer_id, reviewee_id, rating, COALESCE(comment, ''), created_at
		FROM reviews WHERE product_id = ? AND reviewer_id = ?`, productId, reviewerId).
		Scan(&rv.ReviewId, &rv.ProductId, &rv.ReviewerId, &rv.RevieweeId, &rv.Rating, &rv.Comment, &rv.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rv, nil
}
