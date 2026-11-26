package model

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// ---- from account_test.go ----
func TestNewAccount_LowercasesLogin(t *testing.T) {
	a := NewAccount("PlayerOne", "hash", 1, 2)
	if a.Login != "playerone" {
		t.Errorf("Login = %q, want %q", a.Login, "playerone")
	}
	if a.Password != "hash" || a.AccessLevel != 1 || a.LastServer != 2 {
		t.Errorf("NewAccount() = %+v, want Password=hash AccessLevel=1 LastServer=2", a)
	}
}

// TestNewAccount_LoginNormalizationIsLocaleIndependent locks in that login
// normalization does not depend on the OS/JVM default locale: "I" always
// folds to "i", never the Turkish dotless "ı" that Java's
// login.toLowerCase() produces under a tr_TR default locale.
func TestNewAccount_LoginNormalizationIsLocaleIndependent(t *testing.T) {
	a := NewAccount("PLAYER_I", "hash", 0, 0)
	if a.Login != "player_i" {
		t.Errorf("Login = %q, want %q (locale-independent fold of \"I\")", a.Login, "player_i")
	}
}

// ---- from password_test.go ----
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

func TestHashPassword_AcceptsPasswordsLongerThan72Bytes(t *testing.T) {
	prefix := strings.Repeat("a", 72)
	hash, err := HashPassword(prefix + "ignored")
	if err != nil {
		t.Fatalf("HashPassword() unexpected error: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(prefix)); err != nil {
		t.Fatalf("CompareHashAndPassword(prefix) = %v, want nil", err)
	}
}
