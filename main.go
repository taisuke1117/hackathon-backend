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

// main: DB接続 → マイグレーション → DAO生成 → Controller生成（DI） → HTTP起動

func main() {
	// ── DBに接続
	database := db.Connect()
	defer closeDBWithSysCall(database)

	// ── マイグレーション
	runMigrations(database)

	// ── DAOの生成
	userDao := dao.NewUserDao(database)
	productDao := dao.NewProductDao(database)
	chatDao := dao.NewChatDao(database)
	notificationDao := dao.NewNotificationDao(database)
	reviewDao := dao.NewReviewDao(database)
	liveDao := dao.NewLiveDao(database)

	// ── Controllerの生成（DAOを注入）
	userCtrl := controller.NewUserController(userDao, productDao, reviewDao, notificationDao)
	productCtrl := controller.NewProductController(productDao, notificationDao, reviewDao, userDao)
	chatCtrl := controller.NewChatController(chatDao, notificationDao)
	notificationCtrl := controller.NewNotificationController(notificationDao)
	geminiCtrl := controller.NewGeminiController(userDao)
	liveCtrl := controller.NewLiveController(liveDao)

	// ── ルーターのセットアップ
	handler := router.Setup(userCtrl, productCtrl, chatCtrl, notificationCtrl, geminiCtrl, liveCtrl)

	// ── HTTPサーバー起動 ─────────────────────────────────────
	// Cloud Run は PORT 環境変数でポートを指定してくる
	// ローカルでは PORT が未設定なので 8080 をデフォルトにする
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Listening on :%s...\n", port)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatal(err)
	}
}

// runMigrations: 冪等なDDL群をサーバー起動時に実行する（本番マイグレーションツールの代替）
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
		{"likes テーブル作成", `CREATE TABLE IF NOT EXISTS likes (
			user_id    VARCHAR(255) NOT NULL,
			product_id BIGINT UNSIGNED NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (user_id, product_id)
		)`},
		{"chatrooms テーブル作成", `CREATE TABLE IF NOT EXISTS chatrooms (
			chatroom_id       BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
			product_id        BIGINT UNSIGNED NOT NULL,
			proposer_id       VARCHAR(255) NOT NULL,
			discount_proposed INT NOT NULL DEFAULT 0,
			discount_approved INT NOT NULL DEFAULT 0,
			created_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_chatrooms_product (product_id),
			INDEX idx_chatrooms_proposer (proposer_id)
		)`},
		{"chats テーブル作成", `CREATE TABLE IF NOT EXISTS chats (
			chat_id     BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
			chatroom_id BIGINT UNSIGNED NOT NULL,
			sender_id   VARCHAR(255) NOT NULL,
			content     TEXT NOT NULL,
			is_read     TINYINT(1) NOT NULL DEFAULT 0,
			created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_chats_room (chatroom_id)
		)`},
		{"notifications テーブル作成", `CREATE TABLE IF NOT EXISTS notifications (
			notification_id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
			user_id         VARCHAR(255) NOT NULL,
			` + "`type`" + ` VARCHAR(50) NOT NULL,
			title           VARCHAR(255) NOT NULL,
			content         TEXT NOT NULL,
			link_url        TEXT,
			is_read         TINYINT(1) NOT NULL DEFAULT 0,
			created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_notifications_user (user_id)
		)`},
		{"blocks テーブル作成", `CREATE TABLE IF NOT EXISTS blocks (
			blocker_id VARCHAR(255) NOT NULL,
			blocked_id VARCHAR(255) NOT NULL,
			PRIMARY KEY (blocker_id, blocked_id)
		)`},
		// 以下は既存テーブルへのカラム追加・データ修正
		// ── usersテーブルへのカラム追加 ──
		{"users.icon_url カラム追加", `ALTER TABLE users ADD COLUMN icon_url TEXT`},
		{"users.place カラム追加", `ALTER TABLE users ADD COLUMN place VARCHAR(255)`},
		{"users.bio カラム追加", `ALTER TABLE users ADD COLUMN bio TEXT`},
		{"users.shipping_address カラム追加", `ALTER TABLE users ADD COLUMN shipping_address TEXT`},
		{"users.total_sales カラム追加", `ALTER TABLE users ADD COLUMN total_sales INT NOT NULL DEFAULT 0`},
		{"users.mail カラム追加", `ALTER TABLE users ADD COLUMN mail VARCHAR(255)`},
		// ── productsテーブルへのカラム追加 ──
		{"products.tags カラム追加", `ALTER TABLE products ADD COLUMN tags TEXT`},
		{"products.condition カラム追加", "ALTER TABLE products ADD COLUMN `condition` VARCHAR(50)"},
		{"products.image_url カラム追加", `ALTER TABLE products ADD COLUMN image_url TEXT`},
		{"products.buyer_id カラム追加", `ALTER TABLE products ADD COLUMN buyer_id VARCHAR(255) DEFAULT NULL`},
		{"products.likes_count カラム追加", `ALTER TABLE products ADD COLUMN likes_count INT NOT NULL DEFAULT 0`},
		{"products.views_count カラム追加", `ALTER TABLE products ADD COLUMN views_count INT NOT NULL DEFAULT 0`},
		{"products.category_id カラム追加", `ALTER TABLE products ADD COLUMN category_id BIGINT UNSIGNED DEFAULT NULL`},
		{"products.updated_at カラム追加", `ALTER TABLE products ADD COLUMN updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP`},
		{"products.status デフォルトを英語化", `ALTER TABLE products ALTER COLUMN status SET DEFAULT 'available'`},
		// ── product_imagesテーブルへのカラム追加 ──
		{"product_images.position カラム追加", `ALTER TABLE product_images ADD COLUMN position INT NOT NULL DEFAULT 0`},
		// ── 日本語だったステータス値を英語に統一（既存データの移行） ──
		{"既存statusを英語化(出品中)", `UPDATE products SET status='available' WHERE status='出品中'`},
		{"既存statusを英語化(未発送)", `UPDATE products SET status='unshipped' WHERE status IN ('未発送','購入済み')`},
		{"既存statusを英語化(発送済み)", `UPDATE products SET status='shipped' WHERE status IN ('発送済み','発送済')`},
		{"旧 user テーブル削除", "DROP TABLE IF EXISTS `user`"},

		// ── ライブ配信テーブル ──
		{"live_rooms テーブル作成", `CREATE TABLE IF NOT EXISTS live_rooms (
			room_id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
			seller_id         VARCHAR(255) NOT NULL,
			title             VARCHAR(255) NOT NULL,
			status            VARCHAR(20) NOT NULL DEFAULT 'scheduled',
			livekit_room_name VARCHAR(255),
			viewer_count      INT NOT NULL DEFAULT 0,
			created_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_live_rooms_seller (seller_id),
			INDEX idx_live_rooms_status (status)
		)`},
		{"live_products テーブル作成", `CREATE TABLE IF NOT EXISTS live_products (
			id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
			room_id         BIGINT UNSIGNED NOT NULL,
			product_id      BIGINT UNSIGNED NOT NULL,
			position        INT NOT NULL DEFAULT 0,
			mode            VARCHAR(20) NOT NULL DEFAULT 'instant',
			start_price     INT NOT NULL DEFAULT 0,
			instant_price   INT,
			current_price   INT NOT NULL DEFAULT 0,
			current_bidder  VARCHAR(255),
			auction_end_at  TIMESTAMP NULL DEFAULT NULL,
			status          VARCHAR(20) NOT NULL DEFAULT 'waiting',
			buyer_id        VARCHAR(255),
			INDEX idx_live_products_room (room_id),
			INDEX idx_live_products_status (room_id, status)
		)`},
		{"live_comments テーブル作成", `CREATE TABLE IF NOT EXISTS live_comments (
			id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
			room_id    BIGINT UNSIGNED NOT NULL,
			user_id    VARCHAR(255) NOT NULL,
			content    TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_live_comments_room (room_id)
		)`},
	}

	for _, s := range statements {
		if _, err := database.Exec(s.sql); err != nil {
			if me, ok := err.(*mysql.MySQLError); ok {
				switch me.Number {
				case 1060: // Duplicate column name — ADD COLUMN済み
					log.Printf("migrate: skip (already applied) — %s", s.desc)
					continue
				case 1061: // Duplicate key name — INDEX済み
					log.Printf("migrate: skip (index exists) — %s", s.desc)
					continue
				case 3730, 1217: // FK制約でDROP TABLE不可 — 既に削除済みか制約あり
					log.Printf("migrate: skip (fk constraint) — %s", s.desc)
					continue
				}
			}
			// それ以外は警告ログだけ残してサーバーは起動させる
			log.Printf("migrate: WARNING (skipped) — %s: %v", s.desc, err)
		} else {
			log.Printf("migrate: ok — %s", s.desc)
		}
	}

	// ── カテゴリマスターの初期データ投入 ─────────────────────
	// 起動のたびに実行されるが「存在しないものだけINSERT」なので冪等
	categories := []string{
		"レディース", "メンズ", "キッズ・ベビー", "アウター・ジャケット", "バッグ・財布",
		"シューズ", "アクセサリー・ジュエリー", "家電・スマホ", "PC・タブレット", "カメラ・光学機器",
		"本・雑誌・漫画", "ゲーム・ホビー", "おもちゃ・グッズ", "スポーツ・アウトドア", "インテリア・家具",
		"コスメ・美容・香水", "食品・飲料", "楽器・音響機器", "ビンテージ・コレクション", "その他",
	}
	added := 0
	for _, name := range categories {
		// サブクエリで「まだ存在しない場合だけINSERT」を実現（INSERT IF NOT EXISTS の代替）
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

// closeDBWithSysCall: SIGTERMまたはSIGINT（Ctrl+C）を受け取ったときにDBを閉じて終了する
//
// Cloud RunはデプロイのときにSIGTERMを送ってコンテナを停止する。
// このゴルーチンがそれを受け取ってDBコネクションを正常にクローズする（グレースフルシャットダウン）。
func closeDBWithSysCall(database *sql.DB) {
	sig := make(chan os.Signal, 1)
	// SIGTERMとSIGINTを監視するよう登録
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		s := <-sig // シグナルが来るまでここでブロック
		log.Printf("received syscall: %v", s)
		if err := database.Close(); err != nil {
			log.Fatal(err)
		}
		log.Println("db closed")
		os.Exit(0)
	}()
}
