package dao

import (
	"database/sql"
	"main/model"
)

type UserDao struct {
	DB *sql.DB
}

func NewUserDao(db *sql.DB) *UserDao {
	return &UserDao{DB: db}
}

// ユーザーの取得
func (d *UserDao) FindByName(name string) ([]model.User, error) {
	rows, err := d.DB.Query("SELECT id, name, age FROM user WHERE name = ?", name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]model.User, 0)
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.Id, &u.Name, &u.Age); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

// ユーザーをDBに新規登録する（トランザクション対応）
func (d *UserDao) Create(tx *sql.Tx, user model.User) error {
	_, err := tx.Exec("INSERT INTO user (id, name, age) VALUES (?, ?, ?)", user.Id, user.Name, user.Age)
	return err
}
