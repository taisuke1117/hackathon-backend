package main

import (
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

	_ "github.com/go-sql-driver/mysql"
)

var db *sql.DB

func init() {
	mysqlUser := os.Getenv("MYSQL_USER")
	mysqlPwd := os.Getenv("MYSQL_PWD")
	mysqlHost := os.Getenv("MYSQL_HOST")
	mysqlDatabase := os.Getenv("MYSQL_DATABASE")

	var connStr string
	// MYSQL_HOSTが「unix(」から始まっている場合はソケット通信、そうでない場合は通常のTCPとして処理
	if len(mysqlHost) > 5 && mysqlHost[:5] == "unix(" {
		// ソケット通信用: user:password@unix(/cloudsql/connection-name)/dbname
		connStr = fmt.Sprintf("%s:%s@%s/%s", mysqlUser, mysqlPwd, mysqlHost, mysqlDatabase)
	} else {
		// ローカル開発などのTCP用: user:password@tcp(host:port)/dbname
		connStr = fmt.Sprintf("%s:%s@tcp(%s)/%s", mysqlUser, mysqlPwd, mysqlHost, mysqlDatabase)
	}

	var err error
	db, err = sql.Open("mysql", connStr)
	if err != nil {
		log.Fatalf("fail: sql.Open, %v\n", err)
	}

	if err := db.Ping(); err != nil {
		log.Fatalf("fail: _db.Ping, %v\n", err)
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
