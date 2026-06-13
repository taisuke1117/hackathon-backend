package controller

import (
	"encoding/json"
	"main/dao"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleDaoError(t *testing.T) {
	cases := []struct {
		err        error
		wantStatus int
	}{
		{dao.ErrNotFound, http.StatusNotFound},
		{dao.ErrForbidden, http.StatusForbidden},
		{dao.ErrConflict, http.StatusConflict},
		{dao.ErrOwnProduct, http.StatusBadRequest},
		{dao.ErrNotPurchased, http.StatusBadRequest},
		// 未知のエラーは500
		{http.ErrNoCookie, http.StatusInternalServerError},
	}

	for _, tc := range cases {
		rec := httptest.NewRecorder()
		handleDaoError(rec, tc.err)
		if rec.Code != tc.wantStatus {
			t.Errorf("err=%v: got status %d, want %d", tc.err, rec.Code, tc.wantStatus)
		}
		// レスポンスがJSONであること
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Errorf("err=%v: response is not valid JSON: %s", tc.err, rec.Body.String())
		}
		if _, ok := body["error"]; !ok {
			t.Errorf("err=%v: response JSON has no 'error' key: %v", tc.err, body)
		}
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, map[string]string{"key": "value"})

	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Errorf("response is not valid JSON: %s", rec.Body.String())
	}
	if body["key"] != "value" {
		t.Errorf("body[key] = %q, want value", body["key"])
	}
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, "テストエラー")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", rec.Code)
	}
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["error"] != "テストエラー" {
		t.Errorf("error message = %q, want テストエラー", body["error"])
	}
}
