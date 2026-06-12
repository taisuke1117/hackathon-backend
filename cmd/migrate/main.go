package main

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

// DBスキーマを最新状態に揃えるワンショットのマイグレーションスクリプト
// 実行: MYSQL_USER=... MYSQL_PWD=... MYSQL_HOST=... MYSQL_DATABASE=... go run ./cmd/migrate
func main() {
	mysqlUser := os.Getenv("MYSQL_USER")
	mysqlPwd := os.Getenv("MYSQL_PWD")
	mysqlHost := os.Getenv("MYSQL_HOST")
	mysqlDatabase := os.Getenv("MYSQL_DATABASE")

	var connStr string
	if strings.HasPrefix(mysqlHost, "unix(") {
		socketPath := strings.TrimSuffix(strings.TrimPrefix(mysqlHost, "unix("), ")")
		connStr = fmt.Sprintf("%s:%s@unix(%s)/%s?charset=utf8mb4", mysqlUser, mysqlPwd, socketPath, mysqlDatabase)
	} else {
		rootCertPool := x509.NewCertPool()
		pem, err := os.ReadFile("server-ca.pem")
		if err != nil {
			log.Fatal("server-ca.pem が見つかりません: ", err)
		}
		rootCertPool.AppendCertsFromPEM(pem)
		certs, err := tls.LoadX509KeyPair("client-cert.pem", "client-key.pem")
		if err != nil {
			log.Fatal("クライアント証明書の読み込みに失敗: ", err)
		}
		_ = mysql.RegisterTLSConfig("custom-tls", &tls.Config{
			RootCAs:            rootCertPool,
			Certificates:       []tls.Certificate{certs},
			InsecureSkipVerify: true,
		})
		connStr = fmt.Sprintf("%s:%s@tcp(%s:3306)/%s?tls=custom-tls&charset=utf8mb4", mysqlUser, mysqlPwd, mysqlHost, mysqlDatabase)
	}

	db, err := sql.Open("mysql", connStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatal("DB接続失敗: ", err)
	}
	log.Println("✅ DB接続成功。マイグレーション開始")

	statements := []struct {
		desc string
		sql  string
	}{
		{"product_images テーブル作成", `
			CREATE TABLE IF NOT EXISTS product_images (
				image_id   BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
				product_id BIGINT UNSIGNED NOT NULL,
				position   INT NOT NULL DEFAULT 0,
				url        TEXT NOT NULL,
				INDEX idx_product_images_product (product_id)
			)`},
		{"product_categories テーブル作成", `
			CREATE TABLE IF NOT EXISTS product_categories (
				product_id  BIGINT UNSIGNED NOT NULL,
				category_id BIGINT UNSIGNED NOT NULL,
				PRIMARY KEY (product_id, category_id)
			)`},
		{"reviews テーブル作成", `
			CREATE TABLE IF NOT EXISTS reviews (
				review_id   BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
				product_id  BIGINT UNSIGNED NOT NULL,
				reviewer_id VARCHAR(255) NOT NULL,
				reviewee_id VARCHAR(255) NOT NULL,
				rating      INT NOT NULL,
				comment     TEXT,
				created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				UNIQUE KEY uniq_reviews_product_reviewer (product_id, reviewer_id),
				INDEX idx_reviews_reviewee (reviewee_id)
			)`},
		{"products.tags カラム追加", `ALTER TABLE products ADD COLUMN tags TEXT`},
		{"products.condition カラム追加", `ALTER TABLE products ADD COLUMN condition VARCHAR(50)`},
		{"products.status デフォルトを英語化", `ALTER TABLE products ALTER COLUMN status SET DEFAULT 'available'`},
		{"既存statusを英語コードへ移行(出品中)", `UPDATE products SET status='available' WHERE status='出品中'`},
		{"既存statusを英語コードへ移行(未発送)", `UPDATE products SET status='unshipped' WHERE status IN ('未発送','購入済み')`},
		{"既存statusを英語コードへ移行(発送済み)", `UPDATE products SET status='shipped' WHERE status IN ('発送済み','発送済')`},
		{"旧 user テーブル削除", "DROP TABLE IF EXISTS `user`"},
	}

	for _, s := range statements {
		if _, err := db.Exec(s.sql); err != nil {
			// 1060 = Duplicate column name（再実行時のtags追加はスキップ扱い）
			if me, ok := err.(*mysql.MySQLError); ok && me.Number == 1060 {
				log.Printf("⏭️  %s: 既に適用済みのためスキップ", s.desc)
				continue
			}
			log.Fatalf("❌ %s: %v", s.desc, err)
		}
		log.Printf("✅ %s", s.desc)
	}

	// カテゴリマスターが空ならフロントの選択肢と揃えて投入
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM categories").Scan(&count); err != nil {
		log.Fatal(err)
	}
	if count == 0 {
		names := []string{"レディース", "メンズ", "ジャケット・アウター", "家電・スマホ", "カメラ", "本・ゲーム", "ビンテージ", "その他"}
		for _, n := range names {
			if _, err := db.Exec("INSERT INTO categories (name) VALUES (?)", n); err != nil {
				log.Fatal(err)
			}
		}
		log.Printf("✅ カテゴリマスターを%d件投入", len(names))
	} else {
		log.Printf("⏭️  カテゴリマスターは既に%d件あり", count)
	}

	log.Println("🎉 マイグレーション完了")
}
