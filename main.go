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
	// 1. 🌍 環境変数の読み込み
	mysqlUser := os.Getenv("MYSQL_USER")
	mysqlPwd := os.Getenv("MYSQL_PWD")
	mysqlHost := os.Getenv("MYSQL_HOST")
	mysqlDatabase := os.Getenv("MYSQL_DATABASE")

	// 2. 🔑 【SSL証明書の登録】
	// ファイルが存在する場合のみローカル用SSL設定を通す（Cloud Run上では安全にスキップされる）
	rootCertPool := x509.NewCertPool()
	pem, err := os.ReadFile("server-ca.pem")

	var connStr string
	if err != nil {
		// ☁️ 【本番（Cloud Run）環境用の接続文字列】
		// 証明書ファイルがない場合は、通常の接続文字列を組み立てる
		log.Println("💡 server-ca.pem が見つからないため、通常の接続を試みます（Cloud Run環境）")
		connStr = fmt.Sprintf("%s:%s@tcp(%s:3306)/%s", mysqlUser, mysqlPwd, mysqlHost, mysqlDatabase)
	} else {
		// 💻 【ローカル環境用の接続文字列】
		rootCertPool.AppendCertsFromPEM(pem)

		clientCert := make([]tls.Certificate, 0, 1)
		certs, err := tls.LoadX509KeyPair("client-cert.pem", "client-key.pem")
		if err != nil {
			log.Println("⚠️ ローカル証明書の読み込みに失敗しました（処理は続行します）:", err)
		}
		clientCert = append(clientCert, certs)

		// 「custom-tls」という名前で登録
		_ = mysql.RegisterTLSConfig("custom-tls", &tls.Config{
			RootCAs:            rootCertPool,
			Certificates:       clientCert,
			InsecureSkipVerify: true,
		})

		// 末尾に `?tls=custom-tls` をつける
		connStr = fmt.Sprintf("%s:%s@tcp(%s:3306)/%s?tls=custom-tls", mysqlUser, mysqlPwd, mysqlHost, mysqlDatabase)
	}

	// 3. DB接続開始
	db, err = sql.Open("mysql", connStr)
	if err != nil {
		log.Println("⚠️ WARNING: sql.Open failed:", err)
	}

	// 💡 Cloud Runの起動を邪魔しないよう、Ping失敗でも log.Fatal は絶対にさせない
	if err := db.Ping(); err != nil {
		log.Println("⚠️ WARNING: db.Ping failed:", err)
	} else {
		log.Println("✅ SUCCESS: Database connected successfully!!!")
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
