package inmem

import (
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
	return 0, nil
}

func (s *UserService) Exists(id int) (bool, error) {
	return false, nil
}
