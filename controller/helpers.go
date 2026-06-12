package controller

import (
	"encoding/json"
	"errors"
	"log"
	"main/dao"
	"net/http"
	"strconv"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// handleDaoError DAO層のエラーをHTTPステータスへマッピングして返す
func handleDaoError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, dao.ErrNotFound):
		writeError(w, http.StatusNotFound, "対象が見つかりません")
	case errors.Is(err, dao.ErrForbidden):
		writeError(w, http.StatusForbidden, "この操作を行う権限がありません")
	case errors.Is(err, dao.ErrConflict):
		writeError(w, http.StatusConflict, "現在の状態ではこの操作はできません")
	case errors.Is(err, dao.ErrOwnProduct):
		writeError(w, http.StatusBadRequest, "自分の商品は購入できません")
	case errors.Is(err, dao.ErrNotPurchased):
		writeError(w, http.StatusBadRequest, "購入した商品のみ評価できます")
	default:
		log.Printf("internal error: %v", err)
		writeError(w, http.StatusInternalServerError, "サーバーエラーが発生しました")
	}
}

// pathId URLパスの {名前} 部分をint64として取り出す
func pathId(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	return id, err == nil && id > 0
}

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "リクエストボディが不正です")
		return false
	}
	return true
}
