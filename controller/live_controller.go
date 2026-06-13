package controller

import (
	"encoding/json"
	"fmt"
	"log"
	"main/dao"
	mid "main/middleware"
	"main/model"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/livekit/protocol/auth"
)

// ─────────────────────────────────────────────────────────
// LiveController
//
// ライブ配信機能のHTTPハンドラをまとめたコントローラ。
// SSEハブ（liveHub）でリアルタイムな入札・タイマー情報を全視聴者にブロードキャストする。
// ─────────────────────────────────────────────────────────

type LiveController struct {
	dao *dao.LiveDao
	hub *liveHub
}

func NewLiveController(liveDao *dao.LiveDao) *LiveController {
	return &LiveController{
		dao: liveDao,
		hub: newLiveHub(),
	}
}

// ── SSEハブ（Server-Sent Events のブロードキャスト管理） ─────────

type liveHub struct {
	mu     sync.Mutex
	rooms  map[int64]*roomState
}

type roomState struct {
	clients map[chan []byte]struct{}
	timer   *time.Timer // オークションのカウントダウンタイマー
	timerMu sync.Mutex
}

func newLiveHub() *liveHub {
	return &liveHub{rooms: make(map[int64]*roomState)}
}

func (h *liveHub) getOrCreate(roomId int64) *roomState {
	h.mu.Lock()
	defer h.mu.Unlock()
	if s, ok := h.rooms[roomId]; ok {
		return s
	}
	s := &roomState{clients: make(map[chan []byte]struct{})}
	h.rooms[roomId] = s
	return s
}

func (h *liveHub) subscribe(roomId int64) chan []byte {
	s := h.getOrCreate(roomId)
	ch := make(chan []byte, 32)
	h.mu.Lock()
	s.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *liveHub) unsubscribe(roomId int64, ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if s, ok := h.rooms[roomId]; ok {
		delete(s.clients, ch)
	}
}

// broadcast: 指定ルームの全SSEクライアントにイベントを送る
func (h *liveHub) broadcast(roomId int64, event model.LiveEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.mu.Lock()
	s, ok := h.rooms[roomId]
	h.mu.Unlock()
	if !ok {
		return
	}
	h.mu.Lock()
	for ch := range s.clients {
		select {
		case ch <- data:
		default: // クライアントが遅い場合はスキップ
		}
	}
	h.mu.Unlock()
}

// startAuctionTimer: 30秒カウントダウン。入札のたびにリセットされる。
// 満了したら自動的に落札処理を行いキューを進める。
func (h *liveHub) startAuctionTimer(ctrl *LiveController, roomId int64, liveProductId int64) {
	s := h.getOrCreate(roomId)
	s.timerMu.Lock()
	defer s.timerMu.Unlock()

	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(30*time.Second, func() {
		ctrl.onAuctionExpired(roomId, liveProductId)
	})
}

func (h *liveHub) resetTimer(ctrl *LiveController, roomId int64, liveProductId int64) {
	h.startAuctionTimer(ctrl, roomId, liveProductId)
}

func (h *liveHub) stopTimer(roomId int64) {
	h.mu.Lock()
	s, ok := h.rooms[roomId]
	h.mu.Unlock()
	if !ok {
		return
	}
	s.timerMu.Lock()
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timerMu.Unlock()
}

// onAuctionExpired: タイマー満了 → 入札あり=落札 / 入札なし=スキップ → 次の商品へ
func (ctrl *LiveController) onAuctionExpired(roomId int64, liveProductId int64) {
	// まずスキップを試みる（入札者なし判定）
	if err := ctrl.dao.SkipProduct(liveProductId); err == nil {
		// 入札なしでスキップ
		ctrl.hub.broadcast(roomId, model.LiveEvent{Type: "skipped"})
		time.Sleep(2 * time.Second)
		ctrl.advanceToNext(roomId)
		return
	}

	// スキップ失敗 = 入札者がいた → 落札処理
	sold, err := ctrl.dao.SoldByAuction(liveProductId)
	if err != nil {
		log.Printf("live: sold failed after auction: %v", err)
		// 落札もスキップも失敗 = 既に他の処理で完了済み
		ctrl.advanceToNext(roomId)
		return
	}

	buyerName := ctrl.dao.GetBidderName(sold.BuyerId)
	ctrl.hub.broadcast(roomId, model.LiveEvent{
		Type:       "sold",
		Product:    sold,
		FinalPrice: sold.CurrentPrice,
		BuyerName:  buyerName,
	})

	time.Sleep(3 * time.Second)
	ctrl.advanceToNext(roomId)
}

// advanceToNext: 次の商品をアクティブにしてブロードキャスト
// キューが空になっても自動終了しない（配信者が手動で終了する）
func (ctrl *LiveController) advanceToNext(roomId int64) {
	next, err := ctrl.dao.AdvanceQueue(roomId)
	if err != nil {
		log.Printf("live: advance queue error: %v", err)
		return
	}
	if next == nil {
		// キュー終了 → 配信者に委ねる（自動終了しない）
		ctrl.hub.broadcast(roomId, model.LiveEvent{Type: "queue_empty"})
		return
	}

	ctrl.hub.broadcast(roomId, model.LiveEvent{Type: "next", Product: next})

	if next.Mode == "auction" {
		ctrl.hub.startAuctionTimer(ctrl, roomId, next.Id)
	}
}

// ── HTTPハンドラ ──────────────────────────────────────────────────

// POST /api/live/rooms — 配信ルーム作成（商品キューも一括登録）
func (c *LiveController) CreateRoom(w http.ResponseWriter, r *http.Request) {
	uid := mid.UserId(r)
	var req model.CreateLiveRoomRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "タイトルは必須です")
		return
	}
	if len(req.Products) == 0 {
		writeError(w, http.StatusBadRequest, "商品を1つ以上設定してください")
		return
	}
	roomId, err := c.dao.CreateRoom(uid, &req)
	if err != nil {
		handleDaoError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"room_id": roomId})
}

// GET /api/live/rooms — 配信中ルーム一覧
func (c *LiveController) ListRooms(w http.ResponseWriter, r *http.Request) {
	rooms, err := c.dao.ListRooms()
	if err != nil {
		handleDaoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rooms)
}

// GET /api/live/rooms/{id} — ルーム詳細＋現在の商品状態
func (c *LiveController) GetRoom(w http.ResponseWriter, r *http.Request) {
	roomId, ok := pathId(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "無効なルームIDです")
		return
	}
	detail, err := c.dao.FindRoom(roomId)
	if err != nil {
		handleDaoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// POST /api/live/rooms/{id}/start — 配信開始（Livekitトークン発行）
func (c *LiveController) StartRoom(w http.ResponseWriter, r *http.Request) {
	uid := mid.UserId(r)
	roomId, ok := pathId(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "無効なルームIDです")
		return
	}

	// Livekit ルーム名を生成（room_id ベース）
	livekitRoom := fmt.Sprintf("loopa-live-%d", roomId)

	if err := c.dao.StartRoom(roomId, uid, livekitRoom); err != nil {
		handleDaoError(w, err)
		return
	}

	// 配信者用トークン生成
	token, err := generateLivekitToken(livekitRoom, uid, true)
	if err != nil {
		log.Printf("live: livekit token error: %v", err)
		// トークン生成に失敗してもルームは開始済み（映像なしで続行）
		token = ""
	}

	// 最初の商品のオークションタイマーを開始
	first, err := c.dao.GetCurrentProduct(roomId)
	if err == nil && first != nil && first.Mode == "auction" {
		c.hub.startAuctionTimer(c, roomId, first.Id)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"livekit_url":   os.Getenv("LIVEKIT_URL"),
		"livekit_token": token,
		"room_name":     livekitRoom,
	})
}

// POST /api/live/rooms/{id}/end — 配信終了
func (c *LiveController) EndRoom(w http.ResponseWriter, r *http.Request) {
	uid := mid.UserId(r)
	roomId, ok := pathId(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "無効なルームIDです")
		return
	}
	c.hub.stopTimer(roomId)
	if err := c.dao.EndRoom(roomId, uid); err != nil {
		handleDaoError(w, err)
		return
	}
	c.hub.broadcast(roomId, model.LiveEvent{Type: "end"})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ended"})
}

// GET /api/live/rooms/{id}/token — 視聴者用Livekitトークン
func (c *LiveController) GetViewerToken(w http.ResponseWriter, r *http.Request) {
	uid := mid.UserId(r)
	roomId, ok := pathId(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "無効なルームIDです")
		return
	}

	livekitRoom := fmt.Sprintf("loopa-live-%d", roomId)
	token, err := generateLivekitToken(livekitRoom, uid, false)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "Livekitトークンの生成に失敗しました")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"livekit_url":   os.Getenv("LIVEKIT_URL"),
		"livekit_token": token,
		"room_name":     livekitRoom,
	})
}

// POST /api/live/rooms/{id}/bid — 入札
func (c *LiveController) Bid(w http.ResponseWriter, r *http.Request) {
	uid := mid.UserId(r)
	roomId, ok := pathId(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "無効なルームIDです")
		return
	}
	var req struct {
		Amount int `json:"amount"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Amount <= 0 {
		writeError(w, http.StatusBadRequest, "入札額は1円以上を指定してください")
		return
	}

	updated, err := c.dao.PlaceBid(roomId, uid, req.Amount)
	if err != nil {
		handleDaoError(w, err)
		return
	}

	// タイマーリセット
	c.hub.resetTimer(c, roomId, updated.Id)

	// 入札者名を取得してブロードキャスト
	updated.BidderName = c.dao.GetBidderName(uid)
	c.hub.broadcast(roomId, model.LiveEvent{
		Type:        "bid",
		Product:     updated,
		SecondsLeft: 30,
	})

	writeJSON(w, http.StatusOK, updated)
}

// POST /api/live/rooms/{id}/buy — 即決購入
func (c *LiveController) InstantBuy(w http.ResponseWriter, r *http.Request) {
	uid := mid.UserId(r)
	roomId, ok := pathId(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "無効なルームIDです")
		return
	}

	c.hub.stopTimer(roomId) // オークションタイマーが走っていれば止める

	sold, err := c.dao.InstantBuy(roomId, uid)
	if err != nil {
		handleDaoError(w, err)
		return
	}

	buyerName := c.dao.GetBidderName(uid)
	c.hub.broadcast(roomId, model.LiveEvent{
		Type:       "sold",
		Product:    sold,
		FinalPrice: sold.CurrentPrice,
		BuyerName:  buyerName,
	})

	// 3秒後に次へ
	go func() {
		time.Sleep(3 * time.Second)
		c.advanceToNext(roomId)
	}()

	writeJSON(w, http.StatusOK, sold)
}

// POST /api/live/rooms/{id}/next — 配信者が手動で次の商品へ
func (c *LiveController) Next(w http.ResponseWriter, r *http.Request) {
	uid := mid.UserId(r)
	roomId, ok := pathId(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "無効なルームIDです")
		return
	}

	// 権限チェック（配信者本人のみ）
	detail, err := c.dao.FindRoom(roomId)
	if err != nil {
		handleDaoError(w, err)
		return
	}
	if detail.SellerId != uid {
		writeError(w, http.StatusForbidden, "配信者のみ操作できます")
		return
	}

	c.hub.stopTimer(roomId)
	c.advanceToNext(roomId)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GET /api/live/rooms/{id}/events — SSE（視聴者がここに接続してリアルタイム更新を受け取る）
func (c *LiveController) Subscribe(w http.ResponseWriter, r *http.Request) {
	roomId, ok := pathId(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "無効なルームIDです")
		return
	}

	// SSEヘッダー
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	ch := c.hub.subscribe(roomId)
	defer c.hub.unsubscribe(roomId, ch)

	// 視聴者数インクリメント
	_ = c.dao.ChangeViewerCount(roomId, +1)
	defer func() { _ = c.dao.ChangeViewerCount(roomId, -1) }()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case data := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// ── Livekitトークン生成 ──────────────────────────────────────────

func generateLivekitToken(roomName, identity string, isPublisher bool) (string, error) {
	apiKey := os.Getenv("LIVEKIT_API_KEY")
	apiSecret := os.Getenv("LIVEKIT_API_SECRET")
	if apiKey == "" || apiSecret == "" {
		return "", fmt.Errorf("LIVEKIT_API_KEY / LIVEKIT_API_SECRET が未設定")
	}

	canPublish := isPublisher
	canSubscribe := true

	at := auth.NewAccessToken(apiKey, apiSecret)
	grant := &auth.VideoGrant{
		RoomJoin:     true,
		Room:         roomName,
		CanPublish:   &canPublish,
		CanSubscribe: &canSubscribe,
	}
	at.SetVideoGrant(grant).
		SetIdentity(identity).
		SetValidFor(3 * time.Hour)

	return at.ToJWT()
}
