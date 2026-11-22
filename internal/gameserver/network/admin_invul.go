package network

// adminInvul answers //invul: gm turns its own command invulnerability
// source to the opposite of what it reads now, then is told what it reads
// after. Both reads include every other source (spawn protection,
// teleporting, an invincibility effect), so a GM still under one stays
// invulnerable, and is told so, whatever its own source says (#5084).
func (l *GameClientLink) adminInvul(gm *livePlayer, _ string) {
	gm.SetInvul(!gm.Invul())
	if gm.Invul() {
		sendText(gm, "You are now invulnerable.")
		return
	}
	sendText(gm, "You are now vulnerable.")
}
