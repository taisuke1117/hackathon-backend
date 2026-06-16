package controller

import (
	"bytes"
	"encoding/json"
	"main/dao"
	"main/middleware"
	"main/model"
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

// ─── pathIdを使うハンドラ共通ヘルパー ────────────────────────────────────────

// muxRequest PathValue が必要なハンドラをテストするためのヘルパー
func muxRequest(t *testing.T, method, pattern, url string, handler http.HandlerFunc, body any) *httptest.ResponseRecorder {
	t.Helper()
	t.Setenv("DEV_AUTH", "1")
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, url, &buf)
	req.Header.Set("X-Dev-User", "test-user-id")
	req.Header.Set("Content-Type", "application/json")

	mux := http.NewServeMux()
	mux.HandleFunc(pattern, handler)
	rec := httptest.NewRecorder()
	middleware.Auth(mux).ServeHTTP(rec, req)
	return rec
}

// ─── Update ──────────────────────────────────────────────────────────────────

func TestUpdate_InvalidId_Returns400(t *testing.T) {
	ctrl := newProductCtrl(nil, nil)
	rec := muxRequest(t, "PUT", "PUT /api/products/{id}", "/api/products/abc",
		ctrl.Update, map[string]any{"name": "商品", "price": 500})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestUpdate_MissingName_Returns400(t *testing.T) {
	ctrl := newProductCtrl(nil, nil)
	rec := muxRequest(t, "PUT", "PUT /api/products/{id}", "/api/products/1",
		ctrl.Update, map[string]any{"name": "", "price": 500})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestUpdate_ZeroPrice_Returns400(t *testing.T) {
	ctrl := newProductCtrl(nil, nil)
	rec := muxRequest(t, "PUT", "PUT /api/products/{id}", "/api/products/1",
		ctrl.Update, map[string]any{"name": "商品", "price": 0})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestUpdate_Valid_Returns200(t *testing.T) {
	ctrl := newProductCtrl(nil, nil)
	rec := muxRequest(t, "PUT", "PUT /api/products/{id}", "/api/products/1",
		ctrl.Update, map[string]any{"name": "更新商品", "price": 800})
	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
}

func TestUpdate_Forbidden_Returns403(t *testing.T) {
	ctrl := newProductCtrl(&mockProductDao{
		updateFn: func(_ string, _ int64, _ *model.SaveProductRequest) error {
			return dao.ErrForbidden
		},
	}, nil)
	rec := muxRequest(t, "PUT", "PUT /api/products/{id}", "/api/products/1",
		ctrl.Update, map[string]any{"name": "商品", "price": 500})
	if rec.Code != http.StatusForbidden {
		t.Errorf("got %d, want 403", rec.Code)
	}
}

// ─── Delete ──────────────────────────────────────────────────────────────────

func TestDelete_InvalidId_Returns400(t *testing.T) {
	ctrl := newProductCtrl(nil, nil)
	rec := muxRequest(t, "DELETE", "DELETE /api/products/{id}", "/api/products/abc",
		ctrl.Delete, nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestDelete_Valid_Returns200(t *testing.T) {
	ctrl := newProductCtrl(nil, nil)
	rec := muxRequest(t, "DELETE", "DELETE /api/products/{id}", "/api/products/1",
		ctrl.Delete, nil)
	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
}

func TestDelete_NotFound_Returns404(t *testing.T) {
	ctrl := newProductCtrl(&mockProductDao{
		deleteFn: func(_ string, _ int64) error { return dao.ErrNotFound },
	}, nil)
	rec := muxRequest(t, "DELETE", "DELETE /api/products/{id}", "/api/products/1",
		ctrl.Delete, nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

func TestDelete_Forbidden_Returns403(t *testing.T) {
	ctrl := newProductCtrl(&mockProductDao{
		deleteFn: func(_ string, _ int64) error { return dao.ErrForbidden },
	}, nil)
	rec := muxRequest(t, "DELETE", "DELETE /api/products/{id}", "/api/products/1",
		ctrl.Delete, nil)
	if rec.Code != http.StatusForbidden {
		t.Errorf("got %d, want 403", rec.Code)
	}
}

// ─── Like / Unlike ───────────────────────────────────────────────────────────

func TestLike_InvalidId_Returns400(t *testing.T) {
	ctrl := newProductCtrl(nil, nil)
	rec := muxRequest(t, "POST", "POST /api/products/{id}/like", "/api/products/abc/like",
		ctrl.Like, nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestLike_Valid_Returns200(t *testing.T) {
	ctrl := newProductCtrl(nil, nil)
	rec := muxRequest(t, "POST", "POST /api/products/{id}/like", "/api/products/1/like",
		ctrl.Like, nil)
	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
}

func TestLike_Conflict_Returns409(t *testing.T) {
	ctrl := newProductCtrl(&mockProductDao{
		likeFn: func(_ string, _ int64) error { return dao.ErrConflict },
	}, nil)
	rec := muxRequest(t, "POST", "POST /api/products/{id}/like", "/api/products/1/like",
		ctrl.Like, nil)
	if rec.Code != http.StatusConflict {
		t.Errorf("got %d, want 409", rec.Code)
	}
}

func TestUnlike_InvalidId_Returns400(t *testing.T) {
	ctrl := newProductCtrl(nil, nil)
	rec := muxRequest(t, "DELETE", "DELETE /api/products/{id}/like", "/api/products/abc/like",
		ctrl.Unlike, nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestUnlike_Valid_Returns200(t *testing.T) {
	ctrl := newProductCtrl(nil, nil)
	rec := muxRequest(t, "DELETE", "DELETE /api/products/{id}/like", "/api/products/1/like",
		ctrl.Unlike, nil)
	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
}

// ─── Purchase ────────────────────────────────────────────────────────────────

func TestPurchase_InvalidId_Returns400(t *testing.T) {
	ctrl := newProductCtrl(nil, nil)
	rec := muxRequest(t, "POST", "POST /api/products/{id}/purchase", "/api/products/abc/purchase",
		ctrl.Purchase, nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestPurchase_Valid_Returns200WithPrice(t *testing.T) {
	ctrl := newProductCtrl(&mockProductDao{
		purchaseFn: func(_ string, _ int64) (int, string, string, error) {
			return 3000, "seller1", "テスト商品", nil
		},
	}, nil)
	rec := muxRequest(t, "POST", "POST /api/products/{id}/purchase", "/api/products/1/purchase",
		ctrl.Purchase, nil)
	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["price"] == nil {
		t.Errorf("response has no price: %v", body)
	}
}

func TestPurchase_OwnProduct_Returns400(t *testing.T) {
	ctrl := newProductCtrl(&mockProductDao{
		purchaseFn: func(_ string, _ int64) (int, string, string, error) {
			return 0, "", "", dao.ErrOwnProduct
		},
	}, nil)
	rec := muxRequest(t, "POST", "POST /api/products/{id}/purchase", "/api/products/1/purchase",
		ctrl.Purchase, nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestPurchase_Conflict_Returns409(t *testing.T) {
	ctrl := newProductCtrl(&mockProductDao{
		purchaseFn: func(_ string, _ int64) (int, string, string, error) {
			return 0, "", "", dao.ErrConflict
		},
	}, nil)
	rec := muxRequest(t, "POST", "POST /api/products/{id}/purchase", "/api/products/1/purchase",
		ctrl.Purchase, nil)
	if rec.Code != http.StatusConflict {
		t.Errorf("got %d, want 409", rec.Code)
	}
}

// ─── Ship ────────────────────────────────────────────────────────────────────

func TestShip_InvalidId_Returns400(t *testing.T) {
	ctrl := newProductCtrl(nil, nil)
	rec := muxRequest(t, "PUT", "PUT /api/products/{id}/ship", "/api/products/abc/ship",
		ctrl.Ship, nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestShip_Valid_Returns200(t *testing.T) {
	ctrl := newProductCtrl(nil, nil)
	rec := muxRequest(t, "PUT", "PUT /api/products/{id}/ship", "/api/products/1/ship",
		ctrl.Ship, nil)
	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
}

func TestShip_Forbidden_Returns403(t *testing.T) {
	ctrl := newProductCtrl(&mockProductDao{
		shipFn: func(_ string, _ int64) (string, string, error) {
			return "", "", dao.ErrForbidden
		},
	}, nil)
	rec := muxRequest(t, "PUT", "PUT /api/products/{id}/ship", "/api/products/1/ship",
		ctrl.Ship, nil)
	if rec.Code != http.StatusForbidden {
		t.Errorf("got %d, want 403", rec.Code)
	}
}

// ─── MyPurchases ─────────────────────────────────────────────────────────────

func TestMyPurchases_ReviewedFlag(t *testing.T) {
	reviewedProductId := int64(10)
	ctrl := NewProductController(
		&mockProductDao{
			findPurchasesFn: func(_ string) ([]model.ProductSummary, error) {
				return []model.ProductSummary{
					{ProductId: reviewedProductId},
					{ProductId: 20},
				}, nil
			},
		},
		&mockNotificationDao{},
		&mockReviewDao{
			findByProductAndReviewerFn: func(productId int64, _ string) (*model.Review, error) {
				if productId == reviewedProductId {
					return &model.Review{}, nil // 評価済み
				}
				return nil, nil // 未評価
			},
		},
		&mockUserDao{},
	)

	req := authedRequest(t, "GET", "/api/me/purchases", nil)
	rec := serveAuthed(ctrl.MyPurchases, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
	var rows []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &rows)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0]["reviewed"] != true {
		t.Errorf("rows[0].reviewed = %v, want true", rows[0]["reviewed"])
	}
	if rows[1]["reviewed"] != false {
		t.Errorf("rows[1].reviewed = %v, want false", rows[1]["reviewed"])
	}
}
