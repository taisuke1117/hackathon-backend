package router

import (
	"main/controller"
	"main/middleware"
	"net/http"
)

// Setup 全ルートを登録してハンドラを返す
func Setup(
	userCtrl *controller.UserController,
	productCtrl *controller.ProductController,
	chatCtrl *controller.ChatController,
	notificationCtrl *controller.NotificationController,
	geminiCtrl *controller.GeminiController,
) http.Handler {
	mux := http.NewServeMux()

	// 起動時一括取得
	mux.HandleFunc("GET /api/init", userCtrl.Init)

	// ユーザー
	mux.HandleFunc("POST /api/users", userCtrl.Register)
	mux.HandleFunc("GET /api/users/me", userCtrl.Me)
	mux.HandleFunc("PUT /api/users/me", userCtrl.UpdateMe)
	mux.HandleFunc("GET /api/users/{id}", userCtrl.Profile)
	mux.HandleFunc("GET /api/users/{id}/reviews", productCtrl.UserReviews)
	mux.HandleFunc("POST /api/blocks", userCtrl.Block)

	// 商品
	mux.HandleFunc("GET /api/products", productCtrl.List)
	mux.HandleFunc("POST /api/products", productCtrl.Create)
	mux.HandleFunc("GET /api/products/{id}", productCtrl.Detail)
	mux.HandleFunc("PUT /api/products/{id}", productCtrl.Update)
	mux.HandleFunc("DELETE /api/products/{id}", productCtrl.Delete)
	mux.HandleFunc("POST /api/products/{id}/purchase", productCtrl.Purchase)
	mux.HandleFunc("PUT /api/products/{id}/ship", productCtrl.Ship)
	mux.HandleFunc("POST /api/products/{id}/like", productCtrl.Like)
	mux.HandleFunc("DELETE /api/products/{id}/like", productCtrl.Unlike)
	mux.HandleFunc("POST /api/products/{id}/reviews", productCtrl.CreateReview)

	// マイページ系
	mux.HandleFunc("GET /api/me/badges", userCtrl.Badges)
	mux.HandleFunc("GET /api/me/products", productCtrl.MyProducts)
	mux.HandleFunc("GET /api/me/purchases", productCtrl.MyPurchases)
	mux.HandleFunc("GET /api/me/likes", productCtrl.MyLikes)

	// チャット・値引き交渉
	mux.HandleFunc("POST /api/chatrooms", chatCtrl.CreateRoom)
	mux.HandleFunc("GET /api/chatrooms", chatCtrl.ListRooms)
	mux.HandleFunc("GET /api/chatrooms/{id}", chatCtrl.RoomDetail)
	mux.HandleFunc("POST /api/chatrooms/{id}/messages", chatCtrl.SendMessage)
	mux.HandleFunc("PUT /api/chatrooms/{id}/read", chatCtrl.MarkRead)
	mux.HandleFunc("POST /api/chatrooms/{id}/discount", chatCtrl.ProposeDiscount)
	mux.HandleFunc("PUT /api/chatrooms/{id}/discount/approve", chatCtrl.ApproveDiscount)

	// 通知
	mux.HandleFunc("GET /api/notifications", notificationCtrl.List)
	mux.HandleFunc("PUT /api/notifications/{id}/read", notificationCtrl.MarkRead)

	// Gemini連携
	mux.HandleFunc("POST /api/gemini/listing", geminiCtrl.AnalyzeListing)
	mux.HandleFunc("POST /api/gemini/reply", geminiCtrl.GenerateReply)
	mux.HandleFunc("POST /api/gemini/search", geminiCtrl.AiSearch)

	// 画像アップロード（GCSへ保存）
	mux.HandleFunc("POST /api/images", controller.UploadImage)

	// 認証不要エンドポイント（Auth middlewareの外側）
	root := http.NewServeMux()
	root.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	root.HandleFunc("GET /images/{object...}", controller.ServeImage)
	root.Handle("/api/", middleware.Auth(mux))

	return middleware.CORS(root)
}
