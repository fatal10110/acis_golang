package model

import (
	"bytes"
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

// ---- from gameserver_test.go ----
func TestHexKeyTextRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		key  []byte
		want string
	}{
		{name: "positive", key: []byte{0x01, 0x02, 0x03}, want: "10203"},
		{name: "negative", key: []byte{0x80, 0x01}, want: "-7fff"},
		{name: "positive with sign byte", key: []byte{0x00, 0x80, 0x01}, want: "8001"},
		{name: "negative one", key: []byte{0xff}, want: "-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HexKeyText(tt.key)
			if got != tt.want {
				t.Fatalf("HexKeyText(%x) = %q, want %q", tt.key, got, tt.want)
			}

			roundTrip, err := ParseHexKey(got)
			if err != nil {
				t.Fatalf("ParseHexKey(%q) unexpected error: %v", got, err)
			}
			if !bytes.Equal(roundTrip, tt.key) {
				t.Fatalf("ParseHexKey(%q) = %x, want %x", got, roundTrip, tt.key)
			}
		})
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
