package dao

import (
	"database/sql"
	"encoding/json"
	"errors"
	"main/model"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrForbidden    = errors.New("forbidden")
	ErrConflict     = errors.New("conflict")
	ErrOwnProduct   = errors.New("cannot buy own product")
	ErrNotPurchased = errors.New("not purchased by you")
)

type ProductDao struct {
	DB *sql.DB
}

func NewProductDao(db *sql.DB) *ProductDao {
	return &ProductDao{DB: db}
}

func tagsToJSON(tags []string) string {
	if len(tags) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(tags)
	return string(b)
}

func tagsFromJSON(s string) []string {
	tags := []string{}
	_ = json.Unmarshal([]byte(s), &tags)
	return tags
}

// Search 出品中の商品を条件検索（キーワードは商品名・説明・タグに対してLIKE）
func (d *ProductDao) Search(viewerId string, c *model.SearchCondition) ([]model.ProductSummary, error) {
	query := `
		SELECT p.product_id, p.name, p.price, COALESCE(p.image_url, ''), p.likes_count, p.status, p.created_at,
		       COALESCE((SELECT c2.name FROM product_categories pc JOIN categories c2 ON c2.category_id = pc.category_id
		                 WHERE pc.product_id = p.product_id ORDER BY c2.category_id LIMIT 1), ''),
		       EXISTS(SELECT 1 FROM likes l WHERE l.product_id = p.product_id AND l.user_id = ?)
		FROM products p
		WHERE p.status = 'available'
		  AND (? = '' OR p.name LIKE CONCAT('%', ?, '%') OR p.detail LIKE CONCAT('%', ?, '%') OR COALESCE(p.tags,'') LIKE CONCAT('%', ?, '%'))
		  AND (? = 0 OR EXISTS(SELECT 1 FROM product_categories pc2 WHERE pc2.product_id = p.product_id AND pc2.category_id = ?))
		  AND (? = 0 OR p.price >= ?)
		  AND (? = 0 OR p.price <= ?)
		ORDER BY p.created_at DESC
		LIMIT 100`

	rows, err := d.DB.Query(query,
		viewerId,
		c.Keyword, c.Keyword, c.Keyword, c.Keyword,
		c.CategoryId, c.CategoryId,
		c.MinPrice, c.MinPrice,
		c.MaxPrice, c.MaxPrice)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSummaries(rows)
}

func scanSummaries(rows *sql.Rows) ([]model.ProductSummary, error) {
	list := make([]model.ProductSummary, 0)
	for rows.Next() {
		var p model.ProductSummary
		if err := rows.Scan(&p.ProductId, &p.Name, &p.Price, &p.ImageUrl, &p.LikesCount, &p.Status, &p.CreatedAt, &p.Category, &p.LikedByMe); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

// FindDetail 商品詳細（画像・カテゴリ・タグ・出品者込み）。閲覧のたびviews_countを+1
func (d *ProductDao) FindDetail(viewerId string, productId int64) (*model.ProductDetail, error) {
	var p model.ProductDetail
	var tagsJSON string
	err := d.DB.QueryRow(`
		SELECT p.product_id, p.seller_id, COALESCE(p.buyer_id, ''), p.name, p.price, COALESCE(p.detail, ''),
		       COALESCE(p.tags, '[]'), p.views_count, p.likes_count, p.status, p.created_at,
		       EXISTS(SELECT 1 FROM likes l WHERE l.product_id = p.product_id AND l.user_id = ?)
		FROM products p WHERE p.product_id = ?`, viewerId, productId).Scan(
		&p.ProductId, &p.SellerId, &p.BuyerId, &p.Name, &p.Price, &p.Detail,
		&tagsJSON, &p.ViewsCount, &p.LikesCount, &p.Status, &p.CreatedAt, &p.LikedByMe)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.Tags = tagsFromJSON(tagsJSON)

	// 画像一覧
	p.Images = []string{}
	imgRows, err := d.DB.Query("SELECT url FROM product_images WHERE product_id = ? ORDER BY position", productId)
	if err != nil {
		return nil, err
	}
	defer imgRows.Close()
	for imgRows.Next() {
		var u string
		if err := imgRows.Scan(&u); err != nil {
			return nil, err
		}
		p.Images = append(p.Images, u)
	}

	// カテゴリ一覧
	p.Categories = []model.Category{}
	catRows, err := d.DB.Query(`
		SELECT c.category_id, c.name FROM product_categories pc
		JOIN categories c ON c.category_id = pc.category_id
		WHERE pc.product_id = ? ORDER BY c.category_id`, productId)
	if err != nil {
		return nil, err
	}
	defer catRows.Close()
	for catRows.Next() {
		var c model.Category
		if err := catRows.Scan(&c.CategoryId, &c.Name); err != nil {
			return nil, err
		}
		p.Categories = append(p.Categories, c)
	}

	// 閲覧数インクリメント（失敗しても致命的ではないのでエラーは無視）
	_, _ = d.DB.Exec("UPDATE products SET views_count = views_count + 1 WHERE product_id = ?", productId)

	return &p, nil
}

// Create 出品。商品本体・画像・カテゴリをトランザクションで一括登録
func (d *ProductDao) Create(sellerId string, req *model.SaveProductRequest) (int64, error) {
	tx, err := d.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	mainImage := ""
	if len(req.ImageUrls) > 0 {
		mainImage = req.ImageUrls[0]
	}
	firstCategory := sql.NullInt64{}
	if len(req.CategoryIds) > 0 {
		firstCategory = sql.NullInt64{Int64: req.CategoryIds[0], Valid: true}
	}

	res, err := tx.Exec(`
		INSERT INTO products (seller_id, name, category_id, price, detail, image_url, tags, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'available')`,
		sellerId, req.Name, firstCategory, req.Price, req.Detail, mainImage, tagsToJSON(req.Tags))
	if err != nil {
		return 0, err
	}
	productId, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	if err := insertImagesAndCategories(tx, productId, req); err != nil {
		return 0, err
	}
	return productId, tx.Commit()
}

// UpdateProduct 商品編集（出品者本人のみ・出品中のみ）
func (d *ProductDao) UpdateProduct(sellerId string, productId int64, req *model.SaveProductRequest) error {
	tx, err := d.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := checkOwnership(tx, productId, sellerId); err != nil {
		return err
	}

	mainImage := ""
	if len(req.ImageUrls) > 0 {
		mainImage = req.ImageUrls[0]
	}
	firstCategory := sql.NullInt64{}
	if len(req.CategoryIds) > 0 {
		firstCategory = sql.NullInt64{Int64: req.CategoryIds[0], Valid: true}
	}
	if _, err := tx.Exec(`
		UPDATE products SET name = ?, category_id = ?, price = ?, detail = ?, image_url = ?, tags = ?
		WHERE product_id = ?`,
		req.Name, firstCategory, req.Price, req.Detail, mainImage, tagsToJSON(req.Tags), productId); err != nil {
		return err
	}

	if _, err := tx.Exec("DELETE FROM product_images WHERE product_id = ?", productId); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM product_categories WHERE product_id = ?", productId); err != nil {
		return err
	}
	if err := insertImagesAndCategories(tx, productId, req); err != nil {
		return err
	}
	return tx.Commit()
}

func insertImagesAndCategories(tx *sql.Tx, productId int64, req *model.SaveProductRequest) error {
	for i, url := range req.ImageUrls {
		if _, err := tx.Exec("INSERT INTO product_images (product_id, position, url) VALUES (?, ?, ?)", productId, i, url); err != nil {
			return err
		}
	}
	for _, catId := range req.CategoryIds {
		if _, err := tx.Exec("INSERT IGNORE INTO product_categories (product_id, category_id) VALUES (?, ?)", productId, catId); err != nil {
			return err
		}
	}
	return nil
}

func checkOwnership(tx *sql.Tx, productId int64, sellerId string) error {
	var owner, status string
	err := tx.QueryRow("SELECT seller_id, status FROM products WHERE product_id = ? FOR UPDATE", productId).Scan(&owner, &status)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if owner != sellerId {
		return ErrForbidden
	}
	if status != "available" {
		return ErrConflict
	}
	return nil
}

// Delete 出品取り消し（出品者本人のみ・出品中のみ）
func (d *ProductDao) Delete(sellerId string, productId int64) error {
	tx, err := d.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := checkOwnership(tx, productId, sellerId); err != nil {
		return err
	}
	for _, q := range []string{
		"DELETE FROM product_images WHERE product_id = ?",
		"DELETE FROM product_categories WHERE product_id = ?",
		"DELETE FROM likes WHERE product_id = ?",
		"DELETE FROM products WHERE product_id = ?",
	} {
		if _, err := tx.Exec(q, productId); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Purchase 購入確定。値引き承認済みチャットルームがあればその価格を適用する
// 戻り値: 確定価格, 出品者ID, 商品名
func (d *ProductDao) Purchase(buyerId string, productId int64) (int, string, string, error) {
	tx, err := d.DB.Begin()
	if err != nil {
		return 0, "", "", err
	}
	defer tx.Rollback()

	var sellerId, status, name string
	var price int
	err = tx.QueryRow("SELECT seller_id, status, name, price FROM products WHERE product_id = ? FOR UPDATE", productId).
		Scan(&sellerId, &status, &name, &price)
	if err == sql.ErrNoRows {
		return 0, "", "", ErrNotFound
	}
	if err != nil {
		return 0, "", "", err
	}
	if sellerId == buyerId {
		return 0, "", "", ErrOwnProduct
	}
	if status != "available" {
		return 0, "", "", ErrConflict
	}

	// この購入者に対して承認された値引きがあれば適用（チャットルーム単位）
	var approved sql.NullInt64
	err = tx.QueryRow(`
		SELECT MIN(discount_approved) FROM chatrooms
		WHERE product_id = ? AND proposer_id = ? AND discount_approved > 0`,
		productId, buyerId).Scan(&approved)
	if err != nil {
		return 0, "", "", err
	}
	finalPrice := price
	if approved.Valid && approved.Int64 > 0 && int(approved.Int64) < price {
		finalPrice = int(approved.Int64)
	}

	if _, err := tx.Exec(
		"UPDATE products SET buyer_id = ?, status = 'unshipped', price = ? WHERE product_id = ?",
		buyerId, finalPrice, productId); err != nil {
		return 0, "", "", err
	}
	if _, err := tx.Exec(
		"UPDATE users SET total_sales = total_sales + ? WHERE user_id = ?",
		finalPrice, sellerId); err != nil {
		return 0, "", "", err
	}
	return finalPrice, sellerId, name, tx.Commit()
}

// Ship 発送完了（出品者本人のみ・未発送のみ）。戻り値: 購入者ID, 商品名
func (d *ProductDao) Ship(sellerId string, productId int64) (string, string, error) {
	var owner, status, name string
	var buyerId sql.NullString
	err := d.DB.QueryRow("SELECT seller_id, status, name, buyer_id FROM products WHERE product_id = ?", productId).
		Scan(&owner, &status, &name, &buyerId)
	if err == sql.ErrNoRows {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", err
	}
	if owner != sellerId {
		return "", "", ErrForbidden
	}
	if status != "unshipped" || !buyerId.Valid {
		return "", "", ErrConflict
	}
	if _, err := d.DB.Exec("UPDATE products SET status = 'shipped' WHERE product_id = ?", productId); err != nil {
		return "", "", err
	}
	return buyerId.String, name, nil
}

// FindBySeller 自分の出品商品一覧。出品中で値引き提案が来ているものはnegotiatingとして返す
func (d *ProductDao) FindBySeller(sellerId string) ([]model.ProductSummary, error) {
	query := `
		SELECT p.product_id, p.name, p.price, COALESCE(p.image_url, ''), p.likes_count,
		       CASE WHEN p.status = 'available' AND EXISTS(
		              SELECT 1 FROM chatrooms r WHERE r.product_id = p.product_id
		              AND r.discount_proposed > 0 AND r.discount_approved = 0)
		            THEN 'negotiating' ELSE p.status END,
		       p.created_at,
		       COALESCE((SELECT c2.name FROM product_categories pc JOIN categories c2 ON c2.category_id = pc.category_id
		                 WHERE pc.product_id = p.product_id ORDER BY c2.category_id LIMIT 1), ''),
		       FALSE
		FROM products p
		WHERE p.seller_id = ?
		ORDER BY p.created_at DESC`
	rows, err := d.DB.Query(query, sellerId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list, err := scanSummaries(rows)
	if err != nil {
		return nil, err
	}

	// 商品ごとの未読チャット数を付与
	for i := range list {
		var unread int
		err := d.DB.QueryRow(`
			SELECT COUNT(*) FROM chats c
			JOIN chatrooms r ON r.chatroom_id = c.chatroom_id
			WHERE r.product_id = ? AND c.sender_id <> ? AND c.is_read = 0`,
			list[i].ProductId, sellerId).Scan(&unread)
		if err == nil {
			list[i].UnreadChatCount = unread
		}
	}
	return list, nil
}

// GetSellerSummary 取引・出品管理ダッシュボードのサマリー集計
func (d *ProductDao) GetSellerSummary(sellerId string) (*model.SellerSummary, error) {
	var s model.SellerSummary
	err := d.DB.QueryRow(`
		SELECT COALESCE((SELECT total_sales FROM users WHERE user_id = ?), 0),
		       (SELECT COUNT(*) FROM products WHERE seller_id = ? AND buyer_id IS NOT NULL),
		       (SELECT COUNT(*) FROM chatrooms r JOIN products p ON p.product_id = r.product_id
		        WHERE p.seller_id = ? AND r.discount_proposed > 0 AND r.discount_approved = 0 AND p.status = 'available'),
		       (SELECT COUNT(*) FROM products WHERE seller_id = ? AND status = 'available'),
		       (SELECT COUNT(*) FROM products WHERE seller_id = ? AND status = 'unshipped')`,
		sellerId, sellerId, sellerId, sellerId, sellerId).Scan(
		&s.TotalSales, &s.SoldCount, &s.ActiveChatCount, &s.CurrentListingCount, &s.PendingDeliveryCount)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// FindPurchases 自分の購入履歴
func (d *ProductDao) FindPurchases(buyerId string) ([]model.ProductSummary, error) {
	query := `
		SELECT p.product_id, p.name, p.price, COALESCE(p.image_url, ''), p.likes_count, p.status, p.created_at,
		       COALESCE((SELECT c2.name FROM product_categories pc JOIN categories c2 ON c2.category_id = pc.category_id
		                 WHERE pc.product_id = p.product_id ORDER BY c2.category_id LIMIT 1), ''),
		       EXISTS(SELECT 1 FROM likes l WHERE l.product_id = p.product_id AND l.user_id = ?)
		FROM products p
		WHERE p.buyer_id = ?
		ORDER BY p.updated_at DESC`
	rows, err := d.DB.Query(query, buyerId, buyerId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSummaries(rows)
}

// FindLiked 自分がいいねした商品一覧
func (d *ProductDao) FindLiked(userId string) ([]model.ProductSummary, error) {
	query := `
		SELECT p.product_id, p.name, p.price, COALESCE(p.image_url, ''), p.likes_count, p.status, p.created_at,
		       COALESCE((SELECT c2.name FROM product_categories pc JOIN categories c2 ON c2.category_id = pc.category_id
		                 WHERE pc.product_id = p.product_id ORDER BY c2.category_id LIMIT 1), ''),
		       TRUE
		FROM products p
		JOIN likes l ON l.product_id = p.product_id
		WHERE l.user_id = ?
		ORDER BY p.created_at DESC`
	rows, err := d.DB.Query(query, userId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSummaries(rows)
}

// Like いいねを付ける（重複は無視）
func (d *ProductDao) Like(userId string, productId int64) error {
	res, err := d.DB.Exec("INSERT IGNORE INTO likes (product_id, user_id) VALUES (?, ?)", productId, userId)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		_, err = d.DB.Exec("UPDATE products SET likes_count = likes_count + 1 WHERE product_id = ?", productId)
	}
	return err
}

// Unlike いいねを外す
func (d *ProductDao) Unlike(userId string, productId int64) error {
	res, err := d.DB.Exec("DELETE FROM likes WHERE product_id = ? AND user_id = ?", productId, userId)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		_, err = d.DB.Exec("UPDATE products SET likes_count = GREATEST(likes_count - 1, 0) WHERE product_id = ?", productId)
	}
	return err
}
