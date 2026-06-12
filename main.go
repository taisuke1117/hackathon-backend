package main

import (
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"fmt"
	"log"
	"main/controller"
	"main/dao"
	"main/middleware"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/go-sql-driver/mysql"
)

var db *sql.DB

func init() {
	mysqlUser := os.Getenv("MYSQL_USER")
	mysqlPwd := os.Getenv("MYSQL_PWD")
	mysqlHost := os.Getenv("MYSQL_HOST")
	mysqlDatabase := os.Getenv("MYSQL_DATABASE")

	// parseTime=true: TIMESTAMP列をtime.Timeで受け取るために必須
	// DBサーバーはUTCで保存しているので、locは指定せずUTCとして読み取る（表示変換はフロント側）
	const dsnParams = "parseTime=true&charset=utf8mb4"

	var connStr string

	// 本番（Unixソケット）かローカル（TCP）かをMYSQL_HOSTの形式で判別する
	if strings.HasPrefix(mysqlHost, "unix(") {
		socketPath := strings.TrimSuffix(strings.TrimPrefix(mysqlHost, "unix("), ")")
		connStr = fmt.Sprintf("%s:%s@unix(%s)/%s?%s", mysqlUser, mysqlPwd, socketPath, mysqlDatabase, dsnParams)
		log.Println("☁️ Cloud Run本番環境: Unixドメインソケット接続ルートを使用します")
	} else {
		rootCertPool := x509.NewCertPool()
		pem, err := os.ReadFile("server-ca.pem")
		if err != nil {
			log.Fatal("❌ [LOCAL ERROR] ローカル起動ですが server-ca.pem が見つかりません。現在のMYSQL_HOST: ", mysqlHost)
		}

		rootCertPool.AppendCertsFromPEM(pem)
		certs, err := tls.LoadX509KeyPair("client-cert.pem", "client-key.pem")
		if err != nil {
			log.Fatal("❌ [LOCAL ERROR] client-cert.pem または client-key.pem の読み込みに失敗しました")
		}

		_ = mysql.RegisterTLSConfig("custom-tls", &tls.Config{
			RootCAs:            rootCertPool,
			Certificates:       []tls.Certificate{certs},
			InsecureSkipVerify: true,
		})

		connStr = fmt.Sprintf("%s:%s@tcp(%s:3306)/%s?tls=custom-tls&%s", mysqlUser, mysqlPwd, mysqlHost, mysqlDatabase, dsnParams)
		log.Println("💻 ローカル環境: TCP + SSL接続ルートを使用します")
	}

	var err error
	db, err = sql.Open("mysql", connStr)
	if err != nil {
		log.Fatal("❌ [CRITICAL] sql.Open failed: ", err)
	}

	if err := db.Ping(); err != nil {
		log.Fatal("❌ [CRITICAL] データベースへの疎通確認（Ping）に失敗しました: ", err)
	}
	log.Println("✅ [SUCCESS] データベースとの接続に成功しました")
}

func main() {
	// DAO層の初期化
	userDao := dao.NewUserDao(db)
	productDao := dao.NewProductDao(db)
	chatDao := dao.NewChatDao(db)
	notificationDao := dao.NewNotificationDao(db)
	reviewDao := dao.NewReviewDao(db)

	// コントローラの初期化
	userCtrl := controller.NewUserController(userDao, productDao, reviewDao)
	productCtrl := controller.NewProductController(productDao, notificationDao, reviewDao, userDao)
	chatCtrl := controller.NewChatController(chatDao, notificationDao)
	notificationCtrl := controller.NewNotificationController(notificationDao)
	geminiCtrl := controller.NewGeminiController(userDao)

	// ルーティング
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

	// ヘルスチェック（認証不要にするためAuthの外側で配線）
	root := http.NewServeMux()
	root.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	// 画像配信は<img>タグから直接参照されるため認証不要
	root.HandleFunc("GET /images/{object...}", controller.ServeImage)
	root.Handle("/api/", middleware.Auth(mux))

	handler := middleware.CORS(root)

	closeDBWithSysCall()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Listening on :%s...\n", port)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatal(err)
	}
}

func closeDBWithSysCall() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		s := <-sig
		log.Printf("received syscall, %v", s)

		if err := db.Close(); err != nil {
			log.Fatal(err)
		}
		log.Printf("success: db.Close()")
		os.Exit(0)
	}()
}
