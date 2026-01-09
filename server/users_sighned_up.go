package server

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

func EncryptPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("password cannot be empty")
	}
	encrypted, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(encrypted), nil
}

func ComparePasswords(hashedPassword, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}
