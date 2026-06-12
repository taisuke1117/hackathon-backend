package main

import (
	"database/sql"
	"log"
	"main/controller"
	"main/dao"
	"main/db"
	"main/router"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-sql-driver/mysql"
)

func main() {
	database := db.Connect()
	defer closeDBWithSysCall(database)

	runMigrations(database)

	userDao := dao.NewUserDao(database)
	productDao := dao.NewProductDao(database)
	chatDao := dao.NewChatDao(database)
	notificationDao := dao.NewNotificationDao(database)
	reviewDao := dao.NewReviewDao(database)

	userCtrl := controller.NewUserController(userDao, productDao, reviewDao)
	productCtrl := controller.NewProductController(productDao, notificationDao, reviewDao, userDao)
	chatCtrl := controller.NewChatController(chatDao, notificationDao)
	notificationCtrl := controller.NewNotificationController(notificationDao)
	geminiCtrl := controller.NewGeminiController(userDao)

	handler := router.Setup(userCtrl, productCtrl, chatCtrl, notificationCtrl, geminiCtrl)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Listening on :%s...\n", port)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatal(err)
	}
}

func runMigrations(database *sql.DB) {
	statements := []struct {
		desc string
		sql  string
	}{
		{"product_images テーブル作成", `CREATE TABLE IF NOT EXISTS product_images (
			image_id   BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
			product_id BIGINT UNSIGNED NOT NULL,
			position   INT NOT NULL DEFAULT 0,
			url        TEXT NOT NULL,
			INDEX idx_product_images_product (product_id)
		)`},
		{"product_categories テーブル作成", `CREATE TABLE IF NOT EXISTS product_categories (
			product_id  BIGINT UNSIGNED NOT NULL,
			category_id BIGINT UNSIGNED NOT NULL,
			PRIMARY KEY (product_id, category_id)
		)`},
		{"reviews テーブル作成", `CREATE TABLE IF NOT EXISTS reviews (
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
		{"既存statusを英語化(出品中)", `UPDATE products SET status='available' WHERE status='出品中'`},
		{"既存statusを英語化(未発送)", `UPDATE products SET status='unshipped' WHERE status IN ('未発送','購入済み')`},
		{"既存statusを英語化(発送済み)", `UPDATE products SET status='shipped' WHERE status IN ('発送済み','発送済')`},
		{"旧 user テーブル削除", "DROP TABLE IF EXISTS `user`"},
	}

	for _, s := range statements {
		if _, err := database.Exec(s.sql); err != nil {
			if me, ok := err.(*mysql.MySQLError); ok && me.Number == 1060 {
				log.Printf("migrate: skip (already applied) — %s", s.desc)
				continue
			}
			log.Fatalf("migrate: FAILED — %s: %v", s.desc, err)
		}
		log.Printf("migrate: ok — %s", s.desc)
	}

	categories := []string{
		"レディース", "メンズ", "キッズ・ベビー", "アウター・ジャケット", "バッグ・財布",
		"シューズ", "アクセサリー・ジュエリー", "家電・スマホ", "PC・タブレット", "カメラ・光学機器",
		"本・雑誌・漫画", "ゲーム・ホビー", "おもちゃ・グッズ", "スポーツ・アウトドア", "インテリア・家具",
		"コスメ・美容・香水", "食品・飲料", "楽器・音響機器", "ビンテージ・コレクション", "その他",
	}
	added := 0
	for _, name := range categories {
		res, err := database.Exec(
			"INSERT INTO categories (name) SELECT ? FROM dual WHERE NOT EXISTS (SELECT 1 FROM categories WHERE name = ?)",
			name, name)
		if err != nil {
			log.Printf("migrate: category skip (%s): %v", name, err)
			continue
		}
		n, _ := res.RowsAffected()
		added += int(n)
	}
	if added > 0 {
		log.Printf("migrate: カテゴリ %d 件追加", added)
	}
	log.Println("migrate: 完了")
}

func closeDBWithSysCall(database *sql.DB) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		s := <-sig
		log.Printf("received syscall: %v", s)
		if err := database.Close(); err != nil {
			log.Fatal(err)
		}
		log.Println("db closed")
		os.Exit(0)
	}()
}
