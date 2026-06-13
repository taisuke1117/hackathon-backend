package dao

import (
	"database/sql"
	"encoding/json"
	"main/model"
)

// ─────────────────────────────────────────────────────────
// ProductDao
//
// productsテーブルおよび関連テーブル（product_images, product_categories,
// likes）へのDB操作を担うDAO（Data Access Object）。
// SQLとビジネスロジックを分離するために、controllerはこのDaoを呼ぶ。
// ─────────────────────────────────────────────────────────

type ProductDao struct {
	DB *sql.DB
}

func NewProductDao(db *sql.DB) *ProductDao {
	return &ProductDao{DB: db}
}

// tagsToJSON: タグのスライスをJSON文字列にシリアライズしてDBに保存する形式にする
// DBのtags列はTEXT型なので、[]string → `["tag1","tag2"]` のように変換する
func tagsToJSON(tags []string) string {
	if len(tags) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(tags)
	return string(b)
}

// tagsFromJSON: DBから取り出したJSON文字列を []string に戻す
func tagsFromJSON(s string) []string {
	tags := []string{}
	_ = json.Unmarshal([]byte(s), &tags)
	return tags
}

// Search: 出品中の商品一覧を検索条件で絞り込んで返す
//
// viewerId: ログインユーザーのUID（自分がいいねしているかどうかの判定に使う）
// c.Keyword: 商品名・詳細説明・タグに対してLIKE検索（前後ワイルドカード）
// c.CategoryId: 0なら全カテゴリ、0より大きければそのカテゴリのみ
// c.MinPrice/MaxPrice: 0なら下限/上限なし
// 並び順は新着順（created_at DESC）、最大100件
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

	// バインド変数が多いのは、MySQLがプレースホルダを1回しか使えないため同じ値を繰り返す
	rows, err := d.DB.Query(query,
		viewerId,
		c.Keyword, c.Keyword, c.Keyword, c.Keyword, // keyword: 4つのLIKE条件に対応
		c.CategoryId, c.CategoryId,                  // category: 存在チェック + 値
		c.MinPrice, c.MinPrice,                      // min_price: 条件フラグ + 値
		c.MaxPrice, c.MaxPrice)                      // max_price: 条件フラグ + 値
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSummaries(rows)
}

// scanSummaries: sql.Rows を []ProductSummary に変換するヘルパー
// Search と FindBySeller など複数の場所で同じ形のSELECTを使うため共通化
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

// FindDetail: 商品詳細を取得する
//
// 一覧（ProductSummary）より多くの情報を返す:
//   - 複数枚の画像URL（product_imagesテーブル）
//   - 複数カテゴリ（product_categoriesテーブル）
//   - タグ（JSON文字列をデコード）
//   - 出品者情報（controllerで別途付与）
//   - 自分がいいねしているか
//
// 副作用: 閲覧するたびに views_count を +1 する（アクセス解析用）
func (d *ProductDao) FindDetail(viewerId string, productId int64) (*model.ProductDetail, error) {
	var p model.ProductDetail
	var tagsJSON string
	err := d.DB.QueryRow(`
		SELECT p.product_id, p.seller_id, COALESCE(p.buyer_id, ''), p.name, p.price, COALESCE(p.detail, ''),
		       COALESCE(p.condition, ''), COALESCE(p.tags, '[]'), p.views_count, p.likes_count, p.status, p.created_at,
		       EXISTS(SELECT 1 FROM likes l WHERE l.product_id = p.product_id AND l.user_id = ?)
		FROM products p WHERE p.product_id = ?`, viewerId, productId).Scan(
		&p.ProductId, &p.SellerId, &p.BuyerId, &p.Name, &p.Price, &p.Detail,
		&p.Condition, &tagsJSON, &p.ViewsCount, &p.LikesCount, &p.Status, &p.CreatedAt, &p.LikedByMe)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.Tags = tagsFromJSON(tagsJSON) // JSON文字列 → []string に変換

	// ── 画像一覧を取得（positionの昇順＝アップロード順） ──
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

	// ── カテゴリ一覧を取得 ──
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

	// 閲覧数インクリメント（失敗しても詳細表示には影響しないのでエラーは無視）
	_, _ = d.DB.Exec("UPDATE products SET views_count = views_count + 1 WHERE product_id = ?", productId)

	return &p, nil
}

// Create: 新規出品。商品本体・画像・カテゴリをトランザクションで一括INSERT
//
// トランザクションを使う理由:
//   products, product_images, product_categories の3テーブルに書く。
//   途中で失敗したときに中途半端な状態が残らないよう全部か0かにする。
func (d *ProductDao) Create(sellerId string, req *model.SaveProductRequest) (int64, error) {
	tx, err := d.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() // Commit前にreturnしたらRollbackする（Commit後はRollbackは何もしない）

	// products.image_url は1枚目の画像（サムネイル用）
	mainImage := ""
	if len(req.ImageUrls) > 0 {
		mainImage = req.ImageUrls[0]
	}
	// products.category_id は後方互換のために残している旧カラム（1カテゴリ限定時代の名残）
	firstCategory := sql.NullInt64{}
	if len(req.CategoryIds) > 0 {
		firstCategory = sql.NullInt64{Int64: req.CategoryIds[0], Valid: true}
	}

	res, err := tx.Exec(
		"INSERT INTO products (seller_id, name, category_id, price, detail, `condition`, image_url, tags, status)"+
			" VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'available')",
		sellerId, req.Name, firstCategory, req.Price, req.Detail, req.Condition, mainImage, tagsToJSON(req.Tags))
	if err != nil {
		return 0, err
	}
	productId, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	// 画像とカテゴリを別テーブルに登録
	if err := insertImagesAndCategories(tx, productId, req); err != nil {
		return 0, err
	}
	return productId, tx.Commit()
}

// UpdateProduct: 商品編集（出品者本人かつ出品中のものだけ編集可能）
//
// 画像・カテゴリは「全削除してから再INSERT」のシンプルな方式
// （差分更新より実装が簡単で、件数が少ないのでパフォーマンス的にも問題ない）
func (d *ProductDao) UpdateProduct(sellerId string, productId int64, req *model.SaveProductRequest) error {
	tx, err := d.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 所有者チェックと状態チェック（本人以外・出品中以外は弾く）
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
	if _, err := tx.Exec(
		"UPDATE products SET name = ?, category_id = ?, price = ?, detail = ?, `condition` = ?, image_url = ?, tags = ?"+
			" WHERE product_id = ?",
		req.Name, firstCategory, req.Price, req.Detail, req.Condition, mainImage, tagsToJSON(req.Tags), productId); err != nil {
		return err
	}

	// 画像・カテゴリを全削除してから再登録
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

// insertImagesAndCategories: 画像URLとカテゴリIDを各テーブルに一括INSERT するヘルパー
// Create と UpdateProduct の両方から使う
func insertImagesAndCategories(tx *sql.Tx, productId int64, req *model.SaveProductRequest) error {
	// 画像: position = インデックス順（0始まり）で順番を保存
	for i, url := range req.ImageUrls {
		if _, err := tx.Exec("INSERT INTO product_images (product_id, position, url) VALUES (?, ?, ?)", productId, i, url); err != nil {
			return err
		}
	}
	// カテゴリ: INSERT IGNORE で重複を無視（同じカテゴリを2回送ってもエラーにしない）
	for _, catId := range req.CategoryIds {
		if _, err := tx.Exec("INSERT IGNORE INTO product_categories (product_id, category_id) VALUES (?, ?)", productId, catId); err != nil {
			return err
		}
	}
	return nil
}

// checkOwnership: 商品が出品者本人のものであり、かつ出品中（available）であるかを確認
//
// FOR UPDATE: 同時に別のトランザクションが購入処理を走らせていた場合の競合を防ぐ行ロック
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
		return ErrForbidden // 他人の商品を編集しようとした
	}
	if status != "available" {
		return ErrConflict // 購入済みや発送済みは編集不可
	}
	return nil
}

// Delete: 出品取り消し
//
// 関連テーブルを順番に削除してから商品本体を削除する
// （外部キー制約がなくても一貫性を保つために順番が重要）
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

// Purchase: 購入確定処理
//
// 複雑な処理なのでトランザクションで保護する:
//   1. 商品が存在するか・出品中かチェック（FOR UPDATE で行ロック）
//   2. 自分自身の商品は買えない
//   3. 値引き交渉が成立していればその価格を適用する
//   4. 商品ステータスを 'unshipped'（発送待ち）に更新
//   5. 出品者のtotal_salesを加算
//
// 戻り値: 確定価格, 出品者ID, 商品名
func (d *ProductDao) Purchase(buyerId string, productId int64) (int, string, string, error) {
	tx, err := d.DB.Begin()
	if err != nil {
		return 0, "", "", err
	}
	defer tx.Rollback()

	var sellerId, status, name string
	var price int
	// FOR UPDATE: 同時購入の競合を防ぐ（二重購入防止）
	err = tx.QueryRow("SELECT seller_id, status, name, price FROM products WHERE product_id = ? FOR UPDATE", productId).
		Scan(&sellerId, &status, &name, &price)
	if err == sql.ErrNoRows {
		return 0, "", "", ErrNotFound
	}
	if err != nil {
		return 0, "", "", err
	}
	if sellerId == buyerId {
		return 0, "", "", ErrOwnProduct // 自分の商品は買えない
	}
	if status != "available" {
		return 0, "", "", ErrConflict // すでに別の人が購入済み
	}

	// 値引き承認済みの価格を探す
	// 購入者（buyerId）がこの商品のチャットルームで提案した値引きで、出品者が承認したもの
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
		finalPrice = int(approved.Int64) // 値引き価格を適用
	}

	// 商品ステータスを更新 + 確定価格で上書き
	if _, err := tx.Exec(
		"UPDATE products SET buyer_id = ?, status = 'unshipped', price = ? WHERE product_id = ?",
		buyerId, finalPrice, productId); err != nil {
		return 0, "", "", err
	}
	// 出品者の累計売上に加算
	if _, err := tx.Exec(
		"UPDATE users SET total_sales = total_sales + ? WHERE user_id = ?",
		finalPrice, sellerId); err != nil {
		return 0, "", "", err
	}
	return finalPrice, sellerId, name, tx.Commit()
}

// Ship: 発送完了処理（出品者が「発送しました」ボタンを押したとき）
//
// ステータス: unshipped → shipped
// 戻り値: 購入者ID, 商品名（通知に使う）
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
		return "", "", ErrConflict // 発送待ち状態でなければ操作不可
	}
	if _, err := d.DB.Exec("UPDATE products SET status = 'shipped' WHERE product_id = ?", productId); err != nil {
		return "", "", err
	}
	return buyerId.String, name, nil
}

// FindBySeller: 自分が出品した商品一覧を返す（取引管理ページ用）
//
// 出品中で値引き提案が来ているものはステータスを 'negotiating' として返す
// （UIで「交渉中」バッジを出すため）
// さらに各商品の未読チャット数も付与する
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

	// 商品ごとの未読チャット数を1件ずつクエリで取得して付与
	// （パフォーマンスが気になるならJOINにできるが、件数が少ないのでこれで十分）
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

// GetSellerSummary: 取引管理ダッシュボードのサマリー数値を一括取得
// （累計売上・売れた件数・交渉中チャット数・出品中件数・発送待ち件数）
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

// FindPurchases: 自分が購入した商品の履歴（購入日時の新しい順）
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

// FindLiked: 自分がいいねした商品一覧
func (d *ProductDao) FindLiked(userId string) ([]model.ProductSummary, error) {
	query := `
		SELECT p.product_id, p.name, p.price, COALESCE(p.image_url, ''), p.likes_count, p.status, p.created_at,
		       COALESCE((SELECT c2.name FROM product_categories pc JOIN categories c2 ON c2.category_id = pc.category_id
		                 WHERE pc.product_id = p.product_id ORDER BY c2.category_id LIMIT 1), ''),
		       TRUE -- いいね一覧なので常にtrue
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

// Like: いいねを付ける
//
// INSERT IGNORE: 同じ（product_id, user_id）の組み合わせが既にある場合は無視
// RowsAffected で実際にINSERTされたか確認してからlikes_countを+1する
// （二重カウント防止）
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

// Unlike: いいねを外す
//
// GREATEST(likes_count - 1, 0): 万が一 likes_count が0になっても負数にならないよう保護
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
