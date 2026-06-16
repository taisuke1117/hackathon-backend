package router

import (
	"main/controller"
	"main/middleware"
	"net/http"
)

// Setup: 全エンドポイントの登録（CORS→Auth→/api/ の順でミドルウェアが適用される）

func Setup(
	userCtrl *controller.UserController,
	productCtrl *controller.ProductController,
	chatCtrl *controller.ChatController,
	notificationCtrl *controller.NotificationController,
	geminiCtrl *controller.GeminiController,
	liveCtrl *controller.LiveController,
) http.Handler {

	// mux: 認証が必要なAPIエンドポイントを登録するルーター
	// Go 1.22以降の新しいルーティング記法: "メソッド /パス" の形で書ける
	mux := http.NewServeMux()

	// ── 起動時一括取得 ────────────────────────────────────────
	// フロントのAuthContextが最初に呼ぶ。ユーザー情報・カテゴリ・いいね済みIDをまとめて返す
	mux.HandleFunc("GET /api/init", userCtrl.Init)

	// ── ユーザー ──────────────────────────────────────────────
	mux.HandleFunc("POST /api/users", userCtrl.Register)         // 初回ログイン時の登録（既存ならUPDATE）
	mux.HandleFunc("GET /api/users/me", userCtrl.Me)             // 自分のプロフィール取得
	mux.HandleFunc("PUT /api/users/me", userCtrl.UpdateMe)       // 自分のプロフィール更新
	mux.HandleFunc("GET /api/users/{id}", userCtrl.Profile)      // 他ユーザーのプロフィール表示
	mux.HandleFunc("GET /api/users/{id}/reviews", productCtrl.UserReviews) // ユーザーが受けた評価一覧
	mux.HandleFunc("POST /api/blocks", userCtrl.Block)           // ユーザーブロック

	// ── 商品 ──────────────────────────────────────────────────
	mux.HandleFunc("GET /api/products", productCtrl.List)                   // 一覧・検索
	mux.HandleFunc("POST /api/products", productCtrl.Create)                // 出品
	mux.HandleFunc("GET /api/products/{id}", productCtrl.Detail)            // 詳細
	mux.HandleFunc("PUT /api/products/{id}", productCtrl.Update)            // 編集
	mux.HandleFunc("DELETE /api/products/{id}", productCtrl.Delete)         // 出品取り消し
	mux.HandleFunc("POST /api/products/{id}/purchase", productCtrl.Purchase) // 購入確定
	mux.HandleFunc("PUT /api/products/{id}/ship", productCtrl.Ship)         // 発送完了
	mux.HandleFunc("POST /api/products/{id}/like", productCtrl.Like)        // いいね
	mux.HandleFunc("DELETE /api/products/{id}/like", productCtrl.Unlike)    // いいね解除
	mux.HandleFunc("POST /api/products/{id}/reviews", productCtrl.CreateReview) // 受取評価

	// ── マイページ ────────────────────────────────────────────
	mux.HandleFunc("GET /api/me/badges", userCtrl.Badges)          // 未読バッジ数（25秒ごとに叩く）
	mux.HandleFunc("GET /api/me/products", productCtrl.MyProducts)  // 自分の出品一覧
	mux.HandleFunc("GET /api/me/purchases", productCtrl.MyPurchases) // 購入履歴
	mux.HandleFunc("GET /api/me/likes", productCtrl.MyLikes)        // いいね一覧

	// ── チャット・値引き交渉 ───────────────────────────────────
	mux.HandleFunc("POST /api/chatrooms", chatCtrl.CreateRoom)                         // ルーム取得or作成
	mux.HandleFunc("GET /api/chatrooms", chatCtrl.ListRooms)                           // ルーム一覧
	mux.HandleFunc("GET /api/chatrooms/{id}", chatCtrl.RoomDetail)                     // ルーム詳細＋メッセージ
	mux.HandleFunc("POST /api/chatrooms/{id}/messages", chatCtrl.SendMessage)          // メッセージ送信
	mux.HandleFunc("PUT /api/chatrooms/{id}/read", chatCtrl.MarkRead)                  // 既読化
	mux.HandleFunc("POST /api/chatrooms/{id}/discount", chatCtrl.ProposeDiscount)      // 値引き提案（購入者）
	mux.HandleFunc("PUT /api/chatrooms/{id}/discount/approve", chatCtrl.ApproveDiscount) // 値引き承認（出品者）

	// ── 通知 ──────────────────────────────────────────────────
	mux.HandleFunc("GET /api/notifications", notificationCtrl.List)              // 通知一覧
	mux.HandleFunc("PUT /api/notifications/{id}/read", notificationCtrl.MarkRead) // 既読

	// ── Gemini AI連携 ─────────────────────────────────────────
	mux.HandleFunc("POST /api/gemini/listing", geminiCtrl.AnalyzeListing) // 画像→出品情報自動生成
	mux.HandleFunc("POST /api/gemini/reply", geminiCtrl.GenerateReply)    // チャット返信文の提案
	mux.HandleFunc("POST /api/gemini/search", geminiCtrl.AiSearch)        // 自然文→検索条件変換

	// ── ライブ配信 ────────────────────────────────────────────
	mux.HandleFunc("POST /api/live/rooms", liveCtrl.CreateRoom)                   // ルーム作成
	mux.HandleFunc("GET /api/live/rooms", liveCtrl.ListRooms)                     // 配信中一覧
	mux.HandleFunc("GET /api/live/rooms/{id}", liveCtrl.GetRoom)                  // ルーム詳細
	mux.HandleFunc("POST /api/live/rooms/{id}/start", liveCtrl.StartRoom)         // 配信開始
	mux.HandleFunc("POST /api/live/rooms/{id}/end", liveCtrl.EndRoom)             // 配信終了
	mux.HandleFunc("GET /api/live/rooms/{id}/token", liveCtrl.GetViewerToken)     // 視聴者トークン
	mux.HandleFunc("POST /api/live/rooms/{id}/bid", liveCtrl.Bid)                 // 入札
	mux.HandleFunc("POST /api/live/rooms/{id}/buy", liveCtrl.InstantBuy)          // 即決購入
	mux.HandleFunc("POST /api/live/rooms/{id}/next", liveCtrl.Next)               // 次の商品へ
	mux.HandleFunc("GET /api/live/rooms/{id}/events", liveCtrl.Subscribe)         // SSE接続
	mux.HandleFunc("POST /api/live/rooms/{id}/comment", liveCtrl.PostComment)     // チャットコメント投稿
	mux.HandleFunc("GET /api/live/rooms/{id}/comments", liveCtrl.GetComments)     // チャット過去ログ

	// ── 画像アップロード ──────────────────────────────────────
	// GCS（Google Cloud Storage）に画像を保存してURLを返す
	mux.HandleFunc("POST /api/images", controller.UploadImage)

	// ── 認証不要のエンドポイント ──────────────────────────────
	// root は Auth ミドルウェアの外側に配置する
	root := http.NewServeMux()

	// ヘルスチェック: Cloud RunがコンテナのOKを確認するために叩く
	root.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// 画像の配信: GCSから取得した画像をそのまま返す（認証不要で誰でも閲覧可能）
	root.HandleFunc("GET /images/{object...}", controller.ServeImage)

	// /api/ 以下は Auth ミドルウェアを通す
	root.Handle("/api/", middleware.Auth(mux))

	// 一番外側に CORS ミドルウェアを巻く（全リクエストに適用）
	return middleware.CORS(root)
}
