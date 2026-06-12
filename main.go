package main

import (
	"database/sql"
	"log"
	"main/controller"
	"main/dao"
	"main/db"
	"main/router"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	database := db.Connect()
	defer closeDBWithSysCall(database)

	userDao := dao.NewUserDao(database)
	productDao := dao.NewProductDao(database)
	chatDao := dao.NewChatDao(database)
	notificationDao := dao.NewNotificationDao(database)
	reviewDao := dao.NewReviewDao(database)

	userCtrl := controller.NewUserController(userDao, productDao, reviewDao)
	productCtrl := controller.NewProductController(productDao, notificationDao, reviewDao, userDao)
	chatCtrl := controller.NewChatController(chatDao, notificationDao)
	notificationCtrl := controller.NewNotificationController(notificationDao)
	geminiCtrl := controller.NewGeminiController(userDao)

	handler := router.Setup(userCtrl, productCtrl, chatCtrl, notificationCtrl, geminiCtrl)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Listening on :%s...\n", port)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatal(err)
	}
}

func closeDBWithSysCall(database *sql.DB) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		s := <-sig
		log.Printf("received syscall: %v", s)
		if err := database.Close(); err != nil {
			log.Fatal(err)
		}
		log.Println("db closed")
		os.Exit(0)
	}()
}
