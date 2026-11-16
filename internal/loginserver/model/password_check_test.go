package model

import (
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// Reference vectors: every hash below and every outcome was produced by the
// reference server's bcrypt (hashPw/checkPw) on salt Ro0CUfOqk6cXEKf3dyaM7O.
// "error" is a thrown exception, which the login path answers with
// LoginFail ACCESS_FAILED without counting a failed attempt.
const (
	refSalt       = "Ro0CUfOqk6cXEKf3dyaM7O"
	refHash2a     = "$2a$04$Ro0CUfOqk6cXEKf3dyaM7OpNNVVUxK2/c53ykg4bn8cFRHYdF4UzO"
	refHash2      = "$2$04$Ro0CUfOqk6cXEKf3dyaM7O7fMbZhCvrR98qQgvC8MBoT7GmQXPRya"
	refHash2aCost = "$2a$05$Ro0CUfOqk6cXEKf3dyaM7O2o6Ro4eflEEhxmI5jilV/MQBg.AbZ9O"
	refHash2aNone = "$2a$04$Ro0CUfOqk6cXEKf3dyaM7OYRHXi1ra5lHU2sBmG5pcZrSzea72ahC"
	refHashTail   = "pNNVVUxK2/c53ykg4bn8cFRHYdF4UzO"
)

func TestCheckPasswordReferenceVectors(t *testing.T) {
	const ok, mismatch, unusable = "ok", "mismatch", "error"
	tests := []struct {
		name, hash, password, want string
	}{
		{"2a", refHash2a, "s3cret", ok},
		{"2a cost 5", refHash2aCost, "s3cret", ok},
		{"2", refHash2, "s3cret", ok},
		{"2a empty password", refHash2aNone, "", ok},
		{"2a wrong password", refHash2a, "wrong", mismatch},
		{"2 wrong password", refHash2, "wrong", mismatch},
		{"2 empty password", refHash2, "", unusable},
		{"2b", "$2b$04$" + refSalt + refHashTail, "s3cret", unusable},
		{"2y", "$2y$04$" + refSalt + refHashTail, "s3cret", unusable},
		{"2x", "$2x$04$" + refSalt + refHashTail, "s3cret", unusable},
		{"one-digit cost", "$2a$4$" + refSalt, "s3cret", unusable},
		{"cost 3", "$2a$03$" + refSalt + refHashTail, "s3cret", unusable},
		{"cost 31", "$2a$31$" + refSalt + refHashTail, "s3cret", unusable},
		{"cost -1", "$2a$-1$" + refSalt + refHashTail, "s3cret", unusable},
		{"cost +4", "$2a$+4$" + refSalt + refHashTail, "s3cret", mismatch},
		{"cost letters", "$2a$ab$" + refSalt + refHashTail, "s3cret", unusable},
		{"salt only", "$2a$04$" + refSalt, "s3cret", mismatch},
		{"short salt", "$2a$04$" + refSalt[:21], "s3cret", unusable},
		{"truncated hash", refHash2a[:40], "s3cret", mismatch},
		{"trailing byte", refHash2a + "x", "s3cret", mismatch},
		{"non-canonical salt", "$2a$04$" + refSalt[:21] + "v" + refHashTail, "s3cret", mismatch},
		{"bad salt head", "$2a$04$!!" + refSalt[2:] + refHashTail, "s3cret", unusable},
		{"bad salt last", "$2a$04$" + refSalt[:21] + "!" + refHashTail, "s3cret", unusable},
		{"md5 hex", "5f4dcc3b5aa765d61d8327deb882cf99", "s3cret", unusable},
		{"empty", "", "s3cret", unusable},
		{"bare 2", "$2", "s3cret", unusable},
		{"md5 crypt", "$1$abc", "s3cret", unusable},
		{"bare 2a", "$2a", "s3cret", unusable},
		{"no salt", "$2a$04", "s3cret", unusable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match, err := CheckPassword(tt.hash, tt.password)
			got := mismatch
			switch {
			case err != nil:
				if !errors.Is(err, ErrUnusableHash) {
					t.Fatalf("CheckPassword error %v does not wrap ErrUnusableHash", err)
				}
				got = unusable
			case match:
				got = ok
			}
			if got != tt.want {
				t.Fatalf("CheckPassword(%q, %q) = %s (err %v), want %s", tt.hash, tt.password, got, err, tt.want)
			}
		})
	}
}

// A hash the store itself produces keeps verifying.
func TestCheckPasswordAcceptsHashPassword(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if match, err := CheckPassword(string(hash), "s3cret"); err != nil || !match {
		t.Fatalf("CheckPassword(own hash) = %v, %v; want true, nil", match, err)
	}
}
