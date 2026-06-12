package db

import (
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/go-sql-driver/mysql"
)

// Connect DB接続を確立して返す。接続失敗はFatal
func Connect() *sql.DB {
	mysqlUser := os.Getenv("MYSQL_USER")
	mysqlPwd := os.Getenv("MYSQL_PWD")
	mysqlHost := os.Getenv("MYSQL_HOST")
	mysqlDatabase := os.Getenv("MYSQL_DATABASE")

	// parseTime=true: TIMESTAMP列をtime.Timeで受け取るために必須
	const dsnParams = "parseTime=true&charset=utf8mb4"

	var connStr string

	if strings.HasPrefix(mysqlHost, "unix(") {
		socketPath := strings.TrimSuffix(strings.TrimPrefix(mysqlHost, "unix("), ")")
		connStr = fmt.Sprintf("%s:%s@unix(%s)/%s?%s", mysqlUser, mysqlPwd, socketPath, mysqlDatabase, dsnParams)
		log.Println("☁️ Cloud Run本番環境: Unixドメインソケット接続")
	} else {
		rootCertPool := x509.NewCertPool()
		pem, err := os.ReadFile("server-ca.pem")
		if err != nil {
			log.Fatal("❌ server-ca.pem が見つかりません: ", err)
		}
		rootCertPool.AppendCertsFromPEM(pem)
		certs, err := tls.LoadX509KeyPair("client-cert.pem", "client-key.pem")
		if err != nil {
			log.Fatal("❌ クライアント証明書の読み込みに失敗: ", err)
		}
		_ = mysql.RegisterTLSConfig("custom-tls", &tls.Config{
			RootCAs:            rootCertPool,
			Certificates:       []tls.Certificate{certs},
			InsecureSkipVerify: true,
		})
		connStr = fmt.Sprintf("%s:%s@tcp(%s:3306)/%s?tls=custom-tls&%s", mysqlUser, mysqlPwd, mysqlHost, mysqlDatabase, dsnParams)
		log.Println("💻 ローカル環境: TCP + SSL接続")
	}

	db, err := sql.Open("mysql", connStr)
	if err != nil {
		log.Fatal("❌ sql.Open failed: ", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatal("❌ DB Ping失敗: ", err)
	}
	log.Println("✅ DB接続成功")
	return db
}
