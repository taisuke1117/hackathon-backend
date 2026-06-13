package controller

import "main/model"

// ─── ProductDao モック ───────────────────────────────────────────────────────

type mockProductDao struct {
	createFn func(sellerId string, req *model.SaveProductRequest) (int64, error)
}

func (m *mockProductDao) Search(_ string, _ *model.SearchCondition) ([]model.ProductSummary, error) {
	return nil, nil
}
func (m *mockProductDao) FindDetail(_ string, _ int64) (*model.ProductDetail, error) {
	return nil, nil
}
func (m *mockProductDao) Create(sellerId string, req *model.SaveProductRequest) (int64, error) {
	if m.createFn != nil {
		return m.createFn(sellerId, req)
	}
	return 1, nil
}
func (m *mockProductDao) UpdateProduct(_ string, _ int64, _ *model.SaveProductRequest) error {
	return nil
}
func (m *mockProductDao) Delete(_ string, _ int64) error              { return nil }
func (m *mockProductDao) Purchase(_ string, _ int64) (int, string, string, error) {
	return 0, "", "", nil
}
func (m *mockProductDao) Ship(_ string, _ int64) (string, string, error) { return "", "", nil }
func (m *mockProductDao) FindBySeller(_ string) ([]model.ProductSummary, error) {
	return nil, nil
}
func (m *mockProductDao) GetSellerSummary(_ string) (*model.SellerSummary, error) {
	return &model.SellerSummary{}, nil
}
func (m *mockProductDao) FindPurchases(_ string) ([]model.ProductSummary, error) { return nil, nil }
func (m *mockProductDao) FindLiked(_ string) ([]model.ProductSummary, error)     { return nil, nil }
func (m *mockProductDao) Like(_ string, _ int64) error                           { return nil }
func (m *mockProductDao) Unlike(_ string, _ int64) error                         { return nil }

// ─── NotificationDao モック ──────────────────────────────────────────────────

type mockNotificationDao struct{}

func (m *mockNotificationDao) Create(_, _, _, _, _ string) error              { return nil }
func (m *mockNotificationDao) ListByUser(_ string) ([]model.Notification, error) {
	return nil, nil
}
func (m *mockNotificationDao) MarkRead(_ string, _ int64) error { return nil }

// ─── ReviewDao モック ────────────────────────────────────────────────────────

type mockReviewDao struct {
	createFn func(reviewerId string, productId int64, rating int, comment string) (string, string, error)
}

func (m *mockReviewDao) FindByProductAndReviewer(_ int64, _ string) (*model.Review, error) {
	return nil, nil
}
func (m *mockReviewDao) Create(reviewerId string, productId int64, rating int, comment string) (string, string, error) {
	if m.createFn != nil {
		return m.createFn(reviewerId, productId, rating, comment)
	}
	return "seller1", "商品名", nil
}
func (m *mockReviewDao) ListByReviewee(_ string) ([]model.Review, error) { return nil, nil }

// ─── UserDao モック ──────────────────────────────────────────────────────────

type mockUserDao struct {
	upsertFn func(userId string, req *model.RegisterUserRequest) error
	updateFn func(userId string, req *model.UpdateUserRequest) error
	blockFn  func(blockerId, blockedId string) error
	findByIdFn func(userId string) (*model.User, error)
}

func (m *mockUserDao) FindById(userId string) (*model.User, error) {
	if m.findByIdFn != nil {
		return m.findByIdFn(userId)
	}
	return &model.User{UserId: userId, Name: "テストユーザー"}, nil
}
func (m *mockUserDao) Upsert(userId string, req *model.RegisterUserRequest) error {
	if m.upsertFn != nil {
		return m.upsertFn(userId, req)
	}
	return nil
}
func (m *mockUserDao) Update(userId string, req *model.UpdateUserRequest) error {
	if m.updateFn != nil {
		return m.updateFn(userId, req)
	}
	return nil
}
func (m *mockUserDao) Block(blockerId, blockedId string) error {
	if m.blockFn != nil {
		return m.blockFn(blockerId, blockedId)
	}
	return nil
}
func (m *mockUserDao) GetBadges(_ string) (int, int, error)               { return 0, 0, nil }
func (m *mockUserDao) GetAllCategories() ([]model.Category, error)        { return nil, nil }
func (m *mockUserDao) GetLikedProductIds(_ string) ([]int64, error)       { return nil, nil }
