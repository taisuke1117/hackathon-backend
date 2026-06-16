package middleware

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// ─────────────────────────────────────────────────────────
// 認証ミドルウェア
//
// Firebase Authentication が発行する IDトークン（JWT）を検証して
// 「このリクエストを送ってきたのは誰か」を確定する。
//
// フロントエンドの流れ:
//   Firebase ログイン
//   → Firebase SDK が IDトークン（JWT文字列）を発行
//   → apiFetch が Authorization: Bearer <IDトークン> をヘッダーに付ける
//   → このミドルウェアがトークンを検証してユーザーIDをContextに保存
//   → 各コントローラーが UserId(r) でそのIDを取り出す
// ─────────────────────────────────────────────────────────

// contextKey: Context にユーザーIDを保存するためのキーの型
// string をそのままキーにすると他のパッケージと衝突する可能性があるため専用型を使う
type contextKey string

const userIdKey contextKey = "userId"

// UserId: リクエストのContextからユーザーIDを取り出すヘルパー
// コントローラーから middleware.UserId(r) の形で呼ぶ
func UserId(r *http.Request) string {
	if v, ok := r.Context().Value(userIdKey).(string); ok {
		return v
	}
	return ""
}

// ── Google公開鍵キャッシュ ───────────────────────────────────
// Firebase IDトークンの署名を検証するには、Googleが公開している公開鍵が必要
// 毎回Googleにアクセスすると遅いので、1時間キャッシュする
var (
	certMu     sync.RWMutex          // 複数ゴルーチンから同時アクセスされるのでRWMutexで保護
	certKeys   map[string]*rsa.PublicKey // キーID → RSA公開鍵 のマップ
	certExpiry time.Time             // キャッシュの有効期限
)

// GoogleのFirebase公開鍵エンドポイント
const certsURL = "https://www.googleapis.com/robot/v1/metadata/x509/securetoken@system.gserviceaccount.com"

// firebaseProjectId: Firebase プロジェクトIDを返す
// 環境変数 FIREBASE_PROJECT_ID が設定されていればそちらを使う（本番デプロイ用）
// なければハードコードした開発用のプロジェクトIDを使う
func firebaseProjectId() string {
	if v := os.Getenv("FIREBASE_PROJECT_ID"); v != "" {
		return v
	}
	return "hackathon-ebd20"
}

// getCertKeys: Google の公開鍵を取得（キャッシュがあればそれを返す）
func getCertKeys() (map[string]*rsa.PublicKey, error) {
	// 読み取りロック：キャッシュが有効かチェック
	certMu.RLock()
	if certKeys != nil && time.Now().Before(certExpiry) {
		defer certMu.RUnlock()
		return certKeys, nil // キャッシュヒット
	}
	certMu.RUnlock()

	// キャッシュ期限切れまたは未取得 → Googleから公開鍵を取得
	resp, err := http.Get(certsURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// レスポンスは { "キーID": "PEM証明書文字列", ... } の形式
	var raw map[string]string
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	// PEM証明書文字列 → RSA公開鍵オブジェクト に変換してマップに入れる
	keys := make(map[string]*rsa.PublicKey, len(raw))
	for kid, certPEM := range raw {
		block, _ := pem.Decode([]byte(certPEM))
		if block == nil {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			continue
		}
		if pub, ok := cert.PublicKey.(*rsa.PublicKey); ok {
			keys[kid] = pub
		}
	}
	if len(keys) == 0 {
		return nil, errors.New("no valid certs from google")
	}

	// 書き込みロック：キャッシュを更新（1時間有効）
	certMu.Lock()
	certKeys = keys
	certExpiry = time.Now().Add(1 * time.Hour)
	certMu.Unlock()

	return keys, nil
}

// b64Decode: Base64URL（パディングなし）デコード
// JWTの各パートはBase64URLエンコードされている
func b64Decode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

// verifyIDToken: Firebase IDトークン（JWT）を検証してユーザーID（uid）を返す
//
// JWTの構造: "ヘッダー.ペイロード.署名"（ドット区切りのBase64URL）
//
// 検証ステップ:
//  1. JWTを3つに分割してヘッダー・ペイロードをデコード
//  2. アルゴリズムが RS256 かチェック
//  3. 発行先（aud）・発行者（iss）・有効期限（exp）を検証
//  4. Googleの公開鍵でRSA署名を検証（改ざん防止）
//  5. 全部OKなら sub（Firebase UID）を返す
func verifyIDToken(token string) (string, error) {
	// JWTはドット区切りで3パーツ: header.payload.signature
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", errors.New("invalid token format")
	}

	// ── ヘッダーのデコード ──────────────────────────────────────
	headerJSON, err := b64Decode(parts[0])
	if err != nil {
		return "", err
	}
	var header struct {
		Alg string `json:"alg"` // 署名アルゴリズム（RS256であるべき）
		Kid string `json:"kid"` // どの公開鍵で署名したかを示すキーID
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return "", err
	}
	if header.Alg != "RS256" {
		return "", errors.New("unexpected alg")
	}

	// ── ペイロード（クレーム）のデコード ────────────────────────
	payloadJSON, err := b64Decode(parts[1])
	if err != nil {
		return "", err
	}
	var claims struct {
		Aud string `json:"aud"` // 対象サービス（Firebaseプロジェクト名と一致するはず）
		Iss string `json:"iss"` // 発行者（GoogleのFirebase）
		Sub string `json:"sub"` // ユーザーID（Firebase UID）。これを返す
		Exp int64  `json:"exp"` // 有効期限（Unixタイムスタンプ）
	}
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return "", err
	}

	// ── クレームの検証 ──────────────────────────────────────────
	project := firebaseProjectId()
	if claims.Aud != project {
		return "", errors.New("invalid audience") // 別のFirebaseプロジェクト向けトークンを弾く
	}
	if claims.Iss != "https://securetoken.google.com/"+project {
		return "", errors.New("invalid issuer")
	}
	if claims.Sub == "" {
		return "", errors.New("empty subject")
	}
	if time.Now().Unix() > claims.Exp {
		return "", errors.New("token expired") // 期限切れ（フロントは自動更新するので通常は起きない）
	}

	// ── RSA署名の検証 ───────────────────────────────────────────
	sig, err := b64Decode(parts[2])
	if err != nil {
		return "", err
	}
	keys, err := getCertKeys()
	if err != nil {
		return "", err
	}
	pub, ok := keys[header.Kid]
	if !ok {
		// キーIDが見つからない場合はGoogleが鍵をローテーションした直後の可能性がある
		// キャッシュを破棄して1回だけ再取得を試みる
		certMu.Lock()
		certKeys = nil
		certMu.Unlock()
		if keys, err = getCertKeys(); err != nil {
			return "", err
		}
		if pub, ok = keys[header.Kid]; !ok {
			return "", errors.New("unknown key id")
		}
	}

	// "ヘッダー.ペイロード" の部分をSHA256でハッシュ化して署名と照合
	hashed := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, hashed[:], sig); err != nil {
		return "", fmt.Errorf("signature verification failed: %w", err) // 改ざんを検知
	}

	return claims.Sub, nil // 検証成功 → Firebase UID を返す
}

// Auth: APIリクエスト全体の前段に差し込む認証ミドルウェア
//
// 全ての /api/ へのリクエストはここを通る（router.goで設定）
//
// ローカル開発モード（DEV_AUTH=1）:
//   X-Dev-User ヘッダーに任意のユーザーIDを入れるとトークン検証をスキップできる
//   本番環境では DEV_AUTH=1 を絶対に設定しない
func Auth(next http.Handler) http.Handler {
	devMode := os.Getenv("DEV_AUTH") == "1"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		if devMode {
			// 開発モード: X-Dev-User ヘッダーがあればそのUIDを信用する
			if uid := r.Header.Get("X-Dev-User"); uid != "" {
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIdKey, uid)))
				return
			}
		}

		// Authorization: Bearer <IDトークン> ヘッダーを確認
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, `{"error":"missing bearer token"}`, http.StatusUnauthorized)
			return
		}

		// "Bearer " の部分を除いたトークン文字列を検証
		uid, err := verifyIDToken(strings.TrimPrefix(authHeader, "Bearer "))
		if err != nil {
			http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
			return
		}

		// 検証成功: ContextにユーザーIDをセットして次のハンドラへ
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIdKey, uid)))
	})
}

// CORS: クロスオリジンリクエストを許可するミドルウェア
//
// フロント（Vercel or localhost:3000）とバックエンド（Cloud Run）は別ドメインなので
// ブラウザのセキュリティポリシー（Same-Origin Policy）によりリクエストが弾かれる。
// このミドルウェアが適切なヘッダーを返すことで許可する。
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			// リクエスト元のオリジンをそのまま許可（特定ドメインに絞る場合は要変更）
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Dev-User")
			w.Header().Set("Access-Control-Max-Age", "3600") // プリフライトキャッシュ1時間
		}

		// プリフライトリクエスト（OPTIONSメソッド）: 本番リクエスト前にブラウザが許可確認する
		// ヘッダーだけ返して終了（ボディ不要）
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
