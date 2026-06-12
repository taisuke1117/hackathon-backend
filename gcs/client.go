package gcs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"cloud.google.com/go/storage"
)

var (
	mu            sync.Mutex
	client        *storage.Client
	bucketChecked bool
)

// BucketName 画像保存先バケット。環境変数GCS_BUCKETで上書き可能
func BucketName() string {
	if v := os.Getenv("GCS_BUCKET"); v != "" {
		return v
	}
	return "term9-taisuke-ohmori-hackathon-images"
}

func projectID() string {
	if v := os.Getenv("GOOGLE_CLOUD_PROJECT"); v != "" {
		return v
	}
	return "term9-taisuke-ohmori"
}

func getClient(ctx context.Context) (*storage.Client, error) {
	mu.Lock()
	defer mu.Unlock()
	if client != nil {
		return client, nil
	}
	c, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("GCSクライアントの初期化に失敗（Cloud Run上では自動認証されます）: %w", err)
	}
	client = c
	return client, nil
}

// ensureBucket バケットが無ければ作成する（プロセスごとに初回のみ確認）
func ensureBucket(ctx context.Context, c *storage.Client) error {
	mu.Lock()
	checked := bucketChecked
	mu.Unlock()
	if checked {
		return nil
	}

	b := c.Bucket(BucketName())
	_, err := b.Attrs(ctx)
	if errors.Is(err, storage.ErrBucketNotExist) {
		if err := b.Create(ctx, projectID(), &storage.BucketAttrs{Location: "US-CENTRAL1"}); err != nil {
			return fmt.Errorf("バケット %s の自動作成に失敗。GCPコンソールで手動作成してください: %w", BucketName(), err)
		}
	} else if err != nil {
		return err
	}

	mu.Lock()
	bucketChecked = true
	mu.Unlock()
	return nil
}

// Upload オブジェクトを書き込む
func Upload(ctx context.Context, objectName, contentType string, r io.Reader) error {
	c, err := getClient(ctx)
	if err != nil {
		return err
	}
	if err := ensureBucket(ctx, c); err != nil {
		return err
	}

	wctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	w := c.Bucket(BucketName()).Object(objectName).NewWriter(wctx)
	w.ContentType = contentType
	if _, err := io.Copy(w, r); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}

// Open オブジェクトを読み出す（配信エンドポイント用）。戻り値: リーダー, Content-Type
func Open(ctx context.Context, objectName string) (io.ReadCloser, string, error) {
	c, err := getClient(ctx)
	if err != nil {
		return nil, "", err
	}
	rd, err := c.Bucket(BucketName()).Object(objectName).NewReader(ctx)
	if err != nil {
		return nil, "", err
	}
	return rd, rd.Attrs.ContentType, nil
}
