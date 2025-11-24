package models

import "time"

type User struct {
	ID             int
	Name           string
	Email          string
	HashedPassword []byte
	CreatedAt      time.Time
}

type UserInput struct {
	Name     string `form:"name"`
	Email    string `form:"email"`
	Password string `form:"password"`
}

type UserService interface {
	CreateUser(user *UserInput) error
	Authenticate(email, password string) (int, error)
	Exists(id int) (bool, error)
}
