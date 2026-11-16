package network

// KickPlayer ends the connection claiming account at once, as the login
// server asks when the account logs in again while in play here. The
// client reads ServerClose last, a session still waiting on its own login
// check stops waiting, a character in the world detaches without the
// lost-connection linger, and the end of the connection reports the logout
// to the login server. An account no connection claims is ignored. Assign
// it as the login link's LoginLinkHandlers.KickPlayer.
func (l *GameClientLink) KickPlayer(account string) {
	if l.clients == nil {
		return
	}
	client, ok := l.clients.Lookup(account)
	if !ok {
		return
	}
	if v := l.flow.validator; v != nil {
		v.Evict(account)
	}
	client.Session.Close()
}
