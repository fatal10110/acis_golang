package model

import "testing"

func TestNewAccount_LowercasesLogin(t *testing.T) {
	a := NewAccount("PlayerOne", "hash", 1, 2)
	if a.Login != "playerone" {
		t.Errorf("Login = %q, want %q", a.Login, "playerone")
	}
	if a.Password != "hash" || a.AccessLevel != 1 || a.LastServer != 2 {
		t.Errorf("NewAccount() = %+v, want Password=hash AccessLevel=1 LastServer=2", a)
	}
}
