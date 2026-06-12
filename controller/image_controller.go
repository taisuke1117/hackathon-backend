package controller

import (
	"fmt"
	"io"
	"log"
	"main/gcs"
	"main/middleware"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var unsafeChars = regexp.MustCompile(`[^\w.-]`)

// UploadImage POST /api/images — multipart/form-data の "file" フィールドで画像を受け取り、
// GCSへ保存して配信URLを返す
func UploadImage(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 最大10MB
		writeError(w, http.StatusBadRequest, "画像の読み取りに失敗しました（10MB以下にしてください）")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "fileフィールドに画像を添付してください")
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		writeError(w, http.StatusBadRequest, "画像ファイルのみアップロードできます")
		return
	}

	safeName := unsafeChars.ReplaceAllString(header.Filename, "_")
	objectName := fmt.Sprintf("%s/%d_%s", middleware.UserId(r), time.Now().UnixMilli(), safeName)

	if err := gcs.Upload(r.Context(), objectName, contentType, file); err != nil {
		log.Printf("GCS upload failed: %v", err)
		writeError(w, http.StatusBadGateway, "画像の保存に失敗しました: "+err.Error())
		return
	}

	// 配信はこのバックエンドの /images/ エンドポイント経由（認証不要GET）
	scheme := "https"
	if strings.HasPrefix(r.Host, "localhost") || strings.HasPrefix(r.Host, "127.") {
		scheme = "http"
	}
	url := fmt.Sprintf("%s://%s/images/%s", scheme, r.Host, objectName)
	writeJSON(w, http.StatusCreated, map[string]string{"url": url})
}

// ServeImage GET /images/{object...} — GCSのオブジェクトをそのまま配信（<img>から参照するため認証不要）
func ServeImage(w http.ResponseWriter, r *http.Request) {
	objectName := r.PathValue("object")
	if objectName == "" || strings.Contains(objectName, "..") {
		http.NotFound(w, r)
		return
	}
	rd, contentType, err := gcs.Open(r.Context(), objectName)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer rd.Close()

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = io.Copy(w, rd)
}
