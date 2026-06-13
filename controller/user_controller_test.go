package controller

import (
	"encoding/json"
	"net/http"
	"testing"
)

// newUserCtrl テスト用のUserControllerを組み立てる
func newUserCtrl(u *mockUserDao) *UserController {
	if u == nil {
		u = &mockUserDao{}
	}
	return NewUserController(u, &mockProductDao{}, &mockReviewDao{}, &mockNotificationDao{})
}

// ─── Register ────────────────────────────────────────────────────────────────

func TestRegister_MissingName_Returns400(t *testing.T) {
	ctrl := newUserCtrl(nil)
	req := authedRequest(t, "POST", "/api/users", map[string]any{
		"name": "", "mail": "test@example.com",
	})
	rec := serveAuthed(ctrl.Register, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestRegister_MissingMail_Returns400(t *testing.T) {
	ctrl := newUserCtrl(nil)
	req := authedRequest(t, "POST", "/api/users", map[string]any{
		"name": "テストユーザー", "mail": "",
	})
	rec := serveAuthed(ctrl.Register, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestRegister_Valid_Returns200(t *testing.T) {
	ctrl := newUserCtrl(nil)
	req := authedRequest(t, "POST", "/api/users", map[string]any{
		"name": "テストユーザー", "mail": "test@example.com",
	})
	rec := serveAuthed(ctrl.Register, req)
	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["user_id"] == nil {
		t.Errorf("response has no user_id: %v", body)
	}
}

// ─── UpdateMe ────────────────────────────────────────────────────────────────

func TestUpdateMe_MissingName_Returns400(t *testing.T) {
	ctrl := newUserCtrl(nil)
	req := authedRequest(t, "PUT", "/api/users/me", map[string]any{
		"name": "",
	})
	rec := serveAuthed(ctrl.UpdateMe, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestUpdateMe_Valid_Returns200(t *testing.T) {
	ctrl := newUserCtrl(nil)
	req := authedRequest(t, "PUT", "/api/users/me", map[string]any{
		"name": "新しい名前",
	})
	rec := serveAuthed(ctrl.UpdateMe, req)
	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
}

// ─── Block ───────────────────────────────────────────────────────────────────

func TestBlock_MissingBlockedId_Returns400(t *testing.T) {
	ctrl := newUserCtrl(nil)
	req := authedRequest(t, "POST", "/api/blocks", map[string]any{
		"blocked_id": "",
	})
	rec := serveAuthed(ctrl.Block, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestBlock_Valid_Returns200(t *testing.T) {
	ctrl := newUserCtrl(nil)
	req := authedRequest(t, "POST", "/api/blocks", map[string]any{
		"blocked_id": "target-user-id",
	})
	rec := serveAuthed(ctrl.Block, req)
	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
}
