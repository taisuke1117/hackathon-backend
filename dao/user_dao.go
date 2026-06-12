package dao

import (
	"database/sql"
	"main/model"
)

type UserDao struct {
	DB *sql.DB
}

func NewUserDao(db *sql.DB) *UserDao {
	return &UserDao{DB: db}
}

// Upsert 初回ログイン時の登録。既存ならログイン情報で名前だけ更新
func (d *UserDao) Upsert(userId string, req *model.RegisterUserRequest) error {
	_, err := d.DB.Exec(`
		INSERT INTO users (user_id, name, mail, icon_url) VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE name = VALUES(name)`,
		userId, req.Name, req.Mail, req.IconUrl)
	return err
}

// Update プロフィール更新
func (d *UserDao) Update(userId string, req *model.UpdateUserRequest) error {
	_, err := d.DB.Exec(`
		UPDATE users SET name = ?, place = ?, icon_url = ?, bio = ?, shipping_address = ?
		WHERE user_id = ?`,
		req.Name, req.Place, req.IconUrl, req.Bio, req.ShippingAddress, userId)
	return err
}

// FindById プロフィール1件取得（評価の平均・件数つき）
func (d *UserDao) FindById(userId string) (*model.User, error) {
	var u model.User
	query := `SELECT u.user_id, u.name, u.mail,
	                 COALESCE(u.place, ''), COALESCE(u.icon_url, ''),
	                 COALESCE(u.bio, ''), COALESCE(u.shipping_address, ''), u.total_sales,
	                 COALESCE((SELECT AVG(r.rating) FROM reviews r WHERE r.reviewee_id = u.user_id), 0),
	                 (SELECT COUNT(*) FROM reviews r WHERE r.reviewee_id = u.user_id)
	          FROM users u WHERE u.user_id = ?`

	err := d.DB.QueryRow(query, userId).Scan(
		&u.UserId, &u.Name, &u.Mail, &u.Place, &u.IconUrl, &u.Bio, &u.ShippingAddress, &u.TotalSales,
		&u.Rating, &u.ReviewCount,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetAllCategories 全カテゴリマスターの取得
func (d *UserDao) GetAllCategories() ([]model.Category, error) {
	rows, err := d.DB.Query("SELECT category_id, name FROM categories ORDER BY category_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]model.Category, 0)
	for rows.Next() {
		var c model.Category
		if err := rows.Scan(&c.CategoryId, &c.Name); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, nil
}

// GetLikedProductIds 自分がいいねした商品のIDリスト
func (d *UserDao) GetLikedProductIds(userId string) ([]int64, error) {
	rows, err := d.DB.Query("SELECT product_id FROM likes WHERE user_id = ?", userId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// Block ユーザーをブロックする
func (d *UserDao) Block(blockerId, blockedId string) error {
	_, err := d.DB.Exec(
		"INSERT IGNORE INTO blocks (blocker_id, blocked_id) VALUES (?, ?)",
		blockerId, blockedId)
	return err
}
