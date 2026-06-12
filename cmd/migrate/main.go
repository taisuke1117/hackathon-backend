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

	// カテゴリマスターを最新リストに同期（存在しない名前だけ追加）
	categories := []string{
		"レディース",
		"メンズ",
		"キッズ・ベビー",
		"アウター・ジャケット",
		"バッグ・財布",
		"シューズ",
		"アクセサリー・ジュエリー",
		"家電・スマホ",
		"PC・タブレット",
		"カメラ・光学機器",
		"本・雑誌・漫画",
		"ゲーム・ホビー",
		"おもちゃ・グッズ",
		"スポーツ・アウトドア",
		"インテリア・家具",
		"コスメ・美容・香水",
		"食品・飲料",
		"楽器・音響機器",
		"ビンテージ・コレクション",
		"その他",
	}
	added := 0
	for _, name := range categories {
		res, err := db.Exec(
			"INSERT INTO categories (name) SELECT ? FROM dual WHERE NOT EXISTS (SELECT 1 FROM categories WHERE name = ?)",
			name, name)
		if err != nil {
			log.Printf("⚠️  カテゴリ追加スキップ(%s): %v", name, err)
			continue
		}
		n, _ := res.RowsAffected()
		added += int(n)
	}
	if added > 0 {
		log.Printf("✅ カテゴリを%d件追加しました", added)
	} else {
		log.Printf("⏭️  カテゴリはすべて最新です")
	}

	log.Println("🎉 マイグレーション完了")
}
