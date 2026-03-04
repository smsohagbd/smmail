package service

import (
	"database/sql"
	"errors"

	"golang.org/x/crypto/bcrypt"
	"learn/smtp-platform/internal/models"
	"learn/smtp-platform/internal/repo"
)

type AuthService struct {
	users *repo.UserRepo
}

func NewAuthService(users *repo.UserRepo) *AuthService { return &AuthService{users: users} }

func (s *AuthService) Validate(username, password string) (models.User, error) {
	u, err := s.users.GetByUsername(username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.User{}, errors.New("invalid credentials")
		}
		return models.User{}, err
	}
	if !u.Enabled {
		return models.User{}, errors.New("account disabled")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return models.User{}, errors.New("invalid credentials")
	}
	return u, nil
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}