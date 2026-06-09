package controller

import (
	"encoding/json"
	"log"
	"main/usecase"
	"net/http"
)

type SearchUserController struct {
	searchUserUsecase *usecase.SearchUserUsecase
}

func NewSearchUserController(u *usecase.SearchUserUsecase) *SearchUserController {
	return &SearchUserController{searchUserUsecase: u}
}

// Response用の構造体定義
type UserResForHTTPGet struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func (c *SearchUserController) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	name := r.URL.Query().Get("name")
	users, err := c.searchUserUsecase.Execute(name)
	if err != nil {
		log.Printf("fail: SearchUserUsecase, %v\n", err)
		w.WriteHeader(http.StatusBadRequest) // バリデーションエラー等
		return
	}

	// model.User のスライスを レスポンス用構造体 のスライスに詰め替える
	resUsers := make([]UserResForHTTPGet, len(users))
	for i, u := range users {
		resUsers[i] = UserResForHTTPGet{Id: u.Id, Name: u.Name, Age: u.Age}
	}

	bytes, err := json.Marshal(resUsers)
	if err != nil {
		log.Printf("fail: json.Marshal, %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(bytes)
}
