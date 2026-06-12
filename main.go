package main

import (
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"fmt"
	"log"
	"main/controller"
	"main/dao"
	"main/usecase"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-sql-driver/mysql"
	_ "github.com/go-sql-driver/mysql"
)

var db *sql.DB

func init() {
	// 1. 🔑 【SSL証明書の登録】（これはローカル接続に必須です）
	rootCertPool := x509.NewCertPool()
	pem, err := os.ReadFile("server-ca.pem")
	if err != nil {
		log.Println("💡 server-ca.pem なし（Cloud Run環境の可能性あり。処理を続行します）")
	} else {
		rootCertPool.AppendCertsFromPEM(pem)

		clientCert := make([]tls.Certificate, 0, 1)
		certs, err := tls.LoadX509KeyPair("client-cert.pem", "client-key.pem")
		if err != nil {
			log.Fatal("❌ client-cert.pem または client-key.pem の読み込みに失敗:", err)
		}
		clientCert = append(clientCert, certs)

		// 「custom-tls」という名前で登録
		_ = mysql.RegisterTLSConfig("custom-tls", &tls.Config{
			RootCAs:            rootCertPool,
			Certificates:       clientCert,
			InsecureSkipVerify: true,
		})
	}

	// 2. 🌍 【環境変数の読み込み】（元の綺麗な形に戻します！）
	mysqlUser := os.Getenv("MYSQL_USER")
	mysqlPwd := os.Getenv("MYSQL_PWD")
	mysqlHost := os.Getenv("MYSQL_HOST") // 例: 35.226.67.117
	mysqlDatabase := os.Getenv("MYSQL_DATABASE")

	// 3. 🔗 接続文字列の組み立て
	// 末尾に `?tls=custom-tls` をつけてSSLを強制します
	connStr := fmt.Sprintf("%s:%s@tcp(%s:3306)/%s?tls=custom-tls", mysqlUser, mysqlPwd, mysqlHost, mysqlDatabase)

	// 4. DB接続開始
	db, err = sql.Open("mysql", connStr)
	if err != nil {
		log.Println("⚠️ WARNING: sql.Open failed:", err)
	}

	if err := db.Ping(); err != nil {
		log.Println("⚠️ WARNING: db.Ping failed:", err)
	} else {
		log.Println("✅ SUCCESS: Database connected via SSL using Environment Variables!!!")
	}
	_ = db
}

func main() {
	// 1. 各レイヤの依存関係を初期化 (DI)
	userDao := dao.NewUserDao(db)

	searchUsecase := usecase.NewSearchUserUsecase(userDao)
	registerUsecase := usecase.NewRegisterUserUsecase(db, userDao)

	searchController := controller.NewSearchUserController(searchUsecase)
	registerController := controller.NewRegisterUserController(registerUsecase)

	// 2. ルーティングの設定
	// メソッドごとにハンドラーを切り替えるラッパー
	http.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			searchController.Handle(w, r)
		case http.MethodPost:
			registerController.Handle(w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	closeDBWithSysCall()

	// 1. 環境変数 PORT からポート番号を取得（無ければデフォルトで 8080）
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Listening on :%s...\n", port)

	// 2. 文字列をガッチャンコして、取得したポートで起動
	if err := http.ListenAndServe(":"+port, nil); err != nil {
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
