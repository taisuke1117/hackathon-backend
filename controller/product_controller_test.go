package controller

import (
	"bytes"
	"encoding/json"
	"main/middleware"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newProductCtrl テスト用のProductControllerを組み立てる
func newProductCtrl(p *mockProductDao, rv *mockReviewDao) *ProductController {
	if p == nil {
		p = &mockProductDao{}
	}
	if rv == nil {
		rv = &mockReviewDao{}
	}
	return NewProductController(p, &mockNotificationDao{}, rv, &mockUserDao{})
}

// authedRequest DEV_AUTHモードでユーザーIDをセットしたリクエストを作る
func authedRequest(t *testing.T, method, target string, body any) *http.Request {
	t.Helper()
	t.Setenv("DEV_AUTH", "1")
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, target, &buf)
	req.Header.Set("X-Dev-User", "test-user-id")
	req.Header.Set("Content-Type", "application/json")
	return req
}

// serveAuthed Auth ミドルウェアを通してハンドラを実行する
func serveAuthed(handler http.HandlerFunc, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	middleware.Auth(handler).ServeHTTP(rec, req)
	return rec
}

// ─── Create ──────────────────────────────────────────────────────────────────

func TestCreate_MissingName_Returns400(t *testing.T) {
	ctrl := newProductCtrl(nil, nil)
	req := authedRequest(t, "POST", "/api/products", map[string]any{
		"name": "", "price": 1000,
	})
	rec := serveAuthed(ctrl.Create, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestCreate_ZeroPrice_Returns400(t *testing.T) {
	ctrl := newProductCtrl(nil, nil)
	req := authedRequest(t, "POST", "/api/products", map[string]any{
		"name": "テスト商品", "price": 0,
	})
	rec := serveAuthed(ctrl.Create, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestCreate_NegativePrice_Returns400(t *testing.T) {
	ctrl := newProductCtrl(nil, nil)
	req := authedRequest(t, "POST", "/api/products", map[string]any{
		"name": "テスト商品", "price": -100,
	})
	rec := serveAuthed(ctrl.Create, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestCreate_Valid_Returns201(t *testing.T) {
	ctrl := newProductCtrl(nil, nil)
	req := authedRequest(t, "POST", "/api/products", map[string]any{
		"name": "テスト商品", "price": 500,
	})
	rec := serveAuthed(ctrl.Create, req)
	if rec.Code != http.StatusCreated {
		t.Errorf("got %d, want 201", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if _, ok := body["product_id"]; !ok {
		t.Errorf("response has no product_id: %v", body)
	}
}

func TestCreate_InvalidJSON_Returns400(t *testing.T) {
	ctrl := newProductCtrl(nil, nil)
	t.Setenv("DEV_AUTH", "1")
	req := httptest.NewRequest("POST", "/api/products", bytes.NewBufferString("not-json"))
	req.Header.Set("X-Dev-User", "test-user-id")
	rec := serveAuthed(ctrl.Create, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

// ─── CreateReview ────────────────────────────────────────────────────────────

// reviewRequest muxを通してCreateReviewを呼ぶ（PathValueのために必要）
func reviewRequest(t *testing.T, ctrl *ProductController, productId string, body any) *httptest.ResponseRecorder {
	t.Helper()
	t.Setenv("DEV_AUTH", "1")
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(body)
	req := httptest.NewRequest("POST", "/api/products/"+productId+"/reviews", &buf)
	req.Header.Set("X-Dev-User", "test-user-id")
	req.Header.Set("Content-Type", "application/json")

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/products/{id}/reviews", ctrl.CreateReview)
	rec := httptest.NewRecorder()
	middleware.Auth(mux).ServeHTTP(rec, req)
	return rec
}

func TestCreateReview_RatingTooLow_Returns400(t *testing.T) {
	ctrl := newProductCtrl(nil, nil)
	rec := reviewRequest(t, ctrl, "1", map[string]any{"rating": 0})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestCreateReview_RatingTooHigh_Returns400(t *testing.T) {
	ctrl := newProductCtrl(nil, nil)
	rec := reviewRequest(t, ctrl, "1", map[string]any{"rating": 6})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestCreateReview_InvalidProductId_Returns400(t *testing.T) {
	ctrl := newProductCtrl(nil, nil)
	rec := reviewRequest(t, ctrl, "abc", map[string]any{"rating": 5})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestCreateReview_Valid_Returns201(t *testing.T) {
	ctrl := newProductCtrl(nil, nil)
	rec := reviewRequest(t, ctrl, "1", map[string]any{"rating": 4, "comment": "良品でした"})
	if rec.Code != http.StatusCreated {
		t.Errorf("got %d, want 201", rec.Code)
	}
}
