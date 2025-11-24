package inmem

import "github.com/grodier/rss-go/internal/models"

type UserService struct{}

func NewUserService() *UserService {
	return &UserService{}
}

func (s *UserService) CreateUser(user *models.User) error {
	return nil
}

func (s *UserService) Authenticate(email, password string) (int, error) {
	return 0, nil
}

func (s *UserService) Exists(id int) (bool, error) {
	return false, nil
}
