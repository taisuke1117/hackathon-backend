FROM golang:1.26 AS build
WORKDIR /app

# ---- 🔴 ここから書き換え 🔴 ----
# 1. 依存関係ファイルを先にコピーしてキャッシュを利用
COPY go.mod go.sum ./
RUN go mod download

# 2. プロジェクトの全ファイル（controller, dao, usecaseなど）をコピー
COPY . .

# 3. ビルドを実行（CGO無効、スタンドアロンバイナリ）
RUN CGO_ENABLED=0 GOOS=linux go build -o main main.go
# ---- 🍏 ここまで書き換え 🍏 ----

# 実行用ステージ（マルチステージビルドでコンテナサイズを極限まで小さくします）
FROM alpine:latest
WORKDIR /root/
COPY --from=build /app/main .

# 必要なルート証明書（HTTPS通信をする場合に必要なお守り）を追加
RUN apk --no-cache add ca-certificates

# Cloud Runはデフォルトで8080ポートを期待することが多いため環境変数を設定
ENV PORT=8080
EXPOSE 8080
CMD ["./main"]