package controller

import (
	"database/sql"
	"main/dao"
	"main/middleware"
	"main/model"
	"net/http"
)

type UserController struct {
	userDao    *dao.UserDao
	productDao *dao.ProductDao
	reviewDao  *dao.ReviewDao
}

func NewUserController(userDao *dao.UserDao, productDao *dao.ProductDao, reviewDao *dao.ReviewDao) *UserController {
	return &UserController{userDao: userDao, productDao: productDao, reviewDao: reviewDao}
}

// Register POST /api/users — 初回ログイン時のユーザー登録（再ログイン時は名前を同期）
func (c *UserController) Register(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserId(r)
	var req model.RegisterUserRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Name == "" || req.Mail == "" {
		writeError(w, http.StatusBadRequest, "nameとmailは必須です")
		return
	}
	if err := c.userDao.Upsert(uid, &req); err != nil {
		handleDaoError(w, err)
		return
	}
	user, err := c.userDao.FindById(uid)
	if err != nil {
		handleDaoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// Me GET /api/users/me — 自分のプロフィール
func (c *UserController) Me(w http.ResponseWriter, r *http.Request) {
	user, err := c.userDao.FindById(middleware.UserId(r))
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "ユーザーが未登録です。POST /api/users で登録してください")
		return
	}
	if err != nil {
		handleDaoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// UpdateMe PUT /api/users/me — プロフィール更新
func (c *UserController) UpdateMe(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserId(r)
	var req model.UpdateUserRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "nameは必須です")
		return
	}
	if err := c.userDao.Update(uid, &req); err != nil {
		handleDaoError(w, err)
		return
	}
	user, err := c.userDao.FindById(uid)
	if err != nil {
		handleDaoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// Profile GET /api/users/{id} — 公開プロフィール（出品商品・受け取った評価つき）
func (c *UserController) Profile(w http.ResponseWriter, r *http.Request) {
	targetId := r.PathValue("id")
	user, err := c.userDao.FindById(targetId)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "ユーザーが見つかりません")
		return
	}
	if err != nil {
		handleDaoError(w, err)
		return
	}
	// 公開プロフィールでは配送先住所とメールは隠す
	user.ShippingAddress = ""
	user.Mail = ""

	products, err := c.productDao.FindBySeller(targetId)
	if err != nil {
		handleDaoError(w, err)
		return
	}
	reviews, err := c.reviewDao.ListByReviewee(targetId)
	if err != nil {
		handleDaoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":     user,
		"products": products,
		"reviews":  reviews,
	})
}

// Block POST /api/blocks — ユーザーをブロック
func (c *UserController) Block(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BlockedId string `json:"blocked_id"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.BlockedId == "" {
		writeError(w, http.StatusBadRequest, "blocked_idは必須です")
		return
	}
	if err := c.userDao.Block(middleware.UserId(r), req.BlockedId); err != nil {
		handleDaoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "blocked"})
}

// Badges GET /api/me/badges — 未読の通知数・チャット数（ヘッダー/フッターのバッジ用）
func (c *UserController) Badges(w http.ResponseWriter, r *http.Request) {
	notifications, chats, err := c.userDao.GetBadges(middleware.UserId(r))
	if err != nil {
		handleDaoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{
		"unread_notifications": notifications,
		"unread_chats":         chats,
	})
}

// Init GET /api/init — アプリ起動時の一括取得
func (c *UserController) Init(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserId(r)

	user, err := c.userDao.FindById(uid)
	if err == sql.ErrNoRows {
		user = nil // 未登録でもアプリは起動させる（フロントが登録APIを呼ぶ）
	} else if err != nil {
		handleDaoError(w, err)
		return
	}

	categories, err := c.userDao.GetAllCategories()
	if err != nil {
		handleDaoError(w, err)
		return
	}
	notificationDao := dao.NewNotificationDao(c.userDao.DB)
	notifications, err := notificationDao.ListByUser(uid)
	if err != nil {
		handleDaoError(w, err)
		return
	}
	likedIds, err := c.userDao.GetLikedProductIds(uid)
	if err != nil {
		handleDaoError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, &model.AppInitResponse{
		User:            user,
		Categories:      categories,
		Notifications:   notifications,
		LikedProductIds: likedIds,
	})
}
