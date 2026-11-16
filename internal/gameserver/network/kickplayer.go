package network

// KickPlayer ends the connection claiming account at once, as the login
// server asks when the account logs in again while in play here. The
// client reads ServerClose last, a session still waiting on its own login
// check stops waiting, a character in the world detaches without the
// lost-connection linger, and the end of the connection reports the logout
// to the login server. An account no connection claims or waits on is
// ignored. Assign it as the login link's LoginLinkHandlers.KickPlayer.
func (l *GameClientLink) KickPlayer(account string) {
	if l.clients == nil {
		return
	}
	// A connection still waiting on its login check claims nothing yet, so
	// it is stopped through the validator whether or not one holds the
	// account.
	if v := l.flow.validator; v != nil {
		v.Evict(account)
	}
	client, ok := l.clients.Lookup(account)
	if !ok {
		return
	}
	client.Session.Close()
}
