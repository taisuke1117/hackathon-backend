package controller

import (
	"fmt"
	"main/dao"
	"main/middleware"
	"main/model"
	"net/http"
	"strconv"
)

type ProductController struct {
	productDao      *dao.ProductDao
	notificationDao *dao.NotificationDao
	reviewDao       *dao.ReviewDao
	userDao         *dao.UserDao
}

func NewProductController(productDao *dao.ProductDao, notificationDao *dao.NotificationDao, reviewDao *dao.ReviewDao, userDao *dao.UserDao) *ProductController {
	return &ProductController{productDao: productDao, notificationDao: notificationDao, reviewDao: reviewDao, userDao: userDao}
}

// List GET /api/products?keyword=&category_id=&min_price=&max_price=
func (c *ProductController) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cond := &model.SearchCondition{Keyword: q.Get("keyword")}
	cond.CategoryId, _ = strconv.ParseInt(q.Get("category_id"), 10, 64)
	cond.MinPrice, _ = strconv.Atoi(q.Get("min_price"))
	cond.MaxPrice, _ = strconv.Atoi(q.Get("max_price"))

	products, err := c.productDao.Search(middleware.UserId(r), cond)
	if err != nil {
		handleDaoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, products)
}

// Detail GET /api/products/{id}
func (c *ProductController) Detail(w http.ResponseWriter, r *http.Request) {
	id, ok := pathId(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "商品IDが不正です")
		return
	}
	uid := middleware.UserId(r)
	product, err := c.productDao.FindDetail(uid, id)
	if err != nil {
		handleDaoError(w, err)
		return
	}
	// 出品者プロフィール（評価込み）を同梱
	if seller, err := c.userDao.FindById(product.SellerId); err == nil {
		seller.Mail = ""
		seller.ShippingAddress = ""
		product.Seller = seller
	}
	writeJSON(w, http.StatusOK, product)
}

// Create POST /api/products — 出品
func (c *ProductController) Create(w http.ResponseWriter, r *http.Request) {
	var req model.SaveProductRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Name == "" || req.Price <= 0 {
		writeError(w, http.StatusBadRequest, "nameと正の価格priceは必須です")
		return
	}
	productId, err := c.productDao.Create(middleware.UserId(r), &req)
	if err != nil {
		handleDaoError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"product_id": productId})
}

// Update PUT /api/products/{id} — 商品編集
func (c *ProductController) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathId(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "商品IDが不正です")
		return
	}
	var req model.SaveProductRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Name == "" || req.Price <= 0 {
		writeError(w, http.StatusBadRequest, "nameと正の価格priceは必須です")
		return
	}
	if err := c.productDao.UpdateProduct(middleware.UserId(r), id, &req); err != nil {
		handleDaoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// Delete DELETE /api/products/{id} — 出品取り消し
func (c *ProductController) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathId(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "商品IDが不正です")
		return
	}
	if err := c.productDao.Delete(middleware.UserId(r), id); err != nil {
		handleDaoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// Purchase POST /api/products/{id}/purchase — 購入確定
func (c *ProductController) Purchase(w http.ResponseWriter, r *http.Request) {
	id, ok := pathId(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "商品IDが不正です")
		return
	}
	uid := middleware.UserId(r)
	price, sellerId, productName, err := c.productDao.Purchase(uid, id)
	if err != nil {
		handleDaoError(w, err)
		return
	}
	// 出品者へ通知（失敗しても購入自体は成立しているのでエラーにしない）
	_ = c.notificationDao.Create(sellerId, "transaction",
		"商品が購入されました",
		fmt.Sprintf("「%s」が購入されました。発送準備を進めてください。", productName),
		fmt.Sprintf("/deals/manage/%d", id))

	writeJSON(w, http.StatusOK, map[string]any{"status": "purchased", "price": price})
}

// Ship PUT /api/products/{id}/ship — 発送完了
func (c *ProductController) Ship(w http.ResponseWriter, r *http.Request) {
	id, ok := pathId(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "商品IDが不正です")
		return
	}
	buyerId, productName, err := c.productDao.Ship(middleware.UserId(r), id)
	if err != nil {
		handleDaoError(w, err)
		return
	}
	// 受取評価ボタンのある購入履歴ページへ誘導する
	_ = c.notificationDao.Create(buyerId, "transaction",
		"発送完了のお知らせ",
		fmt.Sprintf("「%s」の発送手続きが完了しました。商品が届いたら受取評価をお願いします。", productName),
		"/mypage/purchases")

	writeJSON(w, http.StatusOK, map[string]string{"status": "shipped"})
}

// MyProducts GET /api/me/products — 自分の出品一覧＋サマリー
func (c *ProductController) MyProducts(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserId(r)
	products, err := c.productDao.FindBySeller(uid)
	if err != nil {
		handleDaoError(w, err)
		return
	}
	summary, err := c.productDao.GetSellerSummary(uid)
	if err != nil {
		handleDaoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"summary": summary, "products": products})
}

// MyPurchases GET /api/me/purchases — 購入履歴（評価済みフラグつき）
func (c *ProductController) MyPurchases(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserId(r)
	products, err := c.productDao.FindPurchases(uid)
	if err != nil {
		handleDaoError(w, err)
		return
	}
	type purchaseRow struct {
		model.ProductSummary
		Reviewed bool `json:"reviewed"`
	}
	rows := make([]purchaseRow, 0, len(products))
	for _, p := range products {
		rv, _ := c.reviewDao.FindByProductAndReviewer(p.ProductId, uid)
		rows = append(rows, purchaseRow{ProductSummary: p, Reviewed: rv != nil})
	}
	writeJSON(w, http.StatusOK, rows)
}

// MyLikes GET /api/me/likes — いいね一覧
func (c *ProductController) MyLikes(w http.ResponseWriter, r *http.Request) {
	products, err := c.productDao.FindLiked(middleware.UserId(r))
	if err != nil {
		handleDaoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, products)
}

// Like POST /api/products/{id}/like ・ Unlike DELETE 同
func (c *ProductController) Like(w http.ResponseWriter, r *http.Request) {
	id, ok := pathId(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "商品IDが不正です")
		return
	}
	if err := c.productDao.Like(middleware.UserId(r), id); err != nil {
		handleDaoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "liked"})
}

func (c *ProductController) Unlike(w http.ResponseWriter, r *http.Request) {
	id, ok := pathId(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "商品IDが不正です")
		return
	}
	if err := c.productDao.Unlike(middleware.UserId(r), id); err != nil {
		handleDaoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "unliked"})
}

// CreateReview POST /api/products/{id}/reviews — 受取評価
func (c *ProductController) CreateReview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathId(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "商品IDが不正です")
		return
	}
	var req struct {
		Rating  int    `json:"rating"`
		Comment string `json:"comment"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Rating < 1 || req.Rating > 5 {
		writeError(w, http.StatusBadRequest, "ratingは1〜5で指定してください")
		return
	}
	uid := middleware.UserId(r)
	sellerId, productName, err := c.reviewDao.Create(uid, id, req.Rating, req.Comment)
	if err != nil {
		handleDaoError(w, err)
		return
	}
	_ = c.notificationDao.Create(sellerId, "transaction",
		"受取評価が届きました",
		fmt.Sprintf("「%s」の取引で評価（★%d）を受け取りました。", productName, req.Rating),
		"/deals")

	writeJSON(w, http.StatusCreated, map[string]string{"status": "reviewed"})
}

// UserReviews GET /api/users/{id}/reviews — ユーザーが受け取った評価一覧
func (c *ProductController) UserReviews(w http.ResponseWriter, r *http.Request) {
	reviews, err := c.reviewDao.ListByReviewee(r.PathValue("id"))
	if err != nil {
		handleDaoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, reviews)
}
