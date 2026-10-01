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
//
// Normalization is deliberately locale-independent (Unicode simple case
// folding via strings.ToLower). A fold that follows the server's default
// locale can map "I" to "ı" under a Turkish locale; such locale-dependent
// login normalization would make the resulting stored login nondeterministic
// across deployments and is not reproducible here without adding locale
// machinery the codebase does not otherwise need. Accounts created by a
// server that folded logins under a Turkish (or other locale with non-default
// case folding) default locale may therefore normalize to a different stored
// login here; such accounts must be migrated (or re-registered) when moving
// to this server.
func NewAccount(login, password string, accessLevel, lastServer int) Account {
	return Account{
		Login:       strings.ToLower(login),
		Password:    password,
		AccessLevel: accessLevel,
		LastServer:  lastServer,
	}
}
