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

	// 2. 本番（Unixソケット）かローカル（TCP）かを判別して文字列を組む
	if strings.HasPrefix(mysqlHost, "unix(") {
		// ☁️ 【本番（Cloud Run）環境】
		socketPath := strings.TrimSuffix(strings.TrimPrefix(mysqlHost, "unix("), ")")
		connStr = fmt.Sprintf("%s:%s@unix(%s)/%s", mysqlUser, mysqlPwd, socketPath, mysqlDatabase)
	} else {
		// 💻 【ローカル環境】 TCP接続
		rootCertPool := x509.NewCertPool()
		pem, err := os.ReadFile("server-ca.pem")

		if err != nil {
			// 💡 証明書がない場合は通常のTCP（もしローカルでSSLをオフにした場合用）
			connStr = fmt.Sprintf("%s:%s@tcp(%s:3306)/%s", mysqlUser, mysqlPwd, mysqlHost, mysqlDatabase)
		} else {
			// 🔒 証明書がある場合はSSLを強制
			rootCertPool.AppendCertsFromPEM(pem)
			clientCert := make([]tls.Certificate, 0, 1)
			certs, err := tls.LoadX509KeyPair("client-cert.pem", "client-key.pem")
			if err != nil {
				// 証明書の読み込み自体に失敗したら即死
				log.Fatal("❌ [CRITICAL] ローカル証明書の読み込みに失敗しました: ", err)
			}
			clientCert = append(clientCert, certs)

			_ = mysql.RegisterTLSConfig("custom-tls", &tls.Config{
				RootCAs:            rootCertPool,
				Certificates:       clientCert,
				InsecureSkipVerify: true,
			})
			connStr = fmt.Sprintf("%s:%s@tcp(%s:3306)/%s?tls=custom-tls", mysqlUser, mysqlPwd, mysqlHost, mysqlDatabase)
		}
	}

	// 3. DB接続開始
	var err error
	db, err = sql.Open("mysql", connStr)
	if err != nil {
		log.Fatal("❌ [CRITICAL] sql.Open failed: ", err)
	}

	// 🔴 4. 疎通確認（ここで繋がらなければローカルでも本番でも100%エラーで即死します）
	if err := db.Ping(); err != nil {
		log.Fatal("❌ [CRITICAL] データベースへの接続（Ping）に失敗したため、起動を停止します: ", err)
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
