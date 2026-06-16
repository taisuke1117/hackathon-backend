package controller

import "main/model"

// ─── ProductDao モック ───────────────────────────────────────────────────────

type mockProductDao struct {
	createFn        func(sellerId string, req *model.SaveProductRequest) (int64, error)
	updateFn        func(sellerId string, productId int64, req *model.SaveProductRequest) error
	deleteFn        func(sellerId string, productId int64) error
	purchaseFn      func(buyerId string, productId int64) (int, string, string, error)
	shipFn          func(sellerId string, productId int64) (string, string, error)
	likeFn          func(userId string, productId int64) error
	unlikeFn        func(userId string, productId int64) error
	findPurchasesFn func(buyerId string) ([]model.ProductSummary, error)
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
func (m *mockProductDao) UpdateProduct(sellerId string, productId int64, req *model.SaveProductRequest) error {
	if m.updateFn != nil {
		return m.updateFn(sellerId, productId, req)
	}
	return nil
}
func (m *mockProductDao) Delete(sellerId string, productId int64) error {
	if m.deleteFn != nil {
		return m.deleteFn(sellerId, productId)
	}
	return nil
}
func (m *mockProductDao) Purchase(buyerId string, productId int64) (int, string, string, error) {
	if m.purchaseFn != nil {
		return m.purchaseFn(buyerId, productId)
	}
	return 1000, "seller1", "テスト商品", nil
}
func (m *mockProductDao) Ship(sellerId string, productId int64) (string, string, error) {
	if m.shipFn != nil {
		return m.shipFn(sellerId, productId)
	}
	return "buyer1", "テスト商品", nil
}
func (m *mockProductDao) FindBySeller(_ string) ([]model.ProductSummary, error) {
	return nil, nil
}
func (m *mockProductDao) GetSellerSummary(_ string) (*model.SellerSummary, error) {
	return &model.SellerSummary{}, nil
}
func (m *mockProductDao) FindPurchases(buyerId string) ([]model.ProductSummary, error) {
	if m.findPurchasesFn != nil {
		return m.findPurchasesFn(buyerId)
	}
	return nil, nil
}
func (m *mockProductDao) FindLiked(_ string) ([]model.ProductSummary, error) { return nil, nil }
func (m *mockProductDao) Like(userId string, productId int64) error {
	if m.likeFn != nil {
		return m.likeFn(userId, productId)
	}
	return nil
}
func (m *mockProductDao) Unlike(userId string, productId int64) error {
	if m.unlikeFn != nil {
		return m.unlikeFn(userId, productId)
	}
	return nil
}

// ─── NotificationDao モック ──────────────────────────────────────────────────

type mockNotificationDao struct{}

func (m *mockNotificationDao) Create(_, _, _, _, _ string) error              { return nil }
func (m *mockNotificationDao) ListByUser(_ string) ([]model.Notification, error) {
	return nil, nil
}
func (m *mockNotificationDao) MarkRead(_ string, _ int64) error { return nil }

// ─── ReviewDao モック ────────────────────────────────────────────────────────

type mockReviewDao struct {
	createFn                    func(reviewerId string, productId int64, rating int, comment string) (string, string, error)
	findByProductAndReviewerFn func(productId int64, reviewerId string) (*model.Review, error)
}

func (m *mockReviewDao) FindByProductAndReviewer(productId int64, reviewerId string) (*model.Review, error) {
	if m.findByProductAndReviewerFn != nil {
		return m.findByProductAndReviewerFn(productId, reviewerId)
	}
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
func (m *mockUserDao) GetBadges(_ string) (int, int, int, error)          { return 0, 0, 0, nil }
func (m *mockUserDao) GetAllCategories() ([]model.Category, error)        { return nil, nil }
func (m *mockUserDao) GetLikedProductIds(_ string) ([]int64, error)       { return nil, nil }
