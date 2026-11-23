package main

import (
	"context"
	"strings"
	"testing"
)

// TestAccountCreateRefusesCredentialsTheClientCannotSend: a login over 14
// bytes or a password over 16 bytes is refused with a message and writes no
// row, while the longest ones a client can send are stored (#5048).
func TestAccountCreateRefusesCredentialsTheClientCannotSend(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	const refused = "the account was not created or updated"

	tests := []struct {
		name, login, password string
		wantStored            bool
	}{
		{name: "login over 14 bytes", login: "abcdefghijklmno", password: "s3cret"},
		{name: "password over 16 bytes", login: "player2", password: "abcdefghijklmnopq"},
		{name: "longest login and password", login: "abcdefghijklmn", password: "abcdefghijklmnop", wantStored: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := runScript(t, store, "1 "+tt.login+" "+tt.password+" 0 5")
			_, err := store.Account(ctx, tt.login)
			if tt.wantStored {
				if strings.Contains(out, refused) || err != nil {
					t.Fatalf("account not stored: err=%v output:\n%s", err, out)
				}
				return
			}
			if !strings.Contains(out, refused) {
				t.Fatalf("output missing the refusal:\n%s", out)
			}
			if strings.Contains(out, "has been created or updated") {
				t.Fatalf("refused account reported as created:\n%s", out)
			}
			if err == nil {
				t.Fatalf("account %s row written, want none", tt.login)
			}
		})
	}
}
