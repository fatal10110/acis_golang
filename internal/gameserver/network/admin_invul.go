package network

// adminInvul answers //invul: gm turns its invulnerability flag to the
// opposite of what it reads now, then is told what it reads after. Both
// reads include spawn protection and teleporting, so a GM still spawn
// protected stays invulnerable, and is told so, whatever its flag says.
func (l *GameClientLink) adminInvul(gm *livePlayer, _ string) {
	gm.SetInvul(!gm.Invul())
	if gm.Invul() {
		sendText(gm, "You are now invulnerable.")
		return
	}
	sendText(gm, "You are now vulnerable.")
}
