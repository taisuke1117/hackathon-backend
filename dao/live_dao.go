package dao

import (
	"database/sql"
	"main/model"
	"time"
)

type LiveDao struct {
	DB *sql.DB
}

func NewLiveDao(db *sql.DB) *LiveDao {
	return &LiveDao{DB: db}
}

// CreateRoom: ライブルームと商品キューをまとめて作成
func (d *LiveDao) CreateRoom(sellerId string, req *model.CreateLiveRoomRequest) (int64, error) {
	tx, err := d.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		"INSERT INTO live_rooms (seller_id, title, status) VALUES (?, ?, 'scheduled')",
		sellerId, req.Title)
	if err != nil {
		return 0, err
	}
	roomId, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	for i, p := range req.Products {
		_, err := tx.Exec(
			"INSERT INTO live_products (room_id, product_id, position, mode, start_price, instant_price) VALUES (?, ?, ?, ?, ?, ?)",
			roomId, p.ProductId, i, p.Mode, p.StartPrice, p.InstantPrice)
		if err != nil {
			return 0, err
		}
	}
	return roomId, tx.Commit()
}

// ListRooms: live（配信中）のルーム一覧を返す
func (d *LiveDao) ListRooms() ([]model.LiveRoom, error) {
	rows, err := d.DB.Query(`
		SELECT r.room_id, r.seller_id, COALESCE(u.name,''), COALESCE(u.icon_url,''),
		       r.title, r.status, r.viewer_count, r.created_at
		FROM live_rooms r
		JOIN users u ON u.user_id = r.seller_id
		WHERE r.status = 'live'
		ORDER BY r.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]model.LiveRoom, 0)
	for rows.Next() {
		var room model.LiveRoom
		if err := rows.Scan(&room.RoomId, &room.SellerId, &room.SellerName, &room.SellerIcon,
			&room.Title, &room.Status, &room.ViewerCount, &room.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, room)
	}
	return list, rows.Err()
}

// FindRoom: ルーム詳細（現在の商品＋残りキュー）
func (d *LiveDao) FindRoom(roomId int64) (*model.LiveRoomDetail, error) {
	var detail model.LiveRoomDetail
	err := d.DB.QueryRow(`
		SELECT r.room_id, r.seller_id, COALESCE(u.name,''), COALESCE(u.icon_url,''),
		       r.title, r.status, r.viewer_count, r.created_at
		FROM live_rooms r
		JOIN users u ON u.user_id = r.seller_id
		WHERE r.room_id = ?`, roomId).Scan(
		&detail.RoomId, &detail.SellerId, &detail.SellerName, &detail.SellerIcon,
		&detail.Title, &detail.Status, &detail.ViewerCount, &detail.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	// 商品キュー（active + waiting）を取得
	rows, err := d.DB.Query(`
		SELECT lp.id, lp.room_id, lp.product_id, COALESCE(p.name,''), COALESCE(p.image_url,''),
		       lp.position, lp.mode, lp.start_price, lp.instant_price,
		       lp.current_price, COALESCE(lp.current_bidder,''), lp.auction_end_at,
		       lp.status, COALESCE(lp.buyer_id,'')
		FROM live_products lp
		JOIN products p ON p.product_id = lp.product_id
		WHERE lp.room_id = ? AND lp.status IN ('active','waiting')
		ORDER BY lp.position`, roomId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	detail.Queue = []model.LiveProduct{}
	for rows.Next() {
		lp, err := scanLiveProduct(rows)
		if err != nil {
			return nil, err
		}
		if lp.Status == "active" {
			detail.CurrentProduct = lp
		} else {
			detail.Queue = append(detail.Queue, *lp)
		}
	}
	return &detail, rows.Err()
}

// StartRoom: ステータスを live に更新し Livekit ルーム名を記録
func (d *LiveDao) StartRoom(roomId int64, sellerId, livekitRoom string) error {
	res, err := d.DB.Exec(
		"UPDATE live_rooms SET status='live', livekit_room_name=? WHERE room_id=? AND seller_id=?",
		livekitRoom, roomId, sellerId)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrForbidden
	}
	// 最初の商品をアクティブにする
	return d.activateNextProduct(roomId)
}

// EndRoom: ステータスを ended に更新
func (d *LiveDao) EndRoom(roomId int64, sellerId string) error {
	res, err := d.DB.Exec(
		"UPDATE live_rooms SET status='ended' WHERE room_id=? AND seller_id=?",
		roomId, sellerId)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrForbidden
	}
	return nil
}

// GetCurrentProduct: 現在アクティブな商品を取得
func (d *LiveDao) GetCurrentProduct(roomId int64) (*model.LiveProduct, error) {
	rows, err := d.DB.Query(`
		SELECT lp.id, lp.room_id, lp.product_id, COALESCE(p.name,''), COALESCE(p.image_url,''),
		       lp.position, lp.mode, lp.start_price, lp.instant_price,
		       lp.current_price, COALESCE(lp.current_bidder,''), lp.auction_end_at,
		       lp.status, COALESCE(lp.buyer_id,'')
		FROM live_products lp
		JOIN products p ON p.product_id = lp.product_id
		WHERE lp.room_id = ? AND lp.status = 'active'
		LIMIT 1`, roomId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, ErrNotFound
	}
	return scanLiveProduct(rows)
}

// PlaceBid: 入札。現在価格を更新してタイマーを30秒リセット
func (d *LiveDao) PlaceBid(roomId int64, bidderId string, amount int) (*model.LiveProduct, error) {
	newEndAt := time.Now().Add(30 * time.Second)
	res, err := d.DB.Exec(`
		UPDATE live_products
		SET current_price=?, current_bidder=?, auction_end_at=?
		WHERE room_id=? AND status='active' AND mode='auction' AND ? > current_price`,
		amount, bidderId, newEndAt, roomId, amount)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrConflict // 金額が低いか対象がない
	}
	return d.GetCurrentProduct(roomId)
}

// SoldByAuction: オークション落札（タイマー満了時にコントローラから呼ぶ）
func (d *LiveDao) SoldByAuction(liveProductId int64) (*model.LiveProduct, error) {
	// 現在の最高入札者を落札者に設定
	_, err := d.DB.Exec(`
		UPDATE live_products
		SET status='sold', buyer_id=current_bidder
		WHERE id=? AND status='active'`, liveProductId)
	if err != nil {
		return nil, err
	}
	// productのstatusも更新（フリマ通常フローへ）
	rows, err := d.DB.Query(`
		SELECT lp.id, lp.room_id, lp.product_id, COALESCE(p.name,''), COALESCE(p.image_url,''),
		       lp.position, lp.mode, lp.start_price, lp.instant_price,
		       lp.current_price, COALESCE(lp.current_bidder,''), lp.auction_end_at,
		       lp.status, COALESCE(lp.buyer_id,'')
		FROM live_products lp
		JOIN products p ON p.product_id = lp.product_id
		WHERE lp.id = ?`, liveProductId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, ErrNotFound
	}
	return scanLiveProduct(rows)
}

// InstantBuy: 即決購入
func (d *LiveDao) InstantBuy(roomId int64, buyerId string) (*model.LiveProduct, error) {
	res, err := d.DB.Exec(`
		UPDATE live_products
		SET status='sold', buyer_id=?, current_price=COALESCE(instant_price, start_price)
		WHERE room_id=? AND status='active' AND mode='instant'`,
		buyerId, roomId)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrConflict
	}

	rows, err := d.DB.Query(`
		SELECT lp.id, lp.room_id, lp.product_id, COALESCE(p.name,''), COALESCE(p.image_url,''),
		       lp.position, lp.mode, lp.start_price, lp.instant_price,
		       lp.current_price, COALESCE(lp.current_bidder,''), lp.auction_end_at,
		       lp.status, COALESCE(lp.buyer_id,'')
		FROM live_products lp
		JOIN products p ON p.product_id = lp.product_id
		WHERE lp.room_id=? AND lp.status='sold'
		ORDER BY lp.position DESC LIMIT 1`, roomId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, ErrNotFound
	}
	return scanLiveProduct(rows)
}

// AdvanceQueue: 次の商品をアクティブにする。次がなければ nil を返す
func (d *LiveDao) AdvanceQueue(roomId int64) (*model.LiveProduct, error) {
	// sold/skipped を残して、次の waiting をアクティブにする
	if err := d.activateNextProduct(roomId); err != nil {
		if err == ErrNotFound {
			return nil, nil // キュー終了
		}
		return nil, err
	}
	return d.GetCurrentProduct(roomId)
}

// ChangeViewerCount: 視聴者数を増減
func (d *LiveDao) ChangeViewerCount(roomId int64, delta int) error {
	_, err := d.DB.Exec(
		"UPDATE live_rooms SET viewer_count = GREATEST(0, viewer_count + ?) WHERE room_id = ?",
		delta, roomId)
	return err
}

// GetBidderName: 入札者名を取得
func (d *LiveDao) GetBidderName(userId string) string {
	var name string
	_ = d.DB.QueryRow("SELECT name FROM users WHERE user_id = ?", userId).Scan(&name)
	return name
}

// ── 内部ヘルパー ─────────────────────────────────────────────────

func (d *LiveDao) activateNextProduct(roomId int64) error {
	// 次の waiting 商品の ID を取得
	var nextId int64
	err := d.DB.QueryRow(
		"SELECT id FROM live_products WHERE room_id=? AND status='waiting' ORDER BY position LIMIT 1",
		roomId).Scan(&nextId)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	endAt := time.Now().Add(30 * time.Second)
	_, err = d.DB.Exec(`
		UPDATE live_products
		SET status='active', current_price=start_price, auction_end_at=?
		WHERE id=?`, endAt, nextId)
	return err
}

func scanLiveProduct(rows *sql.Rows) (*model.LiveProduct, error) {
	var lp model.LiveProduct
	var endAt sql.NullTime
	var instantPrice sql.NullInt64
	if err := rows.Scan(
		&lp.Id, &lp.RoomId, &lp.ProductId, &lp.ProductName, &lp.ProductImage,
		&lp.Position, &lp.Mode, &lp.StartPrice, &instantPrice,
		&lp.CurrentPrice, &lp.CurrentBidder, &endAt,
		&lp.Status, &lp.BuyerId,
	); err != nil {
		return nil, err
	}
	if instantPrice.Valid {
		v := int(instantPrice.Int64)
		lp.InstantPrice = &v
	}
	if endAt.Valid {
		t := endAt.Time
		lp.AuctionEndAt = &t
		left := int(time.Until(t).Seconds())
		if left < 0 {
			left = 0
		}
		lp.SecondsLeft = left
	}
	return &lp, nil
}
