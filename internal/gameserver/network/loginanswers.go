package network

import "github.com/rs/zerolog"

// PlayerAuthAnswers returns the login link's PlayerAuthResponse handler. An
// approval reaching a client still waiting first reports the account in
// game to the login server over the link loginLink returns, so the login
// server's online roster holds the account before the client proceeds into
// the world; then the client is released. An answer no client waits on any
// more (evicted, timed out) reports nothing, and a rejection reports
// nothing.
func PlayerAuthAnswers(validator *SessionValidator, loginLink func() *LoginLink, log zerolog.Logger) func(account string, ok bool) {
	return func(account string, ok bool) {
		validator.ResolveThen(account, ok, func() {
			if link := loginLink(); link != nil {
				if err := link.SendPlayerInGame([]string{account}); err != nil {
					log.Debug().Err(err).Str("account", account).Msg("notify player in game")
				}
			}
		})
	}
}
