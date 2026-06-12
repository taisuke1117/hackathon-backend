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

type contextKey string

const userIdKey contextKey = "userId"

// UserId リクエストコンテキストから認証済みユーザーのUIDを取り出す
func UserId(r *http.Request) string {
	if v, ok := r.Context().Value(userIdKey).(string); ok {
		return v
	}
	return ""
}

// FirebaseのIDトークン署名検証用のGoogle公開鍵キャッシュ
var (
	certMu     sync.RWMutex
	certKeys   map[string]*rsa.PublicKey
	certExpiry time.Time
)

const certsURL = "https://www.googleapis.com/robot/v1/metadata/x509/securetoken@system.gserviceaccount.com"

func firebaseProjectId() string {
	if v := os.Getenv("FIREBASE_PROJECT_ID"); v != "" {
		return v
	}
	return "hackathon-ebd20"
}

func getCertKeys() (map[string]*rsa.PublicKey, error) {
	certMu.RLock()
	if certKeys != nil && time.Now().Before(certExpiry) {
		defer certMu.RUnlock()
		return certKeys, nil
	}
	certMu.RUnlock()

	resp, err := http.Get(certsURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var raw map[string]string
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
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

	certMu.Lock()
	certKeys = keys
	certExpiry = time.Now().Add(1 * time.Hour)
	certMu.Unlock()
	return keys, nil
}

func b64Decode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

// verifyIDToken FirebaseのIDトークンを検証してUID(sub)を返す
func verifyIDToken(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", errors.New("invalid token format")
	}

	headerJSON, err := b64Decode(parts[0])
	if err != nil {
		return "", err
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return "", err
	}
	if header.Alg != "RS256" {
		return "", errors.New("unexpected alg")
	}

	payloadJSON, err := b64Decode(parts[1])
	if err != nil {
		return "", err
	}
	var claims struct {
		Aud string `json:"aud"`
		Iss string `json:"iss"`
		Sub string `json:"sub"`
		Exp int64  `json:"exp"`
	}
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return "", err
	}
	project := firebaseProjectId()
	if claims.Aud != project {
		return "", errors.New("invalid audience")
	}
	if claims.Iss != "https://securetoken.google.com/"+project {
		return "", errors.New("invalid issuer")
	}
	if claims.Sub == "" {
		return "", errors.New("empty subject")
	}
	if time.Now().Unix() > claims.Exp {
		return "", errors.New("token expired")
	}

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
		// 鍵がローテーションされた直後はキャッシュを破棄して1回だけ再取得
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

	hashed := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, hashed[:], sig); err != nil {
		return "", fmt.Errorf("signature verification failed: %w", err)
	}
	return claims.Sub, nil
}

// Auth 全APIの前段でFirebase IDトークンを検証するミドルウェア
// ローカル開発用: DEV_AUTH=1 のとき X-Dev-User ヘッダーのUIDをそのまま信用する
func Auth(next http.Handler) http.Handler {
	devMode := os.Getenv("DEV_AUTH") == "1"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if devMode {
			if uid := r.Header.Get("X-Dev-User"); uid != "" {
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIdKey, uid)))
				return
			}
		}

		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, `{"error":"missing bearer token"}`, http.StatusUnauthorized)
			return
		}
		uid, err := verifyIDToken(strings.TrimPrefix(authHeader, "Bearer "))
		if err != nil {
			http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIdKey, uid)))
	})
}

// CORS Vercel上のフロントとlocalhostからのクロスオリジンリクエストを許可する
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Dev-User")
			w.Header().Set("Access-Control-Max-Age", "3600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
