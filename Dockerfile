FROM golang:1.21 AS build
WORKDIR /app
COPY main.go .
# CGOを無効にして軽量なスタンドアロンバイナリをビルド
RUN CGO_ENABLED=0 GOOS=linux go build -o main main.go

# 実行用ステージ（マルチステージビルドでコンテナサイズを極限まで小さくします）
FROM alpine:latest
WORKDIR /root/
COPY --from=build /app/main .
# Cloud Runはデフォルトで8080ポートを期待することが多いため環境変数を設定
ENV PORT=8080
EXPOSE 8080
CMD ["./main"]