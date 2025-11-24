package models

import "time"

type User struct {
	ID             int
	Name           string
	Email          string
	HashedPassword []byte
	CreatedAt      time.Time
}

type UserService interface {
	CreateUser(user *User) error
	Authenticate(email, password string) (int, error)
	Exists(id int) (bool, error)
}
