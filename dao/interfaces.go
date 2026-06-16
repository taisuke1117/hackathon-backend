package dao

import "main/model"

// IProductDao ProductDaoのインターフェース（テスト時にモックに差し替え可能にする）
type IProductDao interface {
	Search(viewerId string, c *model.SearchCondition) ([]model.ProductSummary, error)
	FindDetail(viewerId string, productId int64) (*model.ProductDetail, error)
	Create(sellerId string, req *model.SaveProductRequest) (int64, error)
	UpdateProduct(sellerId string, productId int64, req *model.SaveProductRequest) error
	Delete(sellerId string, productId int64) error
	Purchase(buyerId string, productId int64) (int, string, string, error)
	Ship(sellerId string, productId int64) (string, string, error)
	FindBySeller(sellerId string) ([]model.ProductSummary, error)
	GetSellerSummary(sellerId string) (*model.SellerSummary, error)
	FindPurchases(buyerId string) ([]model.ProductSummary, error)
	FindLiked(userId string) ([]model.ProductSummary, error)
	Like(userId string, productId int64) error
	Unlike(userId string, productId int64) error
}

// INotificationDao NotificationDaoのインターフェース
type INotificationDao interface {
	Create(userId, notifType, title, content, linkUrl string) error
	ListByUser(userId string) ([]model.Notification, error)
	MarkRead(userId string, notificationId int64) error
}

// IReviewDao ReviewDaoのインターフェース
type IReviewDao interface {
	FindByProductAndReviewer(productId int64, reviewerId string) (*model.Review, error)
	Create(reviewerId string, productId int64, rating int, comment string) (string, string, error)
	ListByReviewee(revieweeId string) ([]model.Review, error)
}

// IUserDao UserDaoのインターフェース
type IUserDao interface {
	FindById(userId string) (*model.User, error)
	Upsert(userId string, req *model.RegisterUserRequest) error
	Update(userId string, req *model.UpdateUserRequest) error
	Block(blockerId, blockedId string) error
	GetBadges(userId string) (int, int, int, error)
	GetAllCategories() ([]model.Category, error)
	GetLikedProductIds(userId string) ([]int64, error)
}
