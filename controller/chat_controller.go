package controller

import (
	"fmt"
	"main/dao"
	"main/middleware"
	"net/http"
)

type ChatController struct {
	chatDao         *dao.ChatDao
	notificationDao *dao.NotificationDao
}

func NewChatController(chatDao *dao.ChatDao, notificationDao *dao.NotificationDao) *ChatController {
	return &ChatController{chatDao: chatDao, notificationDao: notificationDao}
}

// CreateRoom POST /api/chatrooms {product_id} — ルーム取得or作成
func (c *ChatController) CreateRoom(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProductId int64 `json:"product_id"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.ProductId <= 0 {
		writeError(w, http.StatusBadRequest, "product_idは必須です")
		return
	}
	roomId, err := c.chatDao.FindOrCreateRoom(middleware.UserId(r), req.ProductId)
	if err != nil {
		handleDaoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"chatroom_id": roomId})
}

// ListRooms GET /api/chatrooms?role=buying|selling
func (c *ChatController) ListRooms(w http.ResponseWriter, r *http.Request) {
	role := r.URL.Query().Get("role")
	if role != "selling" {
		role = "buying"
	}
	rooms, err := c.chatDao.ListRooms(middleware.UserId(r), role)
	if err != nil {
		handleDaoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rooms)
}

// RoomDetail GET /api/chatrooms/{id} — メタ情報＋メッセージ一覧
func (c *ChatController) RoomDetail(w http.ResponseWriter, r *http.Request) {
	id, ok := pathId(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "ルームIDが不正です")
		return
	}
	detail, err := c.chatDao.FindRoomDetail(middleware.UserId(r), id)
	if err != nil {
		handleDaoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// SendMessage POST /api/chatrooms/{id}/messages {content}
func (c *ChatController) SendMessage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathId(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "ルームIDが不正です")
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "contentは必須です")
		return
	}
	if _, err := c.chatDao.SendMessage(middleware.UserId(r), id, req.Content); err != nil {
		handleDaoError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "sent"})
}

// MarkRead PUT /api/chatrooms/{id}/read — 相手メッセージの既読化
func (c *ChatController) MarkRead(w http.ResponseWriter, r *http.Request) {
	id, ok := pathId(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "ルームIDが不正です")
		return
	}
	if err := c.chatDao.MarkRead(middleware.UserId(r), id); err != nil {
		handleDaoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "read"})
}

// ProposeDiscount POST /api/chatrooms/{id}/discount {price} — 値引き提案（購入希望者）
func (c *ChatController) ProposeDiscount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathId(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "ルームIDが不正です")
		return
	}
	var req struct {
		Price int `json:"price"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Price <= 0 {
		writeError(w, http.StatusBadRequest, "正の価格priceは必須です")
		return
	}
	sellerId, productId, err := c.chatDao.ProposeDiscount(middleware.UserId(r), id, req.Price)
	if err != nil {
		handleDaoError(w, err)
		return
	}
	_ = c.notificationDao.Create(sellerId, "transaction",
		"値引き交渉が届きました",
		fmt.Sprintf("¥%dへの値引き希望が届いています。チャットを確認してください。", req.Price),
		fmt.Sprintf("/chat/seller/%d/%d", productId, id))

	writeJSON(w, http.StatusOK, map[string]string{"status": "proposed"})
}

// ApproveDiscount PUT /api/chatrooms/{id}/discount/approve — 値引き承認（出品者）
// このルームの購入希望者だけに承認価格が適用される
func (c *ChatController) ApproveDiscount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathId(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "ルームIDが不正です")
		return
	}
	proposerId, productId, price, err := c.chatDao.ApproveDiscount(middleware.UserId(r), id)
	if err != nil {
		handleDaoError(w, err)
		return
	}
	_ = c.notificationDao.Create(proposerId, "transaction",
		"価格交渉が成立しました！",
		fmt.Sprintf("出品者があなたの値引き要求（¥%d）を承認しました。この価格で購入できます。", price),
		fmt.Sprintf("/chat/%d/%d", productId, id))

	writeJSON(w, http.StatusOK, map[string]any{"status": "approved", "price": price})
}
