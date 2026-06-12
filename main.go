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
	"strings"
	"syscall"

	"github.com/go-sql-driver/mysql"
	_ "github.com/go-sql-driver/mysql"
)

var db *sql.DB

func init() {
	// 1. 環境変数の取得
	mysqlUser := os.Getenv("MYSQL_USER")
	mysqlPwd := os.Getenv("MYSQL_PWD")
	mysqlHost := os.Getenv("MYSQL_HOST")
	mysqlDatabase := os.Getenv("MYSQL_DATABASE")

	var connStr string

	// 2. 🌍 本番（Unixソケット）かローカル（TCP）かを100%正確に判別する
	if strings.HasPrefix(mysqlHost, "unix(") {
		// ☁️ 【本番（Cloud Run）環境】
		// unix(/cloudsql/term9-taisuke-ohmori:us-central1:uttc) からパスだけを抜き出す
		socketPath := strings.TrimSuffix(strings.TrimPrefix(mysqlHost, "unix("), ")")

		// 本番環境はGCPの内部ネットワークなので、SSL証明書(pem)の読み込みは一切不要です！
		connStr = fmt.Sprintf("%s:%s@unix(%s)/%s", mysqlUser, mysqlPwd, socketPath, mysqlDatabase)
		log.Println("☁️ Cloud Run本番環境: Unixドメインソケット接続ルートを使用します")
	} else {
		// 💻 【ローカル環境】 TCP接続 + SSL証明書必須
		rootCertPool := x509.NewCertPool()
		pem, err := os.ReadFile("server-ca.pem")
		if err != nil {
			log.Fatal("❌ [LOCAL ERROR] ローカル起動ですが server-ca.pem が見つかりません。現在のMYSQL_HOST: ", mysqlHost)
		}

		rootCertPool.AppendCertsFromPEM(pem)
		clientCert := make([]tls.Certificate, 0, 1)
		certs, err := tls.LoadX509KeyPair("client-cert.pem", "client-key.pem")
		if err != nil {
			log.Fatal("❌ [LOCAL ERROR] client-cert.pem または client-key.pem の読み込みに失敗しました")
		}
		clientCert = append(clientCert, certs)

		_ = mysql.RegisterTLSConfig("custom-tls", &tls.Config{
			RootCAs:            rootCertPool,
			Certificates:       clientCert,
			InsecureSkipVerify: true,
		})

		connStr = fmt.Sprintf("%s:%s@tcp(%s:3306)/%s?tls=custom-tls", mysqlUser, mysqlPwd, mysqlHost, mysqlDatabase)
		log.Println("💻 ローカル環境: TCP + SSL接続ルートを使用します")
	}

	// 3. DB接続開始
	var err error
	db, err = sql.Open("mysql", connStr)
	if err != nil {
		log.Fatal("❌ [CRITICAL] sql.Open failed: ", err)
	}

	// 4. 疎通確認（エラーなら即死）
	if err := db.Ping(); err != nil {
		log.Fatal("❌ [CRITICAL] データベースへの疎通確認（Ping）に失敗しました。接続文字列を確認してください: ", err)
	} else {
		log.Println("✅ [SUCCESS] データベースとの接続に完全成功しました！！！")
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
