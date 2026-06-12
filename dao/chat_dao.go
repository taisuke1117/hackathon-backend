package dao

import (
	"database/sql"
	"main/model"
)

type ChatDao struct {
	DB *sql.DB
}

func NewChatDao(db *sql.DB) *ChatDao {
	return &ChatDao{DB: db}
}

// FindOrCreateRoom 商品×購入希望者のルームを取得、なければ作成
func (d *ChatDao) FindOrCreateRoom(proposerId string, productId int64) (int64, error) {
	var sellerId string
	err := d.DB.QueryRow("SELECT seller_id FROM products WHERE product_id = ?", productId).Scan(&sellerId)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	if sellerId == proposerId {
		return 0, ErrForbidden // 自分の商品にはルームを作れない
	}

	var roomId int64
	err = d.DB.QueryRow(
		"SELECT chatroom_id FROM chatrooms WHERE product_id = ? AND proposer_id = ? LIMIT 1",
		productId, proposerId).Scan(&roomId)
	if err == nil {
		return roomId, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}

	res, err := d.DB.Exec(
		"INSERT INTO chatrooms (product_id, proposer_id) VALUES (?, ?)",
		productId, proposerId)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListRooms 自分のチャット一覧。role: "buying"(自分が購入希望者) / "selling"(自分が出品者)
func (d *ChatDao) ListRooms(userId, role string) ([]model.ChatRoomSummary, error) {
	var query string
	if role == "selling" {
		query = `
			SELECT r.chatroom_id, p.product_id, p.name, COALESCE(p.image_url, ''), p.price, p.status,
			       u.user_id, u.name, COALESCE(u.icon_url, ''),
			       r.discount_proposed, r.discount_approved
			FROM chatrooms r
			JOIN products p ON p.product_id = r.product_id
			JOIN users u ON u.user_id = r.proposer_id
			WHERE p.seller_id = ?
			ORDER BY r.chatroom_id DESC`
	} else {
		query = `
			SELECT r.chatroom_id, p.product_id, p.name, COALESCE(p.image_url, ''), p.price, p.status,
			       u.user_id, u.name, COALESCE(u.icon_url, ''),
			       r.discount_proposed, r.discount_approved
			FROM chatrooms r
			JOIN products p ON p.product_id = r.product_id
			JOIN users u ON u.user_id = p.seller_id
			WHERE r.proposer_id = ?
			ORDER BY r.chatroom_id DESC`
	}

	rows, err := d.DB.Query(query, userId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]model.ChatRoomSummary, 0)
	for rows.Next() {
		var s model.ChatRoomSummary
		if err := rows.Scan(&s.ChatroomId, &s.ProductId, &s.ProductName, &s.ProductImage, &s.ProductPrice, &s.ProductStatus,
			&s.OtherUserId, &s.OtherUserName, &s.OtherUserIcon,
			&s.DiscountProposed, &s.DiscountApproved); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 最終メッセージと未読数を付与
	for i := range list {
		var content sql.NullString
		var at sql.NullTime
		err := d.DB.QueryRow(
			"SELECT content, created_at FROM chats WHERE chatroom_id = ? ORDER BY chat_id DESC LIMIT 1",
			list[i].ChatroomId).Scan(&content, &at)
		if err == nil {
			list[i].LastMessage = content.String
			if at.Valid {
				t := at.Time
				list[i].LastMessageAt = &t
			}
		}
		_ = d.DB.QueryRow(
			"SELECT COUNT(*) FROM chats WHERE chatroom_id = ? AND sender_id <> ? AND is_read = 0",
			list[i].ChatroomId, userId).Scan(&list[i].UnreadCount)
	}
	return list, nil
}

// FindRoomDetail ルーム詳細＋メッセージ一覧。当事者（購入希望者か出品者）以外は見られない
func (d *ChatDao) FindRoomDetail(userId string, roomId int64) (*model.ChatRoomDetail, error) {
	var detail model.ChatRoomDetail
	err := d.DB.QueryRow(`
		SELECT r.chatroom_id, p.product_id, p.name, COALESCE(p.description, ''), COALESCE(p.image_url, ''), p.price, p.status,
		       p.seller_id, r.proposer_id, r.discount_proposed, r.discount_approved
		FROM chatrooms r
		JOIN products p ON p.product_id = r.product_id
		WHERE r.chatroom_id = ?`, roomId).Scan(
		&detail.ChatroomId, &detail.ProductId, &detail.ProductName, &detail.ProductDescription, &detail.ProductImage, &detail.ProductPrice, &detail.ProductStatus,
		&detail.SellerId, &detail.ProposerId, &detail.DiscountProposed, &detail.DiscountApproved)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if userId != detail.SellerId && userId != detail.ProposerId {
		return nil, ErrForbidden
	}

	// 相手のプロフィール
	otherId := detail.SellerId
	if userId == detail.SellerId {
		otherId = detail.ProposerId
	}
	_ = d.DB.QueryRow("SELECT name, COALESCE(icon_url, '') FROM users WHERE user_id = ?", otherId).
		Scan(&detail.OtherUserName, &detail.OtherUserIcon)

	// メッセージ一覧
	detail.Messages = []model.ChatMessage{}
	rows, err := d.DB.Query(
		"SELECT chat_id, sender_id, content, is_read, created_at FROM chats WHERE chatroom_id = ? ORDER BY chat_id",
		roomId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var m model.ChatMessage
		if err := rows.Scan(&m.ChatId, &m.SenderId, &m.Content, &m.IsRead, &m.CreatedAt); err != nil {
			return nil, err
		}
		detail.Messages = append(detail.Messages, m)
	}
	return &detail, rows.Err()
}

// isParticipant ルームの当事者か検証し、(出品者ID, 購入希望者ID, 商品ID)を返す
func (d *ChatDao) roomParticipants(roomId int64) (sellerId, proposerId string, productId int64, err error) {
	err = d.DB.QueryRow(`
		SELECT p.seller_id, r.proposer_id, p.product_id
		FROM chatrooms r JOIN products p ON p.product_id = r.product_id
		WHERE r.chatroom_id = ?`, roomId).Scan(&sellerId, &proposerId, &productId)
	if err == sql.ErrNoRows {
		err = ErrNotFound
	}
	return
}

// SendMessage メッセージ送信。戻り値: 受信者のユーザーID（通知用）
func (d *ChatDao) SendMessage(userId string, roomId int64, content string) (string, error) {
	sellerId, proposerId, _, err := d.roomParticipants(roomId)
	if err != nil {
		return "", err
	}
	if userId != sellerId && userId != proposerId {
		return "", ErrForbidden
	}
	if _, err := d.DB.Exec(
		"INSERT INTO chats (chatroom_id, sender_id, content) VALUES (?, ?, ?)",
		roomId, userId, content); err != nil {
		return "", err
	}
	if userId == sellerId {
		return proposerId, nil
	}
	return sellerId, nil
}

// MarkRead 相手からのメッセージを既読にする
func (d *ChatDao) MarkRead(userId string, roomId int64) error {
	sellerId, proposerId, _, err := d.roomParticipants(roomId)
	if err != nil {
		return err
	}
	if userId != sellerId && userId != proposerId {
		return ErrForbidden
	}
	_, err = d.DB.Exec(
		"UPDATE chats SET is_read = 1 WHERE chatroom_id = ? AND sender_id <> ?",
		roomId, userId)
	return err
}

// ProposeDiscount 値引き提案（購入希望者のみ）。戻り値: 出品者ID, 商品ID（通知用）
func (d *ChatDao) ProposeDiscount(userId string, roomId int64, price int) (string, int64, error) {
	sellerId, proposerId, productId, err := d.roomParticipants(roomId)
	if err != nil {
		return "", 0, err
	}
	if userId != proposerId {
		return "", 0, ErrForbidden
	}
	if _, err := d.DB.Exec(
		"UPDATE chatrooms SET discount_proposed = ?, discount_approved = 0 WHERE chatroom_id = ?",
		price, roomId); err != nil {
		return "", 0, err
	}
	return sellerId, productId, nil
}

// ApproveDiscount 値引き承認（出品者のみ）。このルームの購入希望者だけに適用される
// 戻り値: 購入希望者ID, 商品ID, 承認価格（通知用）
func (d *ChatDao) ApproveDiscount(userId string, roomId int64) (string, int64, int, error) {
	sellerId, proposerId, productId, err := d.roomParticipants(roomId)
	if err != nil {
		return "", 0, 0, err
	}
	if userId != sellerId {
		return "", 0, 0, ErrForbidden
	}
	var proposed int
	if err := d.DB.QueryRow("SELECT discount_proposed FROM chatrooms WHERE chatroom_id = ?", roomId).Scan(&proposed); err != nil {
		return "", 0, 0, err
	}
	if proposed <= 0 {
		return "", 0, 0, ErrConflict
	}
	if _, err := d.DB.Exec(
		"UPDATE chatrooms SET discount_approved = discount_proposed WHERE chatroom_id = ?",
		roomId); err != nil {
		return "", 0, 0, err
	}
	return proposerId, productId, proposed, nil
}
