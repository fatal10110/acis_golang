package model

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHashPassword_RoundTrip(t *testing.T) {
	hash, err := HashPassword("s3cret")
	if err != nil {
		t.Fatalf("HashPassword() unexpected error: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("s3cret")); err != nil {
		t.Errorf("CompareHashAndPassword(correct) = %v, want nil", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("wrong")); err == nil {
		t.Error("CompareHashAndPassword(wrong) = nil, want error")
	}
}
