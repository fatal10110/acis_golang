package model

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword returns a bcrypt hash of password suitable for storing in an
// Account's Password field.
func HashPassword(password string) (string, error) {
	if len(password) > 72 {
		password = password[:72] // bcrypt uses at most the first 72 bytes.
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}
