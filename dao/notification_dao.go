package dao

import (
	"database/sql"
	"main/model"
)

type NotificationDao struct {
	DB *sql.DB
}

func NewNotificationDao(db *sql.DB) *NotificationDao {
	return &NotificationDao{DB: db}
}

// Create 通知を作成（購入・発送・値引きなどのイベント時にサーバー側から呼ぶ）
func (d *NotificationDao) Create(userId, notifType, title, content, linkUrl string) error {
	_, err := d.DB.Exec(
		"INSERT INTO notifications (user_id, `type`, title, content, link_url) VALUES (?, ?, ?, ?, ?)",
		userId, notifType, title, content, linkUrl)
	return err
}

// ListByUser ユーザー宛ての通知一覧（最新順）
func (d *NotificationDao) ListByUser(userId string) ([]model.Notification, error) {
	rows, err := d.DB.Query(
		"SELECT notification_id, user_id, `type`, title, content, COALESCE(link_url, ''), is_read, created_at"+
			" FROM notifications WHERE user_id = ? ORDER BY created_at DESC LIMIT 100", userId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]model.Notification, 0)
	for rows.Next() {
		var n model.Notification
		if err := rows.Scan(&n.NotificationId, &n.UserId, &n.Type, &n.Title, &n.Content, &n.LinkUrl, &n.IsRead, &n.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, n)
	}
	return list, rows.Err()
}

// MarkRead 既読化（本人の通知のみ）
func (d *NotificationDao) MarkRead(userId string, notificationId int64) error {
	_, err := d.DB.Exec(
		"UPDATE notifications SET is_read = 1 WHERE notification_id = ? AND user_id = ?",
		notificationId, userId)
	return err
}
