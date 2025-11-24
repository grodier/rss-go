package models

import "time"

type User struct {
	ID             int       `form:"-"`
	Name           string    `form:"name"`
	Email          string    `form:"email"`
	HashedPassword []byte    `form:"-"`
	CreatedAt      time.Time `form:"-"`
}

type UserService interface {
	CreateUser(user *User) error
	Authenticate(email, password string) (int, error)
	Exists(id int) (bool, error)
}
