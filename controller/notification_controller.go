package controller

import (
	"main/dao"
	"main/middleware"
	"net/http"
)

type NotificationController struct {
	notificationDao *dao.NotificationDao
}

func NewNotificationController(notificationDao *dao.NotificationDao) *NotificationController {
	return &NotificationController{notificationDao: notificationDao}
}

// List GET /api/notifications
func (c *NotificationController) List(w http.ResponseWriter, r *http.Request) {
	list, err := c.notificationDao.ListByUser(middleware.UserId(r))
	if err != nil {
		handleDaoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// MarkRead PUT /api/notifications/{id}/read
func (c *NotificationController) MarkRead(w http.ResponseWriter, r *http.Request) {
	id, ok := pathId(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "通知IDが不正です")
		return
	}
	if err := c.notificationDao.MarkRead(middleware.UserId(r), id); err != nil {
		handleDaoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "read"})
}
