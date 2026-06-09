package usecase

import (
	"database/sql"
	"main/dao"
	"main/model"

	"errors"
	"math/rand"
	"time"
)

type RegisterUserUsecase struct {
	db      *sql.DB
	userDao *dao.UserDao
}

func NewRegisterUserUsecase(db *sql.DB, userDao *dao.UserDao) *RegisterUserUsecase {
	return &RegisterUserUsecase{db: db, userDao: userDao}
}

func (u *RegisterUserUsecase) Execute(name string, age int) (string, error) {
	// 1. バリデーションチェック
	if name == "" || len(name) > 50 {
		return "", errors.New("validation error on name (empty or > 50 chars)")
	}
	if age < 20 || age > 80 {
		return "", errors.New("validation error on age (out of 20-80 range)")
	}

	// 2. ULIDによるID採番
	t := time.Now()
	entropy := ulid.Monotonic(rand.New(rand.NewSource(t.UnixNano())), 0)
	id := ulid.MustNew(ulid.Timestamp(t), entropy).String()

	user := model.User{
		Id:   id,
		Name: name,
		Age:  age,
	}

	// 3. トランザクションの開始と実行
	tx, err := u.db.Begin()
	if err != nil {
		return "", err
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	if err := u.userDao.Create(tx, user); err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}

	return id, nil
}
