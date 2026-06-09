package controller

import (
	"encoding/json"
	"log"
	"net/http"

	"main/usecase"
)

type RegisterUserController struct {
	registerUserUsecase *usecase.RegisterUserUsecase
}

func NewRegisterUserController(u *usecase.RegisterUserUsecase) *RegisterUserController {
	return &RegisterUserController{registerUserUsecase: u}
}

// Request / Response用の構造体定義
type UserReqForHTTPPost struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

type UserResForHTTPPost struct {
	Id string `json:"id"`
}

func (c *RegisterUserController) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var req UserReqForHTTPPost
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("fail: json.Decode, %v\n", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	id, err := c.registerUserUsecase.Execute(req.Name, req.Age)
	if err != nil {
		log.Printf("fail: RegisterUserUsecase, %v\n", err)
		w.WriteHeader(http.StatusBadRequest) // バリデーションエラーや内部エラー
		return
	}

	res := UserResForHTTPPost{Id: id}
	bytes, err := json.Marshal(res)
	if err != nil {
		log.Printf("fail: json.Marshal, %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(bytes)
}
