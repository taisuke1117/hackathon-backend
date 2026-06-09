package usecase

import (
	"errors"
	_ "errors"
	"main/dao"
	"main/model"
)

type SearchUserUsecase struct {
	userDao *dao.UserDao
}

func NewSearchUserUsecase(userDao *dao.UserDao) *SearchUserUsecase {
	return &SearchUserUsecase{userDao: userDao}
}

func (u *SearchUserUsecase) Execute(name string) ([]model.User, error) {
	if name == "" {
		return nil, errors.New("name is empty")
	}
	return u.userDao.FindByName(name)
}
