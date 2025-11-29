package inmem

import (
	"errors"
	"time"

	"github.com/grodier/rss-go/internal/models"
	"golang.org/x/crypto/bcrypt"
)

type UserService struct {
	users []*models.User
}

func NewUserService() *UserService {
	return &UserService{}
}

func (s *UserService) CreateUser(user *models.UserInput) error {
	for _, existingUser := range s.users {
		if existingUser.Email == user.Email {
			return models.ErrDuplicateEmail
		}
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), 12)
	if err != nil {
		return err
	}

	newUser := &models.User{
		ID:             len(s.users) + 1,
		Name:           user.Name,
		Email:          user.Email,
		HashedPassword: hashedPassword,
		CreatedAt:      time.Now(),
	}

	s.users = append(s.users, newUser)

	return nil
}

func (s *UserService) Authenticate(email, password string) (int, error) {
	for _, user := range s.users {
		if user.Email == email {
			err := bcrypt.CompareHashAndPassword(user.HashedPassword, []byte(password))
			if err != nil {
				if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
					return 0, models.ErrInvalidCredentials
				} else {
					return 0, err
				}
			}

			return user.ID, nil
		}
	}

	return 0, models.ErrInvalidCredentials
}

func (s *UserService) Exists(id int) (bool, error) {
	for _, user := range s.users {
		if user.ID == id {
			return true, nil
		}
	}
	return false, nil
}
