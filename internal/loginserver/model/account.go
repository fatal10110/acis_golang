package model

import "strings"

// Account is a login server account record.
type Account struct {
	Login       string
	Password    string
	AccessLevel int
	LastServer  int
}

// NewAccount returns an Account with Login normalized to lowercase, since
// logins are case-insensitive but stored and compared verbatim.
func NewAccount(login, password string, accessLevel, lastServer int) Account {
	return Account{
		Login:       strings.ToLower(login),
		Password:    password,
		AccessLevel: accessLevel,
		LastServer:  lastServer,
	}
}
